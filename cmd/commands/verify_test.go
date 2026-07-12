package commands_test

import (
	"testing"

	"github.com/Goldziher/ai-rulez/cmd/commands"
	"github.com/stretchr/testify/assert"
)

func TestVerifyCommand(t *testing.T) {
	assert.NotNil(t, commands.VerifyCmd.Flags().Lookup("plugin"))
	assert.NotNil(t, commands.VerifyCmd.Flags().Lookup("if-configured"))
	assert.Equal(t, "r", commands.VerifyCmd.Flags().Lookup("recursive").Shorthand)
}
