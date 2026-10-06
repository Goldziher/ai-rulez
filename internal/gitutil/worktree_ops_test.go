package gitutil

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// opsRepo makes a repository with one commit on main and a bare remote called origin.
func opsRepo(t *testing.T) (repo, remote string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	root := t.TempDir()
	repo, remote = filepath.Join(root, "repo"), filepath.Join(root, "remote.git")
	require.NoError(t, os.MkdirAll(repo, 0o755))
	gitRun(t, repo, "init", "-q", "-b", "main")
	gitRun(t, repo, "config", "commit.gpgsign", "false")
	gitRun(t, repo, "config", "user.email", "t@example.com")
	gitRun(t, repo, "config", "user.name", "t")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "a.txt"), []byte("a\n"), 0o644))
	gitRun(t, repo, "add", "a.txt")
	gitRun(t, repo, "commit", "-q", "-m", "init")
	gitRun(t, root, "init", "-q", "--bare", remote)
	gitRun(t, repo, "remote", "add", "origin", remote)
	return repo, remote
}

func TestWorktreeLifecycle(t *testing.T) {
	// Arrange
	ctx := context.Background()
	repo, remote := opsRepo(t)
	g := Git{}
	wt := filepath.Join(t.TempDir(), "wt")

	// Act
	require.NoError(t, g.WorktreeAdd(ctx, repo, wt, "feature/x", "main"))
	require.NoError(t, os.WriteFile(filepath.Join(wt, "b.txt"), []byte("b\n"), 0o644))
	require.NoError(t, g.Add(ctx, wt, "b.txt"))
	staged, err := g.HasStaged(ctx, wt)
	require.NoError(t, err)
	sha, err := g.Commit(ctx, wt, "feat: add b")
	require.NoError(t, err)
	stagedAfter, err := g.HasStaged(ctx, wt)
	require.NoError(t, err)
	require.NoError(t, g.Push(ctx, wt, "origin", "feature/x"))

	// Assert: the main checkout never changed, and the branch reached the remote.
	assert.True(t, staged)
	assert.False(t, stagedAfter)
	assert.Len(t, sha, 40)
	assert.Equal(t, "main", g.CurrentBranch(ctx, repo))
	_, statErr := os.Stat(filepath.Join(repo, "b.txt"))
	assert.True(t, os.IsNotExist(statErr), "the file exists only in the worktree")
	out, err := g.Output(ctx, remote, "rev-parse", "refs/heads/feature/x")
	require.NoError(t, err)
	assert.Equal(t, sha, out)
	assert.True(t, g.BranchExists(ctx, repo, "feature/x"))
	assert.True(t, g.HasRemote(ctx, repo, "origin"))
	assert.False(t, g.HasRemote(ctx, repo, "upstream"))

	require.NoError(t, g.WorktreeRemove(ctx, repo, wt))
	_, statErr = os.Stat(wt)
	assert.True(t, os.IsNotExist(statErr))
	require.NoError(t, g.BranchDelete(ctx, repo, "feature/x"))
	assert.False(t, g.BranchExists(ctx, repo, "feature/x"))
}

func TestWorktreeAddRefusals(t *testing.T) {
	tests := []struct {
		name          string
		branch, start string
		want          string
	}{
		{"option as branch", "--detach", "main", "git option"},
		{"option as start point", "ok", "--upload-pack=x", "git option"},
		{"existing branch", "main", "main", "already exists"},
		{"unknown start point", "newbranch", "nope", "nope"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			repo, _ := opsRepo(t)

			// Act
			err := Git{}.WorktreeAdd(context.Background(), repo, filepath.Join(t.TempDir(), "wt"), tt.branch, tt.start)

			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestCommitOfAndNothingStaged(t *testing.T) {
	// Arrange
	ctx := context.Background()
	repo, _ := opsRepo(t)
	g := Git{}

	// Act
	head, err := g.CommitOf(ctx, repo, "main")
	_, missing := g.CommitOf(ctx, repo, "nope")
	_, option := g.CommitOf(ctx, repo, "--all")
	staged, serr := g.HasStaged(ctx, repo)

	// Assert
	require.NoError(t, err)
	assert.Len(t, head, 40)
	require.Error(t, missing)
	require.Error(t, option)
	assert.Contains(t, strings.ToLower(option.Error()), "option")
	require.NoError(t, serr)
	assert.False(t, staged)
}
