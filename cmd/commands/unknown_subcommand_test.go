package commands

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func walkCommands(c *cobra.Command, visit func(*cobra.Command)) {
	visit(c)
	for _, sub := range c.Commands() {
		walkCommands(sub, visit)
	}
}

func TestEveryGroupCommandRejectsAnUnknownSubcommand(t *testing.T) {
	// Arrange
	requireKnownSubcommands(RootCmd)
	var groups []*cobra.Command
	walkCommands(RootCmd, func(c *cobra.Command) {
		if c != RootCmd && c.HasSubCommands() && c.Name() != "help" {
			groups = append(groups, c)
		}
	})
	require.NotEmpty(t, groups)

	for _, g := range groups {
		t.Run(g.CommandPath(), func(t *testing.T) {
			if g.CommandPath() == "ai-rulez telemetry report" {
				// telemetry report takes the usage log as a positional argument next to its
				// "evals" subcommand: a path that does not exist is reported by the report itself.
				return
			}
			if g.Name() == "review" {
				// review takes item selectors as positional arguments next to its subcommands: a
				// selector that matches no item is reported by the review itself ("no item matches").
				return
			}
			if g.Run != nil {
				// A group with an action of its own validates its arguments itself.
				require.NotNil(t, g.Args, "%s accepts any argument", g.CommandPath())
				assert.Error(t, g.Args(g, []string{"bogus-subcommand"}))
				return
			}
			// Act
			err := g.RunE(g, []string{"bogus-subcommand"})

			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), `unknown command "bogus-subcommand"`)
			assert.Contains(t, err.Error(), g.CommandPath())
		})
	}
}

func TestGroupCommandWithoutArgumentsShowsHelp(t *testing.T) {
	// Arrange
	requireKnownSubcommands(RootCmd)
	cmd, _, err := RootCmd.Find([]string{"telemetry"})
	require.NoError(t, err)
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	t.Cleanup(func() { cmd.SetOut(nil) })

	// Act
	err = cmd.RunE(cmd, nil)

	// Assert
	require.NoError(t, err)
	assert.Contains(t, out.String(), "Usage:")
}

func TestUnknownSubcommandSuggestsAClosestMatch(t *testing.T) {
	// Arrange
	requireKnownSubcommands(RootCmd)
	cmd, _, err := RootCmd.Find([]string{"telemetry"})
	require.NoError(t, err)

	// Act
	err = cmd.RunE(cmd, []string{"flus"})

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Did you mean this?")
	assert.Contains(t, err.Error(), "flush")
}
