package commands_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/cmd/commands"
)

func TestCRUDCommands(t *testing.T) {
	// Verify CRUD parent commands exist and have subcommands
	assert.NotNil(t, commands.AddCmd)
	assert.Equal(t, "add", commands.AddCmd.Use)
	assert.True(t, commands.AddCmd.HasSubCommands())

	assert.NotNil(t, commands.UpdateCmd)
	assert.Equal(t, "update", commands.UpdateCmd.Use)
	assert.True(t, commands.UpdateCmd.HasSubCommands())

	assert.NotNil(t, commands.DeleteCmd)
	assert.Equal(t, "delete", commands.DeleteCmd.Use)
	assert.True(t, commands.DeleteCmd.HasSubCommands())
}
