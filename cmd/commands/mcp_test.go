package commands_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/cmd/commands"
)

func TestMCPCommand(t *testing.T) {
	assert.NotNil(t, commands.MCPCmd)
	assert.Equal(t, "mcp", commands.MCPCmd.Use)
	assert.Contains(t, commands.MCPCmd.Long, "Model Context Protocol")

	flags := commands.MCPCmd.Flags()

	transportFlag := flags.Lookup("transport")
	assert.NotNil(t, transportFlag)
	assert.True(t, transportFlag.Hidden)
}
