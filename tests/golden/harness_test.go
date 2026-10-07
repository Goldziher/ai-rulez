// Package golden is the characterization suite for the generator (issue #229, step S0).
//
// It builds the CLI once, runs it over a matrix of fixture projects in scratch
// directories and compares every byte the run produced (output tree, manifests,
// exit codes and normalized stdout/stderr) against goldens recorded from the code
// before the engine was refactored. A refactor that claims to change no output has
// to pass it unchanged. Regenerate deliberately with UPDATE_GOLDEN=1.
//
// A scenario records a sha-256 per file; with full set it also records the content
// of the final tree, which keeps most goldens small while still being byte exact.
package golden

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

var (
	binaryPath    string
	binaryVersion string
	toolsDir      string // the stub executables every run sees on PATH
	buildOnce     sync.Once
	buildErr      error
)

// The CLI probes PATH (doctor's tool check, the AR601 MCP command check), so a
// golden would otherwise depend on what the machine has installed. Every run
// gets a PATH of stubs for the tools the goldens were recorded with, then only
// the system directories git and sh live in. cursor and gemini stay absent.
var (
	stubTools  = []string{"claude", "codex", "copilot", "opencode", "npx"}
	systemPath = []string{"/usr/bin", "/bin"}
)

// writeTools fills dir with the stub tools and a git that runs the real one.
func writeTools(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	git, err := exec.LookPath("git")
	if err != nil {
		return fmt.Errorf("git is needed on PATH: %w", err)
	}
	scripts := map[string]string{"git": "#!/bin/sh\nexec '" + strings.ReplaceAll(git, "'", `'\''`) + "' \"$@\"\n"}
	for _, name := range stubTools {
		scripts[name] = "#!/bin/sh\nexit 0\n"
	}
	for name, body := range scripts {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil { //nolint:gosec // an executable stub
			return err
		}
	}
	return nil
}

// TestMain only builds lazily (see binary), so `go test -run Other` stays fast.
func TestMain(m *testing.M) {
	testutil.CeilGit()
	code := m.Run()
	if binaryPath != "" {
		_ = os.RemoveAll(filepath.Dir(binaryPath)) //nolint:errcheck // best-effort cleanup
	}
	os.Exit(code)
}

// binary builds ./cmd/ai-rulez once per test process.
func binary(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "ai-rulez-golden-")
		if err != nil {
			buildErr = err
			return
		}
		out := filepath.Join(dir, "ai-rulez")
		if runtime.GOOS == "windows" {
			out += ".exe"
		}
		cmd := exec.Command("go", "build", "-o", out, "./cmd/ai-rulez") //nolint:gosec // fixed arguments
		cmd.Dir = repoRoot()
		if msg, err := cmd.CombinedOutput(); err != nil {
			buildErr = fmt.Errorf("go build: %w\n%s", err, msg)
			return
		}
		if err := writeTools(filepath.Join(dir, "path")); err != nil {
			buildErr = fmt.Errorf("stub tools: %w", err)
			return
		}
		toolsDir = filepath.Join(dir, "path")
		binaryPath = out
		ver := exec.Command(out, "version") //nolint:gosec // the binary was just built
		raw, _ := ver.CombinedOutput()      //nolint:errcheck // an unreadable version only skips normalization
		if m := regexp.MustCompile(`version=(\S+)`).FindSubmatch(raw); m != nil {
			binaryVersion = string(m[1])
		}
	})
	if buildErr != nil {
		t.Fatalf("build the CLI: %v", buildErr)
	}
	return binaryPath
}

func repoRoot() string {
	wd, _ := os.Getwd() //nolint:errcheck // the test runs in its package directory
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}

// stepKind is what a step does to the scratch project.
type stepKind int

const (
	stepRun    stepKind = iota // run the CLI
	stepWrite                  // write (or overwrite) a file
	stepRemove                 // remove a file
	stepAppend                 // append to a file
)

// step is one action of a scenario.
type step struct {
	kind stepKind
	args []string // stepRun
	env  []string // stepRun: extra KEY=VALUE
	home bool     // path is relative to HOME instead of the project
	path string   // stepWrite, stepRemove, stepAppend
	data string   // stepWrite, stepAppend
	dir  string   // stepRun: working directory relative to the project root
}

func run(args ...string) step { return step{kind: stepRun, args: args} }

func runEnv(env []string, args ...string) step { return step{kind: stepRun, args: args, env: env} }

// scenario is one fixture project plus the steps run against it.
type scenario struct {
	name  string
	files map[string]string // project files, relative path -> content
	exec  map[string]bool   // files that get the executable bit
	home  map[string]string // files below HOME
	git   bool              // run `git init` in the project first
	// prepare runs after the files are written, before the first step (base is the
	// parent of the project and home directories).
	prepare func(t *testing.T, base, root string)
	steps   []step
	full    bool // record file content, not only hashes
	// omit lists project files (slash paths) left out of the snapshot because their
	// bytes depend on the scratch directory (a resolved ${PROJECT_ROOT}).
	omit []string
	// maskDigests replaces 64-digit hex digests in stdout/stderr with a placeholder
	// (a lock digest covers the scratch path of a file:// include).
	maskDigests bool
}

func execute(t *testing.T, sc scenario) string {
	t.Helper()
	bin := binary(t)
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "project")
	home := filepath.Join(base, "home")
	for _, d := range []string{root, home} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	sub := strings.NewReplacer("{{BASE}}", base, "{{ROOT}}", root, "{{HOME}}", home)
	writeTree(t, root, sc.files, sc.exec, sub)
	writeTree(t, home, sc.home, nil, sub)
	if sc.prepare != nil {
		sc.prepare(t, base, root)
	}
	if sc.git {
		cmd := exec.Command("git", "init", "-q", "--template=", ".") //nolint:gosec // fixed arguments
		cmd.Dir = root
		cmd.Env = baseEnv(home, nil)
		if msg, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git init: %v\n%s", err, msg)
		}
	}

	norm := newNormalizer(base, root, home)
	var out strings.Builder
	fmt.Fprintf(&out, "# scenario %s\n", sc.name)
	var prevProject, prevHome map[string]string
	for i, st := range sc.steps {
		switch st.kind {
		case stepWrite, stepAppend, stepRemove:
			target := filepath.Join(root, filepath.FromSlash(st.path))
			if st.home {
				target = filepath.Join(home, filepath.FromSlash(st.path))
			}
			st.data = sub.Replace(st.data)
			applyEdit(t, target, st)
			fmt.Fprintf(&out, "\n## step %d: %s %s\n", i+1, map[stepKind]string{stepWrite: "write", stepAppend: "append", stepRemove: "remove"}[st.kind], st.path)
		case stepRun:
			cmd := exec.Command(bin, expand(sub, st.args)...) //nolint:gosec // the binary was built by this test
			cmd.Dir = filepath.Join(root, filepath.FromSlash(st.dir))
			cmd.Env = baseEnv(home, expand(sub, st.env))
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			exit := 0
			if err := cmd.Run(); err != nil {
				ee, ok := err.(*exec.ExitError)
				if !ok {
					t.Fatalf("run %v: %v", st.args, err)
				}
				exit = ee.ExitCode()
			}
			so, se := norm.text(stdout.String()), norm.text(stderr.String())
			if sc.maskDigests {
				so, se = hexDigest.ReplaceAllString(so, "<sha256>"), hexDigest.ReplaceAllString(se, "<sha256>")
			}
			fmt.Fprintf(&out, "\n## step %d: ai-rulez %s\nexit: %d\n--- stdout\n%s--- stderr\n%s", i+1,
				strings.Join(st.args, " "), exit, so, se)
		}
		last := i == len(sc.steps)-1
		var section string
		section, prevProject = snapshot(t, root, norm, sc.full && last, prevProject, last, sc.omit)
		fmt.Fprintf(&out, "--- project tree\n%s", section)
		section, prevHome = snapshot(t, home, norm, sc.full && last, prevHome, last, nil)
		fmt.Fprintf(&out, "--- home tree\n%s", section)
	}
	return out.String()
}

var hexDigest = regexp.MustCompile(`[0-9a-f]{64}`)

func expand(sub *strings.Replacer, in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = sub.Replace(s)
	}
	return out
}

func applyEdit(t *testing.T, target string, st step) {
	t.Helper()
	switch st.kind {
	case stepWrite:
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(st.data), 0o644); err != nil { //nolint:gosec // test fixture
			t.Fatal(err)
		}
	case stepAppend:
		f, err := os.OpenFile(target, os.O_APPEND|os.O_WRONLY, 0o644) //nolint:gosec // test fixture
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.WriteString(st.data); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	case stepRemove:
		if err := os.RemoveAll(target); err != nil {
			t.Fatal(err)
		}
	case stepRun:
	}
}

func writeTree(t *testing.T, root string, files map[string]string, exec map[string]bool, sub *strings.Replacer) {
	t.Helper()
	for rel, content := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		mode := fs.FileMode(0o644)
		if exec[rel] {
			mode = 0o755
		}
		if err := os.WriteFile(path, []byte(sub.Replace(content)), mode); err != nil { //nolint:gosec // test fixture
			t.Fatal(err)
		}
		if err := os.Chmod(path, mode); err != nil { // defeat the umask
			t.Fatal(err)
		}
	}
}

// baseEnv is the complete environment of every run: nothing from the host leaks in.
func baseEnv(home string, extra []string) []string {
	env := []string{
		"PATH=" + strings.Join(append([]string{toolsDir}, systemPath...), string(os.PathListSeparator)),
		"HOME=" + home,
		"USERPROFILE=" + home,
		"TMPDIR=" + os.TempDir(),
		"TZ=UTC",
		"LANG=C",
		"NO_COLOR=1",
		"TERM=dumb",
		"CI=1",
		// Temporary directories may sit inside a checkout (a TMPDIR below the
		// repository): its ignore rules must not decide a golden.
		"GIT_CEILING_DIRECTORIES=" + os.TempDir(),
		"GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=golden", "GIT_AUTHOR_EMAIL=golden@example.invalid",
		"GIT_COMMITTER_NAME=golden", "GIT_COMMITTER_EMAIL=golden@example.invalid",
	}
	return append(env, extra...)
}

type normalizer struct{ pairs []string }

func newNormalizer(base, root, home string) normalizer {
	var p []string
	add := func(from, to string) {
		if from == "" {
			return
		}
		p = append(p, from, to)
	}
	// Longest paths first so the project is replaced before HOME-relative shorter forms.
	for _, v := range variants(root) {
		add(v, "$ROOT")
	}
	for _, v := range variants(home) {
		add(v, "$HOME")
	}
	for _, v := range variants(base) {
		add(v, "$BASE")
	}
	add(binaryVersion, "$VERSION")
	return normalizer{pairs: p}
}

func variants(dir string) []string {
	out := []string{dir}
	if strings.HasPrefix(dir, "/private/") {
		out = append(out, strings.TrimPrefix(dir, "/private"))
	}
	return out
}

func (n normalizer) text(s string) string {
	if len(n.pairs) == 0 {
		return s
	}
	return strings.NewReplacer(n.pairs...).Replace(s)
}

// ackLedger is the per-user cache (announced commands, include clones); its file
// names hash the scratch path, so it is left out of the snapshot.
const ackLedger = ".cache"

// snapshot lists every file below dir (sorted, .git excluded) with mode, size and
// sha-256, and the content of text files when full is set. Before the final step it
// lists only what changed since prev ("+" new, "~" changed, "-" removed); with the
// final flag, or when prev is nil, it lists everything. It returns the listing and
// the state to diff the next step against.
func snapshot(t *testing.T, dir string, norm normalizer, full bool, prev map[string]string, final bool, omit []string) (string, map[string]string) {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path) //nolint:errcheck // path is below dir
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if d.IsDir() && d.Name() == ".git" {
			return fs.SkipDir
		}
		if rel == ackLedger {
			return fs.SkipDir
		}
		if slices.Contains(omit, rel) {
			return nil
		}
		paths = append(paths, rel)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	state := map[string]string{}
	contents := map[string]string{}
	for _, rel := range paths {
		abs := filepath.Join(dir, filepath.FromSlash(rel))
		info, err := os.Lstat(abs)
		if err != nil {
			t.Fatal(err)
		}
		switch {
		case info.Mode()&fs.ModeSymlink != 0:
			target, _ := os.Readlink(abs) //nolint:errcheck // reported as empty
			state[rel] = fmt.Sprintf("%s symlink -> %s", rel, norm.text(target))
		case info.IsDir():
			state[rel] = fmt.Sprintf("%s/ dir %04o", rel, info.Mode().Perm())
		default:
			data, err := os.ReadFile(abs) //nolint:gosec // scratch tree
			if err != nil {
				t.Fatal(err)
			}
			if utf8.Valid(data) {
				// Scratch paths embedded in a file (manifests record some) must not
				// make the hash differ between runs.
				data = []byte(norm.text(string(data)))
			}
			sum := sha256.Sum256(data)
			state[rel] = fmt.Sprintf("%s file %04o %d %s", rel, info.Mode().Perm(), len(data), hex.EncodeToString(sum[:]))
			if full && utf8.Valid(data) {
				c := ">>>>\n" + string(data)
				if len(data) > 0 && data[len(data)-1] != '\n' {
					c += "\n<<<< (no trailing newline)\n"
				} else {
					c += "<<<<\n"
				}
				contents[rel] = c
			}
		}
	}
	var b strings.Builder
	if prev == nil || final {
		for _, rel := range paths {
			b.WriteString(state[rel] + "\n" + contents[rel])
		}
	} else {
		for _, rel := range paths {
			old, ok := prev[rel]
			switch {
			case !ok:
				b.WriteString("+ " + state[rel] + "\n")
			case old != state[rel]:
				b.WriteString("~ " + state[rel] + "\n")
			}
		}
		var gone []string
		for rel := range prev {
			if _, ok := state[rel]; !ok {
				gone = append(gone, rel)
			}
		}
		sort.Strings(gone)
		for _, rel := range gone {
			b.WriteString("- " + prev[rel] + "\n")
		}
	}
	if b.Len() == 0 {
		return "(no change)\n", state
	}
	return b.String(), state
}

// compare checks got against testdata/<name>.golden, or rewrites it under UPDATE_GOLDEN=1.
func compare(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil { //nolint:gosec // golden file
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path) //nolint:gosec // golden file
	if err != nil {
		t.Fatalf("missing golden %s (record it with UPDATE_GOLDEN=1): %v", path, err)
	}
	if string(want) == got {
		return
	}
	actual := filepath.Join(t.TempDir(), name+".actual")
	_ = os.WriteFile(actual, []byte(got), 0o644) //nolint:errcheck,gosec // diagnostic copy
	t.Fatalf("output differs from %s\n%s\n(actual written to %s; diff it against the golden)", path, firstDiff(string(want), got), actual)
}

func firstDiff(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := 0; i < len(w) || i < len(g); i++ {
		var a, b string
		if i < len(w) {
			a = w[i]
		}
		if i < len(g) {
			b = g[i]
		}
		if a != b {
			return fmt.Sprintf("first difference at line %d:\n  want: %s\n  got:  %s", i+1, a, b)
		}
	}
	return "no line differs (trailing bytes?)"
}
