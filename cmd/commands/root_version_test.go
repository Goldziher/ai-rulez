package commands

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRootVersionFlagPrintsTheVersion(t *testing.T) {
	// Arrange
	prev := Version
	Version = "9.8.7"
	var stdout bytes.Buffer
	RootCmd.SetOut(&stdout)
	RootCmd.SetErr(&stdout)
	RootCmd.SetArgs([]string{"--version"})
	t.Cleanup(func() {
		Version = prev
		RootCmd.Version = prev
		RootCmd.SetOut(nil)
		RootCmd.SetErr(nil)
		RootCmd.SetArgs(nil)
	})

	// Act
	err := Execute()

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "ai-rulez version 9.8.7\n", stdout.String())
}

// A Short is one line of the command list; flag names belong in Long and in --help.
func TestNoShortDescriptionNamesAFlag(t *testing.T) {
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		assert.NotContains(t, c.Short, "--", "Short of %q names a flag", c.CommandPath())
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(RootCmd)
}
