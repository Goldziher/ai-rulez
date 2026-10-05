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
