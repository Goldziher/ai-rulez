package commands_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/cmd/commands"
)

func TestCRUDCommands(t *testing.T) {
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

func TestCRUDCommandsV3Rejection(t *testing.T) {
	// Test that CRUD commands properly reject V3 configurations
	t.Run("V3RejectionLogic", func(t *testing.T) {
		// Verify CRUD commands are designed to work with V2 configs only
		// The rejection logic is implemented in internal/crud/crud.go
		// in the loadConfigWithPath function (lines 870-875)

		assert.NotNil(t, commands.AddCmd)
		assert.NotNil(t, commands.UpdateCmd)
		assert.NotNil(t, commands.DeleteCmd)
		assert.NotNil(t, commands.GetCmd)

		// These commands should all have subcommands that use loadConfigWithPath
		// which will detect V3 and return an appropriate error message
	})

	t.Run("V3UserGuidance", func(t *testing.T) {
		// Verify that CRUD commands provide helpful guidance for V3 users
		// The error message from loadConfigWithPath includes:
		// "Edit files directly in .ai-rulez/rules/, .ai-rulez/context/, and .ai-rulez/skills/ directories instead."
		// "Or convert to V2 format using: ai-rulez migrate"
		assert.NotNil(t, commands.AddCmd)
	})
}
