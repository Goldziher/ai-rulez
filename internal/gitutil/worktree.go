package gitutil

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

// Output runs git in dir and returns its trimmed standard output. A failure carries git's
// own message. It is the seam for the worktree and branch helpers below.
func (g Git) Output(ctx context.Context, dir string, args ...string) (string, error) {
	res := g.Exec(ctx, dir, nil, args...)
	if err := ResultErr(res); err != nil {
		if res.Status == runner.StatusUnavailable {
			return "", fmt.Errorf("git is not available: %w", err)
		}
		msg := strings.TrimSpace(string(res.Stderr))
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", args[0], msg)
	}
	return strings.TrimSpace(string(res.Stdout)), nil
}

// CommitOf resolves rev to a commit id; it fails when rev is not a commit of the repository at dir.
func (g Git) CommitOf(ctx context.Context, dir, rev string) (string, error) {
	if err := CheckArg("revision", rev); err != nil {
		return "", err
	}
	return g.Output(ctx, dir, "rev-parse", "--verify", "--quiet", "--end-of-options", rev+"^{commit}")
}

// CurrentBranch is the checked-out branch of dir, or "" for a detached HEAD.
func (g Git) CurrentBranch(ctx context.Context, dir string) string {
	out, err := g.Output(ctx, dir, "symbolic-ref", "--short", "--quiet", "HEAD")
	if err != nil {
		return ""
	}
	return out
}

// BranchExists reports whether the local branch exists in the repository at dir.
func (g Git) BranchExists(ctx context.Context, dir, branch string) bool {
	if CheckArg("branch", branch) != nil {
		return true // refuse to guess: callers treat an odd name as taken
	}
	_, err := g.Output(ctx, dir, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

// HasRemote reports whether the repository at dir has a remote called name.
func (g Git) HasRemote(ctx context.Context, dir, name string) bool {
	if CheckArg("remote", name) != nil {
		return false
	}
	out, err := g.Output(ctx, dir, "remote")
	if err != nil {
		return false
	}
	for _, r := range strings.Fields(out) {
		if r == name {
			return true
		}
	}
	return false
}

// WorktreeAdd creates a linked worktree at path on a new branch started from start, so the main
// checkout is untouched. It refuses a branch that already exists.
func (g Git) WorktreeAdd(ctx context.Context, repo, path, branch, start string) error {
	for what, v := range map[string]string{"worktree path": path, "branch": branch, "start point": start} {
		if err := CheckArg(what, v); err != nil {
			return err
		}
	}
	if g.BranchExists(ctx, repo, branch) {
		return fmt.Errorf("the branch %s already exists", branch)
	}
	_, err := g.Output(ctx, repo, "worktree", "add", "--quiet", "-b", branch, "--end-of-options", path, start)
	return err
}

// WorktreeRemove deletes a linked worktree and its administrative files, discarding changes in it.
func (g Git) WorktreeRemove(ctx context.Context, repo, path string) error {
	if err := CheckArg("worktree path", path); err != nil {
		return err
	}
	_, err := g.Output(ctx, repo, "worktree", "remove", "--force", "--", path)
	return err
}

// WorktreePrune forgets worktrees whose directory is gone.
func (g Git) WorktreePrune(ctx context.Context, repo string) error {
	_, err := g.Output(ctx, repo, "worktree", "prune")
	return err
}

// BranchDelete force-deletes a local branch (one the caller created and abandoned).
func (g Git) BranchDelete(ctx context.Context, repo, branch string) error {
	if err := CheckArg("branch", branch); err != nil {
		return err
	}
	_, err := g.Output(ctx, repo, "branch", "-D", "--", branch)
	return err
}

// Add stages the given repository-relative paths literally (glob characters are not interpreted).
func (g Git) Add(ctx context.Context, dir string, paths ...string) error {
	if len(paths) == 0 {
		return errors.New("nothing to add")
	}
	args := append([]string{"add", "--"}, literalSpecs(paths)...)
	_, err := g.Output(ctx, dir, args...)
	return err
}

// HasStaged reports whether the index differs from HEAD.
func (g Git) HasStaged(ctx context.Context, dir string) (bool, error) {
	res := g.Exec(ctx, dir, nil, "diff", "--cached", "--quiet")
	switch {
	case res.Status == runner.StatusOK:
		return false, nil
	case res.Status == runner.StatusExit && res.ExitCode == 1:
		return true, nil
	}
	return false, fmt.Errorf("git diff --cached: %s", strings.TrimSpace(string(res.Stderr)))
}

// Commit commits the index with message and returns the new commit id. Hooks are skipped: the commit is
// a machine-made one in a throwaway worktree, where a repository's pre-commit setup is not installed.
func (g Git) Commit(ctx context.Context, dir, message string) (string, error) {
	if _, err := g.Output(ctx, dir, "commit", "--quiet", "--no-verify", "-m", message); err != nil {
		return "", err
	}
	return g.Output(ctx, dir, "rev-parse", "HEAD")
}

// Push publishes branch to remote and sets it as the upstream.
func (g Git) Push(ctx context.Context, dir, remote, branch string) error {
	for what, v := range map[string]string{"remote": remote, "branch": branch} {
		if err := CheckArg(what, v); err != nil {
			return err
		}
	}
	_, err := g.Output(ctx, dir, "push", "--quiet", "--set-upstream", "--", remote, branch)
	return err
}
