package commands_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/cmd/commands"
)

func TestRootCommand(t *testing.T) {
	assert.NotNil(t, commands.RootCmd)
	assert.Equal(t, "ai-rulez", commands.RootCmd.Use)
	assert.True(t, commands.RootCmd.HasSubCommands())

	// Verify key subcommands are registered
	cmdNames := make(map[string]bool)
	for _, cmd := range commands.RootCmd.Commands() {
		// Extract command name without arguments
		if cmd.Use != "" {
			parts := strings.Fields(cmd.Use)
			if len(parts) > 0 {
				cmdNames[parts[0]] = true
			}
		}
	}

	expectedCommands := []string{"generate", "validate", "init", "mcp", "version", "add", "update", "delete"}
	for _, expected := range expectedCommands {
		assert.True(t, cmdNames[expected], "Expected command %s to be registered", expected)
	}
}
