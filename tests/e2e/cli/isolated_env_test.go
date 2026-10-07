package cli

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Goldziher/ai-rulez/v5/tests/e2e/testutil"
	"github.com/stretchr/testify/require"
)

// isoEnv runs the built binary with an environment built from nothing: a
// private HOME and XDG tree, a PATH of system directories plus fakes, a fixed
// git identity, and no credential or CI variable from the developer's shell.
// Every command test in this package that needs network-free, user-state-free
// behavior goes through it.
type isoEnv struct {
	t     *testing.T
	home  string
	fakes string
	vars  map[string]string
}

func newIsoEnv(t *testing.T) *isoEnv {
	t.Helper()
	home := t.TempDir()
	gitCfg := filepath.Join(home, "gitconfig")
	require.NoError(t, os.WriteFile(gitCfg, []byte("[init]\n\tdefaultBranch = main\n"), 0o600))
	e := &isoEnv{t: t, home: home, vars: map[string]string{
		"HOME":                home,
		"USERPROFILE":         home,
		"XDG_CONFIG_HOME":     filepath.Join(home, ".config"),
		"XDG_CACHE_HOME":      filepath.Join(home, ".cache"),
		"XDG_STATE_HOME":      filepath.Join(home, ".local", "state"),
		"XDG_DATA_HOME":       filepath.Join(home, ".local", "share"),
		"TMPDIR":              t.TempDir(),
		"PATH":                basePath(t),
		"LANG":                "C.UTF-8",
		"GIT_CONFIG_GLOBAL":   gitCfg,
		"GIT_CONFIG_NOSYSTEM": "1",
		"GIT_AUTHOR_NAME":     "e2e",
		"GIT_AUTHOR_EMAIL":    "e2e@example.test",
		"GIT_COMMITTER_NAME":  "e2e",
		"GIT_COMMITTER_EMAIL": "e2e@example.test",
		"AI_RULEZ_EVAL_DATE":  "2026-10-01",
	}}
	if runtime.GOOS == "windows" {
		for _, k := range []string{"SystemRoot", "ComSpec", "PATHEXT", "TEMP", "TMP"} {
			if v := os.Getenv(k); v != "" {
				e.vars[k] = v
			}
		}
	}
	return e
}

// basePath is the directory of git plus the system binary directories. A real
// gh or npm may live in any of them (git and gh share /opt/homebrew/bin), which
// is why a fake must be asserted to resolve first.
func basePath(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		return os.Getenv("PATH")
	}
	dirs := []string{}
	if git, err := exec.LookPath("git"); err == nil {
		dirs = append(dirs, filepath.Dir(git))
	}
	dirs = append(dirs, "/usr/bin", "/bin", "/usr/sbin", "/sbin")
	return strings.Join(dirs, string(os.PathListSeparator))
}

func (e *isoEnv) set(k, v string) *isoEnv { e.vars[k] = v; return e }

// fakeTool writes an executable shell script named name into a fakes
// directory at the front of PATH and asserts that a lookup through that PATH
// finds it before any real tool. It returns the directory, where the scripts
// conventionally log their calls.
func (e *isoEnv) fakeTool(name, script string) string {
	e.t.Helper()
	if runtime.GOOS == "windows" {
		e.t.Skip("the fake tools are POSIX shell scripts")
	}
	if e.fakes == "" {
		e.fakes = e.t.TempDir()
		e.vars["PATH"] = e.fakes + string(os.PathListSeparator) + e.vars["PATH"]
	}
	path := filepath.Join(e.fakes, name)
	require.NoError(e.t, os.WriteFile(path, []byte(script), 0o755)) //nolint:gosec // an executable test stub
	got, err := lookPathIn(e.vars["PATH"], name)
	require.NoError(e.t, err)
	require.Equal(e.t, path, got, "the fake %s must resolve first on PATH, or the test would run a real %s", name, name)
	return e.fakes
}

// lookPathIn resolves name the way exec.LookPath would with PATH set to path.
func lookPathIn(path, name string) (string, error) {
	for _, dir := range filepath.SplitList(path) {
		candidate := filepath.Join(dir, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return candidate, nil
		}
	}
	return "", errors.New(name + " is not on PATH")
}

func (e *isoEnv) environ() []string {
	keys := make([]string, 0, len(e.vars))
	for k := range e.vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if e.vars[k] != "" {
			out = append(out, k+"="+e.vars[k])
		}
	}
	return out
}

// run executes the binary in dir with only the isolated environment.
func (e *isoEnv) run(dir string, args ...string) *testutil.CLIResult {
	e.t.Helper()
	return e.runStdin(dir, "", args...)
}

func (e *isoEnv) runStdin(dir, stdin string, args ...string) *testutil.CLIResult {
	e.t.Helper()
	cmd := exec.Command(testutil.SetupTestBinary(e.t), args...) //nolint:gosec // the test binary
	cmd.Dir = dir
	cmd.Env = e.environ()
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	require.NoError(e.t, cmd.Start())
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var err error
	select {
	case err = <-done:
	case <-time.After(2 * testutil.TestTimeout):
		_ = cmd.Process.Kill() //nolint:errcheck // the test fails below
		e.t.Fatalf("ai-rulez %v timed out", args)
	}
	code := 0
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code = exitErr.ExitCode()
	} else if err != nil {
		code = -1
	}
	return &testutil.CLIResult{Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: code, Err: err}
}

// git runs git in dir with the isolated identity and no signing.
func (e *isoEnv) git(dir string, args ...string) string {
	e.t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false"}, args...)...) //nolint:gosec // test git
	cmd.Dir = dir
	cmd.Env = e.environ()
	out, err := cmd.CombinedOutput()
	require.NoError(e.t, err, "git %v: %s", args, out)
	return strings.TrimSpace(string(out))
}

// commitAll makes dir a git repository (if it is not one) and commits everything.
func (e *isoEnv) commitAll(dir, msg string) {
	e.t.Helper()
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		e.git(dir, "init", "-q", "-b", "main")
	}
	e.git(dir, "add", "-A")
	e.git(dir, "commit", "-q", "--allow-empty", "-m", msg)
}

// writeTree writes files (slash-separated paths relative to root).
func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
}

// writeExec writes an executable script.
func writeExec(t *testing.T, path, script string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the test scripts are POSIX shell scripts")
	}
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755)) //nolint:gosec // an executable test stub
}

// requireJSONDoc asserts stdout is exactly one JSON document and decodes it.
func requireJSONDoc(t *testing.T, res *testutil.CLIResult) map[string]any {
	t.Helper()
	var doc map[string]any
	dec := json.NewDecoder(strings.NewReader(res.Stdout))
	require.NoError(t, dec.Decode(&doc), "stdout is not JSON (exit %d)\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	var extra any
	require.Error(t, dec.Decode(&extra), "stdout holds more than one JSON document: %s", res.Stdout)
	return doc
}

// blockedOn skips a test that pins an open finding. Set
// AI_RULEZ_E2E_RUN_BLOCKED=1 to run it anyway and see the defect fail.
func blockedOn(t *testing.T, finding string) {
	t.Helper()
	if os.Getenv("AI_RULEZ_E2E_RUN_BLOCKED") != "1" {
		t.Skip("blocked on " + finding)
	}
}

// lingeringTempDir is a temporary directory for a test whose binary can leave a
// detached child behind (telemetry record starts `telemetry flush
// --background`, which outlives the command by design). t.TempDir fails the test
// when such a child writes during cleanup, so removal is retried for a while.
func lingeringTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "ai-rulez-e2e-")
	require.NoError(t, err)
	t.Cleanup(func() {
		deadline := time.Now().Add(10 * time.Second)
		for os.RemoveAll(dir) != nil && time.Now().Before(deadline) {
			time.Sleep(200 * time.Millisecond)
		}
	})
	return dir
}

// minimalProject is a project with one rule and one skill.
func minimalProject(t *testing.T, extraConfig string) string {
	t.Helper()
	return minimalProjectIn(t, t.TempDir(), extraConfig)
}

func minimalProjectIn(t *testing.T, root, extraConfig string) string {
	t.Helper()
	writeTree(t, root, map[string]string{
		".ai-rulez/config.toml":            "version = \"5.0\"\nname = \"e2e\"\npresets = [\"claude\"]\ngitignore = false\n" + extraConfig,
		".ai-rulez/rules/local.md":         "---\ndescription: local rule\n---\n# Local\n\nAlways run the tests.\n",
		".ai-rulez/skills/deploy/SKILL.md": "---\nname: deploy\ndescription: Deploy the service to staging.\n---\n# Deploy\n\nRun make deploy.\n",
	})
	return root
}
