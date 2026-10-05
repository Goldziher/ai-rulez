package gitutil

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hookEnv makes the process look like a git hook: git exports GIT_DIR and
// GIT_INDEX_FILE (and friends) to the hooks it runs.
func hookEnv(t *testing.T, gitDir string) {
	t.Helper()
	t.Setenv("GIT_DIR", gitDir)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(gitDir, "index"))
}

// linkedWorktree returns a repository with one commit, a linked worktree of it,
// and the worktree's private git dir (what a hook in that worktree gets as GIT_DIR).
func linkedWorktree(t *testing.T) (repo, worktree, gitDir string) {
	t.Helper()
	gitAvailable(t)
	repo = filepath.Join(t.TempDir(), "main")
	require.NoError(t, os.MkdirAll(repo, 0o755))
	runGit(t, repo, "init", "-q")
	runGit(t, repo, "commit", "-q", "--allow-empty", "-m", "init")
	worktree = filepath.Join(filepath.Dir(repo), "wt")
	runGit(t, repo, "worktree", "add", "-q", worktree)
	return repo, worktree, filepath.Join(repo, ".git", "worktrees", "wt")
}

func readConfig(t *testing.T, repo string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repo, ".git", "config"))
	require.NoError(t, err)
	return string(data)
}

// Regression: under a git hook, `git -C <mirror> init` honored the inherited
// GIT_DIR and re-initialized the hook's repository instead of the mirror; for a
// linked worktree that writes core.bare = true into the shared config, which
// breaks every git command in every worktree.
func TestIgnoreRulesMirrored_InsideGitHookLeavesTheOuterRepositoryAlone(t *testing.T) {
	repo, worktree, gitDir := linkedWorktree(t)
	require.NoError(t, os.WriteFile(filepath.Join(worktree, ".gitignore"), []byte("mine\nowned\n"), 0o644))
	before := readConfig(t, repo)
	require.NotContains(t, before, "bare = true")

	hookEnv(t, gitDir)
	rules, err := IgnoreRulesMirrored(worktree, []string{"mine", "owned"}, func(rel, content string) string {
		if rel == ".gitignore" {
			return "mine\n"
		}
		return content
	})

	require.NoError(t, err)
	assert.True(t, rules["mine"].Ignored())
	assert.False(t, rules["owned"].Matched(), "the rewritten mirror decides")
	assert.Equal(t, before, readConfig(t, repo), "the shared repository config must be untouched (core.bare)")
}

func TestCommand_StripsRepositorySelectingEnvironment(t *testing.T) {
	for _, name := range strippedEnv {
		t.Setenv(name, "/somewhere/else")
	}
	t.Setenv("GIT_AUTHOR_NAME", "keep-me")

	cmd := Command(context.Background(), "", "version")

	for _, kv := range cmd.Env {
		for _, name := range strippedEnv {
			assert.False(t, strings.HasPrefix(kv, name+"="), "%s must be removed", name)
		}
	}
	assert.Contains(t, cmd.Env, "GIT_AUTHOR_NAME=keep-me", "unrelated GIT_* variables are kept")
}

func TestCommand_DirBecomesDashC(t *testing.T) {
	cmd := Command(context.Background(), "/tmp/x", "status")
	assert.Equal(t, []string{"git", "-C", "/tmp/x", "status"}, cmd.Args)
	cmd = Command(context.Background(), "", "status")
	assert.Equal(t, []string{"git", "status"}, cmd.Args)
}
