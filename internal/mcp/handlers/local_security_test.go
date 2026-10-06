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

func TestValidateConfigHandler_NeverEchoesOverlayValues(t *testing.T) {
	tests := []struct {
		name  string
		local string
	}{
		{"mistyped mcp args", "[[mcp_servers]]\nname = \"x\"\ncommand = \"c\"\nargs = \"SECRETY2\"\n"},
		{"default without profiles", "default = \"SECRETY6\"\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			writeMinimalConfig(t, dir)
			require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", "config.local.toml"), []byte(tt.local), 0o600))

			// Act
			res, err := ValidateConfigHandler(context.Background(), newRequestWithArgs(map[string]any{"working_directory": dir}))

			// Assert
			require.NoError(t, err)
			assert.NotContains(t, textOf(t, res), "SECRET")
		})
	}
}

func TestSourceHandlers_RedactCredentialedURLs(t *testing.T) {
	tests := []struct {
		name    string
		handler func(context.Context, *ToolRequest) (*sdkmcp.CallToolResult, error)
		args    map[string]any
	}{
		{"install_skill", InstallSkillHandler, map[string]any{
			"name": "s", "source": "https://user:SECRETURL@example.com/o/r.git?access_token=SECRETQ", "local": true}},
		{"add_include", AddIncludeHandler, map[string]any{
			"name": "i", "source": "https://user:SECRETURL@example.com/o/r.git?access_token=SECRETQ", "local": true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			writeMinimalConfig(t, dir)
			tt.args["working_directory"] = dir

			// Act
			res, err := tt.handler(context.Background(), newRequestWithArgs(tt.args))

			// Assert
			require.NoError(t, err)
			text := textOf(t, res)
			assert.NotContains(t, text, "SECRET")
		})
	}
}
