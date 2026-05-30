package commands_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/cmd/commands"
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
	assert.Equal(t, "M", flags.Lookup("no-configure-cli-mcp").Shorthand)
	assert.Equal(t, "S", flags.Lookup("skip-cli-mcp").Shorthand)
	assert.Equal(t, "p", flags.Lookup("profile").Shorthand)
	assert.Equal(t, "f", flags.Lookup("no-fetch").Shorthand)
	assert.Equal(t, "n", flags.Lookup("config-dir").Shorthand)
	assert.Equal(t, "e", flags.Lookup("env").Shorthand)
	assert.Equal(t, "E", flags.Lookup("env-file").Shorthand)
}

func TestGenerateCommand_ProfileFlag(t *testing.T) {
	cmd := commands.GenerateCmd
	profileFlag := cmd.Flags().Lookup("profile")
	assert.NotNil(t, profileFlag, "Profile flag should exist")
}
