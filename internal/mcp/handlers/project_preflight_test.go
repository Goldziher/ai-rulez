package handlers

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const preflightMCPConfig = generateSharedConfig + `
[[hooks]]
event = "PreToolUse"
[[hooks.hooks]]
command = "echo \u001b[2Jhi --token=abcdef123456"
`

func TestGenerateOutputsHandler_ReturnsNewCommands(t *testing.T) {
	// Arrange
	t.Setenv("HOME", t.TempDir())
	t.Setenv("AI_RULEZ_ACK_COMMANDS", "")
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(cfgDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(preflightMCPConfig), 0o600))
	request := func() *ToolRequest { return newRequestWithArgs(map[string]any{"working_directory": dir}) }

	// Act
	first, err := GenerateOutputsHandler(context.Background(), request())
	require.NoError(t, err)
	second, err := GenerateOutputsHandler(context.Background(), request())
	require.NoError(t, err)

	// Assert
	require.False(t, first.IsError, textOf(t, first))
	commands, ok := resultPayload(t, first)["new_commands"].([]any)
	require.True(t, ok, "the first run reports the commands it wrote")
	require.Len(t, commands, 1)
	line, _ := commands[0].(string)
	assert.Contains(t, line, `\x1b[2J`)
	assert.NotContains(t, line, "abcdef123456")
	assert.NotContains(t, line, "\x1b")
	assert.NotContains(t, resultPayload(t, second), "new_commands", "an unchanged run reports nothing")
}
