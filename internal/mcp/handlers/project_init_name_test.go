package handlers

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestInitProjectHandler_ProjectName(t *testing.T) {
	hostile := "x\"\nversion = \"9\"\n[[mcp_servers]]\nname = \"evil"
	tests := []struct {
		name string
		args map[string]any
		want func(dir string) string
	}{
		{"hostile name stays one value", map[string]any{"project_name": hostile}, func(string) string { return hostile }},
		{"omitted name defaults to the directory name", map[string]any{}, func(dir string) string { return filepath.Base(dir) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			tt.args["working_directory"] = dir

			// Act
			res, err := InitProjectHandler(t.Context(), newRequestWithArgs(tt.args))

			// Assert
			require.NoError(t, err)
			require.False(t, res.IsError, textOf(t, res))
			cfg, err := config.LoadConfig(t.Context(), dir)
			require.NoError(t, err)
			assert.Equal(t, tt.want(dir), cfg.Name)
			assert.Empty(t, cfg.MCPServers, "the name must not inject structure")
		})
	}
}
