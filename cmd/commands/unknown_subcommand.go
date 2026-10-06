package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// requireKnownSubcommands makes every group command (one with subcommands and no
// action of its own) reject an unknown subcommand. Without it cobra prints the
// group's help and exits 0 for `ai-rulez telemetry bogus`, which a script cannot
// tell from success. With no argument the help still shows.
func requireKnownSubcommands(root *cobra.Command) {
	for _, c := range root.Commands() {
		requireKnownSubcommands(c)
	}
	if !root.HasSubCommands() || root.Runnable() {
		return
	}
	root.Args = cobra.ArbitraryArgs
	root.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}
		return unknownSubcommandError(cmd, args[0])
	}
}

func unknownSubcommandError(cmd *cobra.Command, arg string) error {
	msg := fmt.Sprintf("unknown command %q for %q", arg, cmd.CommandPath())
	if suggestions := cmd.SuggestionsFor(arg); len(suggestions) > 0 {
		msg += "\n\nDid you mean this?\n\t" + strings.Join(suggestions, "\n\t")
	}
	return fmt.Errorf("%s", msg) //nolint:err113 // a user-facing usage message
}
