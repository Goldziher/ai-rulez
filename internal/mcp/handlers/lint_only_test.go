package handlers

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func TestGenerateOutputsHandler_LintOnlyWritesNothing(t *testing.T) {
	tests := []struct {
		name string
		toml string
	}{
		{name: "presets omitted", toml: "version = \"5.0\"\nname = \"lint\"\n"},
		{name: "empty presets", toml: "version = \"5.0\"\nname = \"lint\"\npresets = []\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			testutil.IsolateHome(t)
			dir := t.TempDir()
			cfgDir := filepath.Join(dir, ".ai-rulez")
			require.NoError(t, os.MkdirAll(cfgDir, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(tt.toml), 0o600))

			// Act
			res, err := GenerateOutputsHandler(context.Background(), newRequestWithArgs(map[string]any{"working_directory": dir}))

			// Assert
			require.NoError(t, err)
			require.False(t, res.IsError, textOf(t, res))
			for _, name := range []string{"AGENTS.md", "CLAUDE.md", ".mcp.json"} {
				assert.NoFileExists(t, filepath.Join(dir, name))
			}
		})
	}
}
