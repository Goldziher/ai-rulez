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

func TestRootCommandShorthands(t *testing.T) {
	flags := commands.RootCmd.PersistentFlags()

	assert.Equal(t, "C", flags.Lookup("config").Shorthand)
	assert.Equal(t, "V", flags.Lookup("verbose").Shorthand)
	assert.Equal(t, "D", flags.Lookup("debug").Shorthand)
	assert.Equal(t, "q", flags.Lookup("quiet").Shorthand)
	assert.Equal(t, "T", flags.Lookup("token").Shorthand)
}
