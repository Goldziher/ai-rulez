package commands_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/cmd/commands"
)

func TestInitCommand(t *testing.T) {
	assert.NotNil(t, commands.InitCmd)
	assert.Equal(t, "init [project-name]", commands.InitCmd.Use)

	// Check important flags exist
	flags := commands.InitCmd.Flags()
	assert.NotNil(t, flags.Lookup("preset"))
	assert.NotNil(t, flags.Lookup("all"))
	assert.NotNil(t, flags.Lookup("popular"))
	assert.NotNil(t, flags.Lookup("claude"))
	assert.NotNil(t, flags.Lookup("cursor"))
}
