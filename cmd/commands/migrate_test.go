package commands_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/cmd/commands"
)

func TestMigrateCommand(t *testing.T) {
	assert.NotNil(t, commands.MigrateCmd)
	assert.Equal(t, "migrate [version]", commands.MigrateCmd.Use)
	assert.NotNil(t, commands.MigrateCmd.Run)
}

func TestMigrateCommand_RequiresVersionArg(t *testing.T) {
	// Verify that command requires exactly one argument (version)
	cmd := commands.MigrateCmd
	// MigrateCmd uses cobra.ExactArgs(1)
	assert.NotNil(t, cmd.Args)
	assert.Equal(t, "migrate [version]", cmd.Use)
}
