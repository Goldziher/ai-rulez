package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadRecursiveExclude(t *testing.T) {
	write := func(t *testing.T, body string) string {
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, ".ai-rulez"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", "config.toml"), []byte(body), 0o644))
		return dir
	}

	got, err := LoadRecursiveExclude(write(t, "version = \"5.0\"\nrecursive_exclude = [\"a\", \"b/*\"]\n"))
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b/*"}, got)

	got, err = LoadRecursiveExclude(write(t, "version = \"5.0\"\n"))
	require.NoError(t, err)
	assert.Empty(t, got)

	got, err = LoadRecursiveExclude(t.TempDir())
	require.NoError(t, err, "a directory without configuration has none")
	assert.Empty(t, got)

	_, err = LoadRecursiveExclude(write(t, "recursive_exclude = ["))
	assert.Error(t, err)
}
