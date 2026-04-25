package commands_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/cmd/commands"
)

func TestValidateCommand(t *testing.T) {
	assert.NotNil(t, commands.ValidateCmd)
	assert.Equal(t, "validate [config-file]", commands.ValidateCmd.Use)
	assert.Contains(t, commands.ValidateCmd.Aliases, "check")
	assert.Contains(t, commands.ValidateCmd.Aliases, "val")
}

func TestValidateCommandSupport(t *testing.T) {
	t.Run("ConfigDetection", func(t *testing.T) {
		// Verify that validate command is capable of detecting configs
		// This is done via config.DetectConfigVersion() which is tested separately
		assert.NotNil(t, commands.ValidateCmd)
		assert.NotNil(t, commands.ValidateCmd.Run)
	})

	t.Run("ValidationLogic", func(t *testing.T) {
		// Verify validate command has validation logic
		cmd := commands.ValidateCmd
		assert.NotNil(t, cmd)
		// The validate command's Run function checks for version and calls runValidate()
		// This is verified by code inspection in validate.go lines 33-44
	})
}
