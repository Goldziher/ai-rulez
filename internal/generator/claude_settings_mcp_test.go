package generator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerate_ClaudeSettingsHoldNoMCPServers(t *testing.T) {
	// Arrange: servers plus a permission rule, so settings.json is written.
	root := newSecretMCPRepo(t, `["claude"]`)
	appendConfig(t, root, "\n[permissions]\nallow = [\"Bash(git status)\"]\n")

	// Act
	generateRepo(t, root)

	// Assert: Claude Code reads servers from .mcp.json; settings.json carries none.
	settings, err := os.ReadFile(filepath.Join(root, ".claude", "settings.json"))
	require.NoError(t, err)
	assert.NotContains(t, string(settings), "mcpServers")
	assert.NotContains(t, string(settings), leakToken)
	mcp, err := os.ReadFile(filepath.Join(root, ".mcp.json"))
	require.NoError(t, err)
	assert.Contains(t, string(mcp), "github")
}

func TestGenerate_PreviousMCPServersEntryInSettingsIsTakenBack(t *testing.T) {
	// Arrange: an earlier version wrote the server into settings.json and recorded it.
	root := newSecretMCPRepo(t, `["claude"]`)
	appendConfig(t, root, "\n[permissions]\nallow = [\"Bash(git status)\"]\n")
	entry := map[string]any{"command": "npx", "args": []string{"-y", "gh-mcp", root}}
	entryJSON, err := json.Marshal(entry)
	require.NoError(t, err)
	settings := "{\n  \"mcpServers\": {\"github\": " + string(entryJSON) + "},\n  \"model\": \"opus\"\n}\n"
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".claude"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".claude", "settings.json"), []byte(settings), 0o644))
	manifest := `{"version":"1","files":[],"merged":{".claude/settings.json":[{"path":["mcpServers","github"],"sum":"` +
		jsonmerge.Digest(entry) + `"}]}}`
	require.NoError(t, os.WriteFile(filepath.Join(root, ".ai-rulez", ".generated-manifest.local.json"), []byte(manifest), 0o644))

	// Act
	generateRepo(t, root)

	// Assert
	got, err := os.ReadFile(filepath.Join(root, ".claude", "settings.json"))
	require.NoError(t, err)
	assert.NotContains(t, string(got), "mcpServers")
	assert.Contains(t, string(got), "opus")
}
