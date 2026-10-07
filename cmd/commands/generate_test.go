package commands_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/cmd/commands"
)

func TestGenerateCommand(t *testing.T) {
	assert.NotNil(t, commands.GenerateCmd)
	assert.Equal(t, "generate [config-file]", commands.GenerateCmd.Use)
	assert.Contains(t, commands.GenerateCmd.Aliases, "gen")

	flags := commands.GenerateCmd.Flags()
	assert.Equal(t, "d", flags.Lookup("dry-run").Shorthand)
	assert.Equal(t, "i", flags.Lookup("gitignore").Shorthand)
	assert.Equal(t, "r", flags.Lookup("recursive").Shorthand)
	// Removed in v5: the deprecated aliases and no-op flags are gone.
	for _, name := range []string{"update-gitignore", "no-configure-cli-mcp", "skip-cli-mcp"} {
		assert.Nil(t, flags.Lookup(name), name)
	}
	assert.Nil(t, flags.ShorthandLookup("M"))
	assert.Nil(t, flags.ShorthandLookup("S"))
	assert.Equal(t, "p", flags.Lookup("profile").Shorthand)
	assert.NotNil(t, flags.Lookup("offline"))
	assert.Nil(t, flags.Lookup("no-fetch"))
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
