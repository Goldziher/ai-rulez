package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthoringACommandOnALegacyTreeNamesTheDeprecation(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(filepath.Join(cfgDir, "rules"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte("version = \"5.0\"\nname = \"p\"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "rules", "r.md"), []byte("# R\n"), 0o644))
	chdir(t, dir)

	stderr := captureStderr(t, func() {
		_, err := newContentOperator(false)
		require.NoError(t, err)
	})

	assert.Contains(t, stderr, "legacy layout")
	assert.Contains(t, stderr, "migrate okf")
}

func TestAuthoringACommandOnAnOKFBundleIsQuiet(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(filepath.Join(cfgDir, "rules"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte("version = \"5.0\"\nname = \"p\"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "rules", "r.md"), []byte("# R\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "index.md"), []byte("---\nokf_version: \"0.2\"\n---\n"), 0o644))
	chdir(t, dir)

	stderr := captureStderr(t, func() {
		_, err := newContentOperator(false)
		require.NoError(t, err)
	})

	assert.NotContains(t, stderr, "legacy layout")
}
