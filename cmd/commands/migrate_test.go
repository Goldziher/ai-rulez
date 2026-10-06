package commands_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/cmd/commands"
)

func TestMigrateCommand(t *testing.T) {
	assert.NotNil(t, commands.MigrateCmd)
	assert.Equal(t, "migrate [version]", commands.MigrateCmd.Use)
	assert.NotNil(t, commands.MigrateCmd.RunE)
}

func TestMigrateCommand_RequiresVersionArg(t *testing.T) {
	cmd := commands.MigrateCmd
	assert.NotNil(t, cmd.Args)
}

func TestMigrateCommand_UnknownTargetIsAnError(t *testing.T) {
	err := commands.MigrateCmd.RunE(commands.MigrateCmd, []string{"v9"})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), `unsupported migration target "v9"`)
}
