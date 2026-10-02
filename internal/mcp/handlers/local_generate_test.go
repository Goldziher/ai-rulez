package handlers

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const generateSharedConfig = "version = \"4.0\"\nname = \"shared-project\"\npresets = [\"claude\"]\ngitignore = false\n"

func driftProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(cfgDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(generateSharedConfig), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.local.toml"), []byte("name = \"local-name\"\n"), 0o600))
	return dir
}

func TestGenerateOutputsHandler_LocalDriftGuardAndFlags(t *testing.T) {
	tests := []struct {
		name      string
		args      map[string]any
		wantError bool
		wantFile  bool
	}{
		{"drift is refused by default", map[string]any{}, true, false},
		{"allow_local_drift is not an MCP option", map[string]any{"allow_local_drift": true}, true, false},
		{"no_local generates the shared view", map[string]any{"no_local": true}, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := driftProject(t)
			args := map[string]any{"working_directory": dir}
			for k, v := range tt.args {
				args[k] = v
			}

			// Act
			res, err := GenerateOutputsHandler(context.Background(), newRequestWithArgs(args))

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.wantError, res.IsError, textOf(t, res))
			_, statErr := os.Stat(filepath.Join(dir, "CLAUDE.md"))
			assert.Equal(t, tt.wantFile, statErr == nil)
			if tt.wantFile {
				data, readErr := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
				require.NoError(t, readErr)
				if tt.args["no_local"] == true {
					assert.NotContains(t, string(data), "local-name")
				} else {
					assert.Contains(t, string(data), "local-name")
				}
			}
		})
	}
}

func TestValidateConfigHandler_NoLocalSkipsTheOverlay(t *testing.T) {
	dir := driftProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", "config.local.toml"), []byte("[header]\nstyle = \"weird\"\n"), 0o600))

	withOverlay, err := ValidateConfigHandler(context.Background(), newRequestWithArgs(map[string]any{"working_directory": dir}))
	require.NoError(t, err)
	without, err := ValidateConfigHandler(context.Background(), newRequestWithArgs(map[string]any{"working_directory": dir, "no_local": true}))
	require.NoError(t, err)

	assert.Equal(t, false, resultPayload(t, withOverlay)["valid"])
	assert.Equal(t, true, resultPayload(t, without)["valid"])
}
