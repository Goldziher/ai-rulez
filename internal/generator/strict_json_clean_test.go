package generator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClean_RestoresStrictJSONSettingsByteForByte(t *testing.T) {
	// Arrange: a hand-formatted settings.json with a one-line nested object.
	root := newSecretMCPRepo(t, `["claude"]`)
	appendConfig(t, root, "\n[permissions]\nallow = [\"Bash(git status)\"]\ndeny = [\"Read(./.env)\"]\n")
	original := "{\n  \"model\": \"opus\",\n  \"permissions\": { \"allow\": [\"Read\"] }\n}\n"
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".claude"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".claude", "settings.json"), []byte(original), 0o644))
	generateRepo(t, root)
	merged, err := os.ReadFile(filepath.Join(root, ".claude", "settings.json"))
	require.NoError(t, err)
	require.Contains(t, string(merged), "Bash(git status)")

	// Act
	cfg, err := config.LoadConfig(context.Background(), root)
	require.NoError(t, err)
	_, err = NewGenerator(cfg).Clean("default", CleanOptions{})

	// Assert
	require.NoError(t, err)
	restored, err := os.ReadFile(filepath.Join(root, ".claude", "settings.json"))
	require.NoError(t, err)
	assert.Equal(t, original, string(restored))
}
