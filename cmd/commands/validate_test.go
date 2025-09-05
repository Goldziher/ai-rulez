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
