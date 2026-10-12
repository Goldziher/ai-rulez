package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRootPlacement(t *testing.T) {
	repo := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(repo, ".git"), 0o755))
	nested := filepath.Join(repo, "products", "em", "webapps")
	require.NoError(t, os.MkdirAll(nested, 0o755))

	t.Run("outermost root has no location", func(t *testing.T) {
		assert.Equal(t, "", rootRelInRepo(repo))
	})
	t.Run("a nested root is located from the repository root", func(t *testing.T) {
		assert.Equal(t, "products/em/webapps", rootRelInRepo(nested))
	})
	t.Run("outside a repository there is no location", func(t *testing.T) {
		assert.Equal(t, "", rootRelInRepo(t.TempDir()))
	})
	t.Run("a worktree .git file marks the repository root", func(t *testing.T) {
		wt := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: /elsewhere\n"), 0o644))
		sub := filepath.Join(wt, "svc")
		require.NoError(t, os.Mkdir(sub, 0o755))
		assert.Equal(t, "svc", rootRelInRepo(sub))
	})
	t.Run("nested only when an enclosing directory is itself a root", func(t *testing.T) {
		assert.False(t, nestedInRepo(nested), "no ancestor holds a config")
		writeConfigDir(t, filepath.Join(repo, "products"))
		assert.True(t, nestedInRepo(nested))
		assert.False(t, nestedInRepo(filepath.Join(repo, "products")), "the enclosing root is not nested itself")
		assert.False(t, nestedInRepo(t.TempDir()), "outside a repository nothing is nested")
	})
	t.Run("the repository root itself counts as an enclosing root", func(t *testing.T) {
		sub := filepath.Join(repo, "x", "y")
		require.NoError(t, os.MkdirAll(sub, 0o755))
		writeConfigDir(t, repo)
		assert.True(t, nestedInRepo(sub))
		assert.False(t, nestedInRepo(repo))
	})
}

func writeConfigDir(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".ai-rulez"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", "config.toml"), []byte("name = \"x\"\n"), 0o644))
}
