package gitutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanEnvDropsInheritedRepoVariables(t *testing.T) {
	in := []string{
		"PATH=/bin", "GIT_DIR=/x/.git", "GIT_WORK_TREE=/x", "GIT_INDEX_FILE=/x/.git/index",
		"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=core.bare", "GIT_CONFIG_VALUE_0=true", "GIT_AUTHOR_NAME=me", "HOME=/h",
	}
	assert.Equal(t, []string{"PATH=/bin", "GIT_AUTHOR_NAME=me", "HOME=/h"}, CleanEnv(in))
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
