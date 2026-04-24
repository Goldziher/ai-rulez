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
	cmd := commands.MigrateCmd
	assert.NotNil(t, cmd.Args)
}
