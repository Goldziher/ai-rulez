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
}
