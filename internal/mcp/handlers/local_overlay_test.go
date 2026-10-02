package handlers

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	handlerSharedConfig = "version = \"4.0\"\nname = \"shared\"\npresets = [\"claude\"]\n"
	handlerLocalConfig  = "name = \"mine\"\n\n[[mcp_servers]]\nname = \"ghsrv\"\ncommand = \"gh\"\n\n" +
		"[mcp_servers.env]\nTOKEN = \"s3cr3t-token\"\n"
)

func writeOverlayProject(t *testing.T, local string) (dir, configPath string) {
	t.Helper()
	dir = t.TempDir()
	cfgDir := filepath.Join(dir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(cfgDir, 0o755))
	configPath = filepath.Join(cfgDir, "config.toml")
	require.NoError(t, os.WriteFile(configPath, []byte(handlerSharedConfig), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.local.toml"), []byte(local), 0o600))
	return dir, configPath
}

func TestReadConfigHandler_ReturnsSharedViewAndOverlayKeys(t *testing.T) {
	// Arrange
	dir, _ := writeOverlayProject(t, handlerLocalConfig)

	// Act
	res, err := ReadConfigHandler(context.Background(), newRequestWithArgs(map[string]any{"working_directory": dir}))

	// Assert
	require.NoError(t, err)
	require.False(t, res.IsError)
	payload := resultPayload(t, res)
	assert.Equal(t, "shared", payload["name"], "read_config is the editable shared view")
	overlay, ok := payload["local_overlay"].(map[string]interface{})
	require.True(t, ok, "local_overlay must be present")
	assert.Equal(t, filepath.Join(dir, ".ai-rulez", "config.local.toml"), overlay["path"])
	assert.ElementsMatch(t, []interface{}{"name", "mcp_servers.ghsrv.name", "mcp_servers.ghsrv.command", "mcp_servers.ghsrv.env.TOKEN"}, overlay["keys"])
	assert.NotContains(t, textOf(t, res), "s3cr3t-token", "overlay values must never be returned")
}

func TestReadConfigHandler_NoOverlayOmitsField(t *testing.T) {
	dir := t.TempDir()
	writeMinimalConfig(t, dir)

	res, err := ReadConfigHandler(context.Background(), newRequestWithArgs(map[string]any{"working_directory": dir}))

	require.NoError(t, err)
	assert.NotContains(t, resultPayload(t, res), "local_overlay")
}

func TestUpdateConfigHandler_DoesNotPersistLocalOverlay(t *testing.T) {
	// Arrange
	dir, configPath := writeOverlayProject(t, handlerLocalConfig)

	// Act
	res, err := UpdateConfigHandler(context.Background(), newRequestWithArgs(map[string]any{
		"working_directory": dir,
		"default_effort":    "high",
	}))

	// Assert
	require.NoError(t, err)
	require.False(t, res.IsError)
	saved, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Contains(t, string(saved), "high")
	assert.NotContains(t, string(saved), "mine")
	assert.NotContains(t, string(saved), "ghsrv")
	assert.NotContains(t, string(saved), "s3cr3t-token")
}

func TestValidateConfigHandler_ChecksLocalOverlaySchema(t *testing.T) {
	tests := []struct {
		name      string
		local     string
		wantValid bool
	}{
		{"valid overlay", "name = \"mine\"\n", true},
		{"schema-invalid overlay", "[header]\nstyle = \"weird\"\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir, _ := writeOverlayProject(t, tt.local)

			// Act
			res, err := ValidateConfigHandler(context.Background(), newRequestWithArgs(map[string]any{"working_directory": dir}))

			// Assert
			require.NoError(t, err)
			payload := resultPayload(t, res)
			assert.Equal(t, tt.wantValid, payload["valid"], "%v", payload)
			if !tt.wantValid {
				assert.Contains(t, payload["error"], "config.local.toml")
			}
		})
	}
}

func textOf(t *testing.T, res *sdkmcp.CallToolResult) string {
	t.Helper()
	require.NotEmpty(t, res.Content)
	tc, ok := res.Content[0].(*sdkmcp.TextContent)
	require.True(t, ok)
	return tc.Text
}
