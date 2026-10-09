package commands_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/cmd/commands"
)

func TestMCPCommand(t *testing.T) {
	assert.NotNil(t, commands.MCPCmd)
	assert.Equal(t, "mcp", commands.MCPCmd.Use)
	assert.Contains(t, commands.MCPCmd.Long, "Model Context Protocol")

	flags := commands.MCPCmd.Flags()

	// stdio is the only transport; flags for a websocket one would be dead.
	for _, dead := range []string{"transport", "address", "port"} {
		assert.Nil(t, flags.Lookup(dead), "--%s configures a transport that does not exist", dead)
	}
	assert.NotNil(t, flags.Lookup("serve-skills"))
}
