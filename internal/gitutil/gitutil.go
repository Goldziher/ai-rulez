// Package gitutil answers the few repository questions generation needs (is a
// path tracked, is it ignored, where is the exclude file) by shelling out to git.
// "Not a repository" and "git is not installed" are normal answers (nothing is
// tracked, no exclude file exists); a git command that fails inside a repository
// is reported as an error so callers can fail closed.
package gitutil

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/internal/logger"
)

const (
	commandTimeout = 30 * time.Second
	// pathChunk bounds how many paths one git invocation receives.
	pathChunk = 200
)

// maxIgnoreFileSize caps how much of one ignore file is mirrored; git itself
// stops reading a .gitignore at 100 MB. A var so tests can lower it.
var maxIgnoreFileSize int64 = 100 << 20

// run executes git in dir. ok is false when git could not run or exited
// non-zero; exitCode distinguishes "no match" (1) from failure (>1) for the
// commands that use it.
func run(dir string, stdin []byte, args ...string) (out []byte, exitCode int, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...) //nolint:gosec // fixed git subcommands
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	runErr := cmd.Run()
	if runErr == nil {
		return stdout.Bytes(), 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return stdout.Bytes(), exitErr.ExitCode(), oops.With("args", strings.Join(args, " ")).Wrapf(runErr, "git failed: %s", strings.TrimSpace(stderr.String()))
	}
	return nil, -1, oops.With("args", strings.Join(args, " ")).Wrapf(runErr, "run git")
}

// IsRepo reports whether dir is inside a git work tree. A missing git binary or
// a directory outside any repository is simply "no".
func IsRepo(dir string) bool {
	out, _, err := run(dir, nil, "rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(string(out)) == "true"
}

// TopLevel returns the work tree root containing dir, or "" outside a repository.
func TopLevel(dir string) string {
	out, _, err := run(dir, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return ""
	}
	return filepath.Clean(strings.TrimSpace(string(out)))
}

// literalSpecs turns slash-separated relative paths into literal pathspecs so
// glob characters in a file name are not interpreted.
func literalSpecs(paths []string) []string {
	specs := make([]string, len(paths))
	for i, p := range paths {
		specs[i] = ":(literal)" + p
	}
	return specs
}

// TrackedAmong returns which of paths (slash-separated, relative to dir) git
// tracks. Outside a repository nothing is tracked. When git fails inside a
// repository the error is returned: callers should treat every candidate as
// tracked.
func TrackedAmong(dir string, paths []string) (map[string]bool, error) {
	tracked := map[string]bool{}
	if len(paths) == 0 || !IsRepo(dir) {
		return tracked, nil
	}
	for start := 0; start < len(paths); start += pathChunk {
		end := min(start+pathChunk, len(paths))
		args := append([]string{"ls-files", "-z", "--"}, literalSpecs(paths[start:end])...)
		out, _, err := run(dir, nil, args...)
		if err != nil {
			return nil, err
		}
		for _, name := range strings.Split(string(out), "\x00") {
			if name != "" {
				tracked[filepath.ToSlash(name)] = true
			}
		}
	}
	return tracked, nil
}

// IgnoredAmong returns which of paths (slash-separated, relative to dir) git
// ignores, per .gitignore files, .git/info/exclude and the global excludes file,
// in one call. Tracked files are judged by the patterns too (--no-index). It
// returns (nil, nil) outside a repository so callers can fall back to their own
// matcher.
func IgnoredAmong(dir string, paths []string) (map[string]bool, error) {
	if len(paths) == 0 || !IsRepo(dir) {
		return nil, nil
	}
	var stdin bytes.Buffer
	for _, p := range paths {
		stdin.WriteString(p)
		stdin.WriteByte(0)
	}
	out, code, err := run(dir, stdin.Bytes(), "check-ignore", "--no-index", "--stdin", "-z")
	if err != nil && code != 1 { // exit status 1 means "none of them is ignored"
		return nil, err
	}
	ignored := map[string]bool{}
	for _, name := range strings.Split(string(out), "\x00") {
		if name != "" {
			ignored[filepath.ToSlash(name)] = true
		}
	}
	return ignored, nil
}

// IgnoreMatch is the last ignore rule git found for one path.
type IgnoreMatch struct {
	Source  string // file holding the rule; empty when nothing matched
	Line    int
	Pattern string // as written, "!" prefix included; empty when nothing matched
}

// Matched reports whether any rule matched the path.
func (m IgnoreMatch) Matched() bool { return m.Pattern != "" }

// Negated reports whether the last matching rule un-ignores the path.
func (m IgnoreMatch) Negated() bool { return strings.HasPrefix(m.Pattern, "!") }

// Ignored reports whether the last matching rule ignores the path.
func (m IgnoreMatch) Ignored() bool { return m.Matched() && !m.Negated() }

// IgnoreRules returns, for each of paths (slash-separated, relative to dir), the
// last rule git matched across .gitignore files, .git/info/exclude and the global
// excludes file, negations included. Paths nothing matched map to a zero
// IgnoreMatch. It returns (nil, nil) outside a repository.
func IgnoreRules(dir string, paths []string) (map[string]IgnoreMatch, error) {
	if len(paths) == 0 || !IsRepo(dir) {
		return nil, nil
	}
	return checkIgnore(dir, nil, paths)
}

// checkIgnore runs one batched verbose check-ignore in dir.
func checkIgnore(dir string, gitFlags, paths []string) (map[string]IgnoreMatch, error) {
	var stdin bytes.Buffer
	for _, p := range paths {
		stdin.WriteString(p)
		stdin.WriteByte(0)
	}
	args := append(append([]string{}, gitFlags...), "check-ignore", "--no-index", "-v", "-n", "--stdin", "-z")
	out, code, err := run(dir, stdin.Bytes(), args...)
	if err != nil && code != 1 { // exit status 1 means "none of them is ignored"
		return nil, err
	}
	// -z -v -n emits <source> <line> <pattern> <path>, each NUL-terminated.
	fields := strings.Split(string(out), "\x00")
	rules := make(map[string]IgnoreMatch, len(paths))
	for i := 0; i+3 < len(fields); i += 4 {
		line := 0
		if n, convErr := strconv.Atoi(fields[i+1]); convErr == nil {
			line = n
		}
		rules[filepath.ToSlash(fields[i+3])] = IgnoreMatch{Source: fields[i], Line: line, Pattern: fields[i+2]}
	}
	return rules, nil
}

// IgnoreRulesMirrored is IgnoreRules evaluated against a throwaway copy of the
// repository's ignore files, so the caller can leave out content of its own
// without touching the user's files. rewrite receives each ignore file's path
// (slash-separated, relative to the work tree root, or "info/exclude" for the
// repository exclude file) and its content, and returns the content to use. The
// global excludes file is honored as configured. Paths are relative to dir. It
// returns (nil, nil) outside a repository.
func IgnoreRulesMirrored(dir string, paths []string, rewrite func(rel, content string) string) (map[string]IgnoreMatch, error) {
	if len(paths) == 0 || !IsRepo(dir) {
		return nil, nil
	}
	top := TopLevel(dir)
	prefix := RepoRelative(top, dir)
	if top == "" || prefix == "" {
		return nil, oops.Errorf("cannot place %s inside its repository", dir)
	}
	if prefix == "." {
		prefix = ""
	} else {
		prefix += "/"
	}

	mirror, err := os.MkdirTemp("", "ai-rulez-ignore-")
	if err != nil {
		return nil, oops.Wrapf(err, "create ignore mirror")
	}
	defer os.RemoveAll(mirror) //nolint:errcheck // best-effort cleanup of a temp dir
	if err := fillIgnoreMirror(dir, top, mirror, rewrite); err != nil {
		return nil, err
	}

	var flags []string
	if v, _, cfgErr := run(dir, nil, "config", "--get", "--type=path", "core.excludesFile"); cfgErr == nil {
		if path := strings.TrimSpace(string(v)); path != "" {
			flags = []string{"-c", "core.excludesFile=" + path}
		}
	}
	prefixed := make([]string, len(paths))
	for i, p := range paths {
		prefixed[i] = prefix + p
	}
	rules, err := checkIgnore(mirror, flags, prefixed)
	if err != nil {
		return nil, err
	}
	back := make(map[string]IgnoreMatch, len(rules))
	for p, m := range rules {
		back[strings.TrimPrefix(p, prefix)] = m
	}
	return back, nil
}

// fillIgnoreMirror makes mirror a repository holding copies of top's ignore
// files (and the exclude file) passed through rewrite.
func fillIgnoreMirror(dir, top, mirror string, rewrite func(rel, content string) string) error {
	if _, _, err := run(mirror, nil, "init", "-q"); err != nil {
		return err
	}

	// Tracked and untracked-but-not-ignored files cover every .gitignore git
	// would read; one inside an ignored directory is never consulted anyway.
	out, _, err := run(top, nil, "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--", ":(glob)**/.gitignore")
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, rel := range strings.Split(string(out), "\x00") {
		rel = filepath.ToSlash(rel)
		if rel == "" || seen[rel] {
			continue
		}
		seen[rel] = true
		if err := copyRewritten(filepath.Join(top, filepath.FromSlash(rel)), filepath.Join(mirror, filepath.FromSlash(rel)), rel, rewrite); err != nil {
			return err
		}
	}
	if exclude := InfoExcludePath(dir); exclude != "" {
		if err := copyRewritten(exclude, filepath.Join(mirror, ".git", "info", "exclude"), "info/exclude", rewrite); err != nil {
			return err
		}
	}

	return nil
}

// copyRewritten copies src to dst (creating parents) with its content passed
// through rewrite. A missing src is skipped, and so is anything that is not a
// regular file (git does not read a symlinked ignore file, and following one to
// a device or a huge file would exhaust memory) or is larger than
// maxIgnoreFileSize.
func copyRewritten(src, dst, rel string, rewrite func(rel, content string) string) error {
	info, err := os.Lstat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return oops.With("path", src).Wrapf(err, "stat ignore file")
	}
	if !info.Mode().IsRegular() {
		logger.Debug("Skipping an ignore file that is not a regular file", "path", src)
		return nil
	}
	f, err := os.Open(src) //nolint:gosec // ignore file located through git
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return oops.With("path", src).Wrapf(err, "read ignore file")
	}
	defer f.Close() //nolint:errcheck // read-only handle
	data, err := io.ReadAll(io.LimitReader(f, maxIgnoreFileSize+1))
	if err != nil {
		return oops.With("path", src).Wrapf(err, "read ignore file")
	}
	if int64(len(data)) > maxIgnoreFileSize {
		logger.Warn("Skipping an ignore file larger than the size limit", "path", src, "limit", maxIgnoreFileSize)
		return nil
	}
	content := string(data)
	if rewrite != nil {
		content = rewrite(rel, content)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return oops.Wrapf(err, "create ignore mirror directory")
	}
	if err := os.WriteFile(dst, []byte(content), 0o600); err != nil {
		return oops.With("path", dst).Wrapf(err, "write ignore mirror")
	}
	return nil
}

// InfoExcludePath returns the repository's info/exclude file, resolved through
// git so linked worktrees (whose .git is a file) get the shared one. It returns
// "" outside a repository.
func InfoExcludePath(dir string) string {
	out, _, err := run(dir, nil, "rev-parse", "--git-path", "info/exclude")
	if err != nil {
		return ""
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return ""
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	return filepath.Clean(path)
}

// RepoRelative converts an absolute path to a slash-separated path relative to
// the given work tree root (from TopLevel), or "" when it is not below it. The
// path need not exist.
func RepoRelative(top, absPath string) string {
	if top == "" {
		return ""
	}
	rel, err := filepath.Rel(Resolve(top), Resolve(absPath))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	return filepath.ToSlash(rel)
}

// Resolve resolves symlinks in the longest existing ancestor of path and
// re-appends the rest, so paths that do not exist yet compare correctly.
func Resolve(path string) string {
	rest := ""
	for p := path; ; p = filepath.Dir(p) {
		if resolved, err := filepath.EvalSymlinks(p); err == nil {
			return filepath.Join(resolved, rest)
		}
		rest = filepath.Join(filepath.Base(p), rest)
		if filepath.Dir(p) == p {
			return path
		}
	}
}

// TrackedFiles returns every path in the git index below dir, slash-separated
// and relative to dir, mapped to its git file mode (0o100644, 0o100755,
// 0o120000 for a symlink, ...). ok is false outside a repository, where
// callers fall back to walking the file system. The index is the source, so a
// path listed there counts even when its working-tree file is absent.
func TrackedFiles(dir string) (files map[string]uint32, ok bool, err error) {
	if !IsRepo(dir) {
		return nil, false, nil
	}
	out, _, err := run(dir, nil, "ls-files", "-s", "-z")
	if err != nil {
		return nil, true, err
	}
	files = map[string]uint32{}
	for _, entry := range strings.Split(string(out), "\x00") {
		meta, name, found := strings.Cut(entry, "\t")
		if !found {
			continue
		}
		fields := strings.Fields(meta)
		if len(fields) == 0 {
			continue
		}
		mode, perr := strconv.ParseUint(fields[0], 8, 32)
		if perr != nil {
			continue
		}
		files[filepath.ToSlash(name)] = uint32(mode)
	}
	return files, true, nil
}

// IsLinkedWorktree reports whether dir is inside a linked git worktree (one made
// with `git worktree add`) rather than the main checkout. Outside a repository,
// or when git cannot run, it is false.
func IsLinkedWorktree(dir string) bool {
	gitDir := gitPath(dir, "--git-dir")
	common := gitPath(dir, "--git-common-dir")
	return gitDir != "" && common != "" && gitDir != common
}

// gitPath resolves the path `git rev-parse <flag>` prints, which may be
// relative to dir, to a symlink-free absolute path.
func gitPath(dir, flag string) string {
	out, _, err := run(dir, nil, "rev-parse", flag)
	if err != nil {
		return ""
	}
	p := strings.TrimSpace(string(out))
	if p == "" {
		return ""
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(dir, p)
	}
	return Resolve(p)
}
