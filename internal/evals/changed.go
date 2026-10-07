package evals

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

// GitFunc runs git in dir and returns its standard output. Tests replace it.
type GitFunc func(dir string, args ...string) (string, error)

// ExecGit is the GitFunc that runs the git binary.
func ExecGit(dir string, args ...string) (string, error) { return NewGitFunc(nil)(dir, args...) }

// NewGitFunc is the GitFunc that runs git through r (nil: real git).
func NewGitFunc(r runner.Runner) GitFunc {
	git := gitutil.New(r)
	return func(dir string, args ...string) (string, error) {
		res := git.Exec(context.Background(), dir, nil, args...)
		if err := gitutil.ResultErr(res); err != nil {
			return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
		}
		return string(res.Stdout), nil
	}
}

// ChangedSkills returns the ids of skills with a changed file below the skill
// directory or below their project-level eval directory. The change set is what
// `git diff --name-only <base>` reports (the base against the working tree, so it
// covers both committed and uncommitted edits) plus untracked files.
func ChangedSkills(run GitFunc, repoDir, base string, skills []Skill) (map[string]bool, error) {
	if base == "" {
		base = "HEAD"
	}
	if strings.HasPrefix(base, "-") {
		return nil, fmt.Errorf("--base %q looks like an option, not a git ref", base)
	}
	top, err := run(repoDir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("--changed-only needs a git repository: %w", err)
	}
	topDir := strings.TrimSpace(top)
	if _, err := run(repoDir, "rev-parse", "--verify", "--quiet", base+"^{commit}"); err != nil {
		return nil, fmt.Errorf("--base %q is not a commit: %w", base, err)
	}
	diff, err := run(repoDir, "diff", "--name-only", "-z", base, "--")
	if err != nil {
		return nil, err
	}
	untracked, err := run(repoDir, "ls-files", "--others", "--exclude-standard", "--full-name", "-z")
	if err != nil {
		return nil, err
	}
	var changed []string
	// NUL-separated: git quotes non-ASCII names in line mode.
	for _, line := range strings.Split(diff+"\x00"+untracked, "\x00") {
		if line != "" {
			changed = append(changed, filepath.Join(topDir, filepath.FromSlash(line)))
		}
	}
	out := map[string]bool{}
	for i := range skills {
		roots := append([]string{skills[i].Dir}, skills[i].EvalDirs...)
		for _, file := range changed {
			for _, root := range roots {
				if resolvedWithin(root, file) {
					out[skills[i].ID] = true
				}
			}
		}
	}
	return out, nil
}

// resolvedWithin compares after resolving symlinks in the directory, since
// git reports the real top-level path (on macOS /private/var vs /var).
func resolvedWithin(root, file string) bool {
	if within(root, file) {
		return true
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		return within(resolved, file)
	}
	return false
}
