package crud_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/crud"
)

const overlayProjectConfig = `version = "4.0"
name = "test-project"
presets = ["claude"]
`

const overlayLocalConfig = `[profiles]
mine = ["backend"]

[[mcp_servers]]
name = "local-server"
command = "local-cmd"
`

func setupOverlayProject(t *testing.T) (baseDir, configPath string) {
	t.Helper()
	baseDir = t.TempDir()
	dir := filepath.Join(baseDir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "domains", "backend", "rules"), 0o755))
	configPath = filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(configPath, []byte(overlayProjectConfig), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.local.toml"), []byte(overlayLocalConfig), 0o600))
	return baseDir, configPath
}

func TestAddProfile_DoesNotPersistLocalOverlay(t *testing.T) {
	// Arrange
	baseDir, configPath := setupOverlayProject(t)
	op, err := crud.NewOperator(baseDir)
	require.NoError(t, err)

	// Act
	err = op.AddProfile(context.Background(), "team", []string{"backend"})

	// Assert
	require.NoError(t, err)
	saved, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Contains(t, string(saved), "team")
	assert.NotContains(t, string(saved), "mine")
	assert.NotContains(t, string(saved), "local-server")
	assert.NotContains(t, string(saved), "local-cmd")

	merged, err := config.LoadConfig(context.Background(), baseDir)
	require.NoError(t, err)
	assert.Contains(t, merged.Profiles, "team")
	assert.Contains(t, merged.Profiles, "mine")
	assert.Len(t, merged.MCPServersRaw, 1)
}

func TestAddInclude_DoesNotPersistLocalOverlay(t *testing.T) {
	// Arrange
	baseDir, configPath := setupOverlayProject(t)
	op, err := crud.NewOperator(baseDir)
	require.NoError(t, err)

	// Act
	err = op.AddInclude(context.Background(), &crud.AddIncludeRequest{
		Name:   "shared-rules",
		Source: "git@github.com:example/shared-rules.git",
	})

	// Assert
	require.NoError(t, err)
	saved, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Contains(t, string(saved), "shared-rules")
	assert.NotContains(t, string(saved), "mine")
	assert.NotContains(t, string(saved), "local-server")
}
