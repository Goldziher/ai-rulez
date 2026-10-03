package config

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig_YAMLOverlayNumbersOnTOMLMain(t *testing.T) {
	// Arrange
	base := t.TempDir()
	dir := filepath.Join(base, ".ai-rulez")
	writeProjectFile(t, dir, "config.toml", overlayMainTOML)
	writeProjectFile(t, dir, "config.local.yaml", "mcp_servers:\n  - name: srv\n    env: {PORT: 8080}\n")

	// Act
	cfg, err := LoadConfig(context.Background(), base)

	// Assert
	require.NoError(t, err)
	require.Len(t, cfg.MCPServersRaw, 1)
	assert.Equal(t, "8080", cfg.MCPServersRaw[0].Env["PORT"])
	assert.Equal(t, "x", cfg.MCPServersRaw[0].Command)
}

func TestLoadConfig_MergedDecodeErrorNamesBothFiles(t *testing.T) {
	// Arrange
	base := t.TempDir()
	dir := filepath.Join(base, ".ai-rulez")
	writeProjectFile(t, dir, "config.toml", overlayMainTOML)
	writeProjectFile(t, dir, "config.local.toml", "compact = \"yes\"\n")

	// Act
	_, err := LoadConfig(context.Background(), base)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode config.toml merged with config.local.toml")
}

func TestSaveConfig_RefusesMergedConfig_YAMLMain(t *testing.T) {
	// Arrange
	base := t.TempDir()
	dir := filepath.Join(base, ".ai-rulez")
	writeProjectFile(t, dir, "config.yaml", overlayMainYAML)
	writeProjectFile(t, dir, "config.local.yaml", overlayLocalYAML)
	merged, err := LoadConfig(context.Background(), base)
	require.NoError(t, err)
	before, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	require.NoError(t, err)

	// Act
	err = SaveConfig(merged, dir)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "local overlay")
	after, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after))
}

func TestFindLocalConfigFile_StatErrorIsReturned(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stat through a regular file reports not-exist on Windows")
	}
	// Arrange: a regular file where the config dir should be makes stat fail with ENOTDIR.
	file := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))

	// Act
	_, err := findLocalConfigFile(file, "config.toml")

	// Assert
	require.Error(t, err)
}

func TestLocalOverlay_KeyPaths(t *testing.T) {
	// Arrange
	overlay := &LocalOverlay{Doc: decodeDoc(t, `
name = "mine"
presets = ["!cursor", "codex"]
[profiles]
dev = ["a"]
[header]
style = "compact"
[[mcp_servers]]
name = "gh"
[mcp_servers.env]
TOKEN = "s3cr3t"
[[scopes]]
path = "svc"
`)}

	// Act
	keys := overlay.KeyPaths()

	// Assert
	assert.Equal(t, []string{
		"header.style", "mcp_servers.gh.env.TOKEN", "mcp_servers.gh.name", "name",
		"presets", "profiles.dev", "scopes.svc.path",
	}, keys)
	assert.Nil(t, (*LocalOverlay)(nil).KeyPaths())
}
