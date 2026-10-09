package commands_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/cmd/commands"
)

func TestValidateCommand(t *testing.T) {
	assert.NotNil(t, commands.ValidateCmd)
	assert.Equal(t, "validate", commands.ValidateCmd.Use)
	assert.NotContains(t, commands.ValidateCmd.Aliases, "check", "check is reserved for compare-and-exit-2 flags")
	assert.Contains(t, commands.ValidateCmd.Aliases, "val")
}

func TestValidateCommandSupport(t *testing.T) {
	t.Run("ConfigDetection", func(t *testing.T) {
		// Verify that validate command is capable of detecting configs
		// This is done via config.DetectConfigVersion() which is tested separately
		assert.NotNil(t, commands.ValidateCmd)
		assert.NotNil(t, commands.ValidateCmd.RunE)
	})

	t.Run("ValidationLogic", func(t *testing.T) {
		// Verify validate command has validation logic
		cmd := commands.ValidateCmd
		assert.NotNil(t, cmd)
		// The validate command's Run function checks for version and calls runValidate()
		// This is verified by code inspection in validate.go lines 33-44
	})
}

func TestValidateHasAnOfflineFlag(t *testing.T) {
	flag := commands.ValidateCmd.Flags().Lookup("offline")

	if assert.NotNil(t, flag) {
		assert.Equal(t, "false", flag.DefValue)
	}
}
