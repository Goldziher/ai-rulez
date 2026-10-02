package handlers

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An empty name or description with local: true clears the local override
// instead of overriding the shared value with an empty string.
func TestUpdateConfigHandler_LocalEmptyNameAndDescriptionClearTheOverride(t *testing.T) {
	for _, field := range []string{"name", "description"} {
		t.Run(field, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			writeMinimalConfig(t, dir)
			set := newRequestWithArgs(map[string]any{"working_directory": dir, "local": true, field: "mine-value"})
			res, err := UpdateConfigHandler(context.Background(), set)
			require.NoError(t, err)
			require.False(t, res.IsError, textOf(t, res))

			// Act
			clearReq := newRequestWithArgs(map[string]any{"working_directory": dir, "local": true, field: ""})
			res, err = UpdateConfigHandler(context.Background(), clearReq)

			// Assert
			require.NoError(t, err)
			require.False(t, res.IsError, textOf(t, res))
			local, err := os.ReadFile(filepath.Join(dir, ".ai-rulez", "config.local.yaml"))
			require.NoError(t, err)
			assert.NotContains(t, string(local), "mine-value")
			assert.NotContains(t, string(local), field+": \"\"", "an empty override must not be written")
		})
	}
}
