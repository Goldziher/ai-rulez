package commands_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/cmd/commands"
)

func TestGenerateCommand(t *testing.T) {
	assert.NotNil(t, commands.GenerateCmd)
	assert.Equal(t, "generate [config-file]", commands.GenerateCmd.Use)
	assert.Contains(t, commands.GenerateCmd.Aliases, "gen")

	flags := commands.GenerateCmd.Flags()
	assert.Equal(t, "d", flags.Lookup("dry-run").Shorthand)
	assert.Equal(t, "i", flags.Lookup("gitignore").Shorthand)
	assert.NotNil(t, flags.Lookup("update-gitignore"))
	assert.True(t, flags.Lookup("update-gitignore").Hidden)
	assert.Equal(t, "r", flags.Lookup("recursive").Shorthand)
	// Removed behavior: the flags stay accepted but are hidden, deprecated no-ops.
	for name, short := range map[string]string{"no-configure-cli-mcp": "M", "skip-cli-mcp": "S"} {
		flag := flags.Lookup(name)
		require.NotNil(t, flag, name)
		assert.Equal(t, short, flag.Shorthand, name)
		assert.True(t, flag.Hidden, name)
		assert.NotEmpty(t, flag.Deprecated, name)
	}
	assert.Equal(t, "p", flags.Lookup("profile").Shorthand)
	assert.Equal(t, "f", flags.Lookup("no-fetch").Shorthand)
	assert.Equal(t, "n", flags.Lookup("config-dir").Shorthand)
	assert.Equal(t, "e", flags.Lookup("env").Shorthand)
	assert.Equal(t, "E", flags.Lookup("env-file").Shorthand)
	assert.NotNil(t, flags.Lookup("if-configured"))
}

func TestGenerateCommand_ProfileFlag(t *testing.T) {
	cmd := commands.GenerateCmd
	profileFlag := cmd.Flags().Lookup("profile")
	assert.NotNil(t, profileFlag, "Profile flag should exist")
}
