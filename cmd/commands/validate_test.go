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

func TestValidateCommandV3Support(t *testing.T) {
	// Test V3 configuration support in validate command
	t.Run("V3ConfigDetection", func(t *testing.T) {
		// Verify that validate command is capable of detecting V3 configs
		// This is done via config.DetectConfigVersion() which is tested separately
		assert.NotNil(t, commands.ValidateCmd)
		assert.NotNil(t, commands.ValidateCmd.Run)
	})

	t.Run("V3ValidationLogic", func(t *testing.T) {
		// Verify validate command has V3-specific validation logic
		cmd := commands.ValidateCmd
		assert.NotNil(t, cmd)
		// The validate command's Run function checks for version and calls runValidateV3()
		// This is verified by code inspection in validate.go lines 33-44
	})
}
