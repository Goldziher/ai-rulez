package commands

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func TestMigrateV4_ConvertsLocalOverlayToTOML(t *testing.T) {
	tests := []struct {
		name      string
		mainFile  string
		mainBody  string
		localFile string
		localBody string
	}{
		{
			"yaml main and yaml local", "config.yaml", "version: \"3.0\"\nname: shared\npresets: [claude]\n",
			"config.local.yaml", "name: mine\nmcp_servers:\n  - name: s\n    command: c\n    env: {PORT: 8080}\n",
		},
		{
			"toml main and json local", "config.toml", "version = \"4.0\"\nname = \"shared\"\npresets = [\"claude\"]\n",
			"config.local.json", `{"name":"mine","mcp_servers":[{"name":"s","command":"c","env":{"PORT":"8080"}}]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			project := t.TempDir()
			dir := filepath.Join(project, ".ai-rulez")
			require.NoError(t, os.MkdirAll(dir, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, tt.mainFile), []byte(tt.mainBody), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(dir, tt.localFile), []byte(tt.localBody), 0o600))
			t.Chdir(project)

			// Act
			runMigrateV4()

			// Assert
			assert.NoFileExists(t, filepath.Join(dir, tt.localFile))
			assert.FileExists(t, filepath.Join(dir, "config.local.toml"))
			cfg, err := config.LoadConfig(context.Background(), project)
			require.NoError(t, err)
			assert.Equal(t, "mine", cfg.Name)
			require.Len(t, cfg.MCPServersRaw, 1)
			assert.Equal(t, "8080", cfg.MCPServersRaw[0].Env["PORT"])
		})
	}
}

func TestMigrateLocalOverlay_KeepsOldFileWhenTargetExists(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.local.yaml"), []byte("name: a\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.local.toml"), []byte("name = \"b\"\n"), 0o600))

	// Act
	_, _, err := config.MigrateLocalOverlayToTOML(dir)

	// Assert
	require.Error(t, err)
	assert.FileExists(t, filepath.Join(dir, "config.local.yaml"))
}
