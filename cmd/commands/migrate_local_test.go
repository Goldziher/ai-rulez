package commands

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
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

func TestMigrateV4_StrayOverlayIsAWarningNotAFailure(t *testing.T) {
	// Arrange: a TOML main config and two overlays in different formats.
	project := t.TempDir()
	dir := filepath.Join(project, ".ai-rulez")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.toml"), []byte("version = \"4.0\"\nname = \"x\"\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.local.toml"), []byte("name = \"a\"\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.local.yaml"), []byte("name: b\n"), 0o600))
	t.Chdir(project)

	// Act: os.Exit would terminate the test binary.
	runMigrateV4()

	// Assert
	assert.FileExists(t, filepath.Join(dir, "config.local.yaml"), "the stray file is left for the user")
}

func TestMigrateLocalOverlay_MapsSchemaKey(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.local.yaml"),
		[]byte("$schema: https://example.com/s.json\nname: a\n"), 0o600))

	_, to, err := config.MigrateLocalOverlayToTOML(dir)

	require.NoError(t, err)
	data, err := os.ReadFile(to)
	require.NoError(t, err)
	assert.Contains(t, string(data), `schema = 'https://example.com/s.json'`)
	assert.NotContains(t, string(data), "$schema")
}

func TestMigrateV4_KeepsLegacyMCPServers(t *testing.T) {
	tests := []struct {
		name        string
		files       map[string]string
		wantServers []string
		wantRemoved []string
		wantKept    []string
	}{
		{
			name: "inline servers and mcp.yaml are both written",
			files: map[string]string{
				"config.yaml": "version: \"3.0\"\nname: p\npresets: [claude]\nmcp_servers:\n  - name: zeta\n    command: z\n  - name: alpha\n    command: a\n",
				"mcp.yaml":    "mcp_servers:\n  - name: legacy-b\n    command: b\n  - name: legacy-a\n    command: a\n  - name: alpha\n    command: dup\n",
			},
			wantServers: []string{"zeta", "alpha", "legacy-a", "legacy-b"},
			wantRemoved: []string{"mcp.yaml", "config.yaml"},
		},
		{
			name: "a second legacy file that was never loaded is kept",
			files: map[string]string{
				"config.yaml": "version: \"3.0\"\nname: p\npresets: [claude]\n",
				"mcp.toml":    "[[mcp_servers]]\nname = \"first\"\ncommand = \"f\"\n",
				"mcp.json":    `{"mcp_servers":[{"name":"second","command":"s"}]}`,
			},
			wantServers: []string{"first"},
			wantRemoved: []string{"mcp.toml"},
			wantKept:    []string{"mcp.json"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			project := t.TempDir()
			dir := filepath.Join(project, ".ai-rulez")
			require.NoError(t, os.MkdirAll(dir, 0o755))
			for name, body := range tt.files {
				require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600))
			}
			t.Chdir(project)

			// Act
			runMigrateV4()

			// Assert
			cfg, err := config.LoadConfig(context.Background(), project)
			require.NoError(t, err)
			var names []string
			for _, s := range cfg.MCPServersRaw {
				names = append(names, s.Name)
			}
			assert.Equal(t, tt.wantServers, names)
			for _, f := range tt.wantRemoved {
				assert.NoFileExists(t, filepath.Join(dir, f))
			}
			for _, f := range tt.wantKept {
				assert.FileExists(t, filepath.Join(dir, f))
			}
		})
	}
}
