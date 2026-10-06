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

func TestRunIgnoresInheritedGitDir(t *testing.T) {
	gitAvailable(t)
	other := t.TempDir()
	runGit(t, other, "init", "-q")
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.md"), []byte("x"), 0o600))
	runGit(t, dir, "add", "a.md")

	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
	t.Setenv("GIT_WORK_TREE", other)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(other, ".git", "index"))

	tracked, err := TrackedAmong(dir, []string{"a.md"})
	require.NoError(t, err)
	assert.True(t, tracked["a.md"], "git must address the given directory, not the hook's repository")
}

func TestChangedSince(t *testing.T) {
	gitAvailable(t)
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	write := func(name, body string) {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600))
	}
	write("a.md", "a")
	write("sub/b.md", "b")
	write("gone.md", "g")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "one")
	runGit(t, dir, "tag", "base")
	write("sub/c.md", "c")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "two")
	write("a.md", "changed in the working tree")
	write("new file.md", "untracked")
	require.NoError(t, os.Remove(filepath.Join(dir, "gone.md")))

	got, err := ChangedSince(filepath.Join(dir, "sub"), "base")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"a.md", "gone.md", "sub/c.md", "new file.md"}, got, "paths are repository-relative wherever dir is")

	head, err := ChangedSince(dir, "HEAD")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"a.md", "gone.md", "new file.md"}, head)

	_, err = ChangedSince(dir, "no-such-rev")
	assert.Error(t, err)
	_, err = ChangedSince(dir, "--output=/tmp/x")
	assert.Error(t, err, "a rev that looks like an option is rejected")
	_, err = ChangedSince(t.TempDir(), "HEAD")
	assert.Error(t, err)
}

func TestStageExecutable(t *testing.T) {
	gitAvailable(t)
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	path := filepath.Join(dir, "run.sh")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"), 0o644))                     //nolint:gosec // test
	require.NoError(t, os.WriteFile(filepath.Join(dir, "untracked.sh"), []byte("x"), 0o644)) //nolint:gosec // test
	runGit(t, dir, "add", "run.sh")

	changed, err := StageExecutable(path)
	require.NoError(t, err)
	assert.True(t, changed)
	_, _, err = Git{}.run(dir, nil, "diff", "--cached", "--quiet")
	assert.Error(t, err, "the mode change is staged")

	again, err := StageExecutable(path)
	require.NoError(t, err)
	assert.False(t, again, "already executable in the index")

	none, err := StageExecutable(filepath.Join(dir, "untracked.sh"))
	require.NoError(t, err)
	assert.False(t, none)
	outside, err := StageExecutable(filepath.Join(t.TempDir(), "x.sh"))
	require.NoError(t, err)
	assert.False(t, outside)
}
