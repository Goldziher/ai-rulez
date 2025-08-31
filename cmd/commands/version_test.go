package commands_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/cmd/commands"
)

func TestVersionCommand(t *testing.T) {
	assert.NotNil(t, commands.VersionCmd)
	assert.Equal(t, "version", commands.VersionCmd.Use)
}
