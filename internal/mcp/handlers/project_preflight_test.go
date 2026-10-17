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

const preflightMCPConfig = generateSharedConfig + `
[[hooks]]
event = "PreToolUse"
[[hooks.hooks]]
command = "echo \u001b[2Jhi --token=abcdef123456"
`

func TestGenerateOutputsHandler_ReturnsNewCommands(t *testing.T) {
	// Arrange
	testutil.IsolateHome(t)
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
	// The configured hook, and the SessionStart hook every Claude Code project gets by default.
	require.Len(t, commands, 2)
	lines := make([]string, len(commands))
	for i, c := range commands {
		lines[i], _ = c.(string)
	}
	line := lines[0]
	assert.Contains(t, line, `\x1b[2J`)
	assert.NotContains(t, line, "abcdef123456")
	assert.NotContains(t, line, "\x1b")
	assert.Contains(t, lines[1], "SessionStart: sh -c")
	assert.Contains(t, lines[1], "-> claude", "the summary names the harness the hook is written for")
	assert.NotContains(t, resultPayload(t, second), "new_commands", "an unchanged run reports nothing")
}
