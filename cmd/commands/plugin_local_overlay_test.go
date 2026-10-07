package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPluginLoadOptions(t *testing.T) {
	assert.Len(t, pluginLoadOptions(true), 1)
	assert.Empty(t, pluginLoadOptions(false))
}

func TestSelectRecursivePluginConfigs_IgnoresLocalOverlay(t *testing.T) {
	// Arrange: the overlay alone would turn this project into a plugin producer.
	dir := filepath.Join(t.TempDir(), ".ai-rulez")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	mainPath := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(mainPath, []byte("version = \"5.0\"\nname = \"x\"\n"), 0o600))
	overlay := "[plugin]\nname = \"p\"\nversion = \"1.0.0\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.local.toml"), []byte(overlay), 0o600))

	// Act
	selected, err := selectRecursivePluginConfigs([]string{mainPath})

	// Assert
	require.NoError(t, err)
	assert.Empty(t, selected)
}
