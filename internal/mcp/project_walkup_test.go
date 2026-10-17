package mcp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
)

func TestProjectToolsWalkUpFromASubdirectory(t *testing.T) {
	t.Parallel()
	layouts := []struct {
		name  string
		setup func(t *testing.T) string
	}{
		{"ai-rulez dir", telemetryProject},
		{"root file", func(t *testing.T) string {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "ai-rulez.toml"), []byte("version = \"5.0\"\nname = \"t\"\npresets = [\"claude\"]\n"), 0o600))
			return dir
		}},
	}
	for _, tt := range layouts {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Arrange
			root := tt.setup(t)
			sub := filepath.Join(root, "sub", "deep")
			require.NoError(t, os.MkdirAll(sub, 0o750))
			session := connect(t, NewServer("test", WithRoot(sub)))

			// Act and Assert
			for _, tool := range []string{"read_config", "list_installed_skills", "list_domains"} {
				res := call(t, session, tool, map[string]any{})
				assert.False(t, res.IsError, "%s: %s", tool, textOfResult(t, res))
			}
		})
	}
}

func TestInitProjectDoesNotWalkUpAndRefusesUnderAnAncestorConfig(t *testing.T) {
	t.Parallel()
	// Arrange
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "ai-rulez.toml"), []byte("version = \"5.0\"\nname = \"t\"\npresets = [\"claude\"]\n"), 0o600))
	require.NoError(t, gitutil.CommandNoContext("", "init", "-q", root).Run())
	sub := filepath.Join(root, "sub", "deep")
	require.NoError(t, os.MkdirAll(sub, 0o750))
	session := connect(t, NewServer("test", WithRoot(sub)))

	// Act
	res := call(t, session, "init_project", map[string]any{})

	// Assert
	assert.True(t, res.IsError)
	assert.Contains(t, textOfResult(t, res), "ai-rulez.toml")
	assert.NoDirExists(t, filepath.Join(sub, ".ai-rulez"))
}
