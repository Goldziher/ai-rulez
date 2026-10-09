package commands_test

import (
	"testing"

	"github.com/Goldziher/ai-rulez/v5/cmd/commands"
	"github.com/stretchr/testify/assert"
)

func TestVerifyCommand(t *testing.T) {
	assert.NotNil(t, commands.VerifyCmd.Flags().Lookup("plugin"))
	assert.NotNil(t, commands.VerifyCmd.Flags().Lookup("if-configured"))
	assert.Empty(t, commands.VerifyCmd.Flags().Lookup("recursive").Shorthand)
}
