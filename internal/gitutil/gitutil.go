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
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/samber/oops"
)

const (
	commandTimeout = 30 * time.Second
	// pathChunk bounds how many paths one git invocation receives.
	pathChunk = 200
)

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
