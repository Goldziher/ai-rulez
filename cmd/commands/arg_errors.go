package commands

import (
	"fmt"
	"strings"

	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

// wrappedArgs records the commands whose argument validator already reports
// usage errors, so wrapping twice (a test calls Execute repeatedly) is a no-op.
var wrappedArgs = map[*cobra.Command]bool{}

// explainArgErrors makes every command say what it expects when its arguments
// are wrong. cobra's own text ("accepts 1 arg(s), received 0") names neither the
// command nor its usage, and an unknown subcommand gets no suggestion.
func explainArgErrors(root *cobra.Command) {
	for _, c := range root.Commands() {
		explainArgErrors(c)
	}
	if root.Args == nil || wrappedArgs[root] {
		return
	}
	wrappedArgs[root] = true
	original := root.Args
	root.Args = func(cmd *cobra.Command, args []string) error {
		err := original(cmd, args)
		if err == nil {
			return nil
		}
		text := err.Error()
		switch {
		case strings.HasPrefix(text, "unknown command ") && cmd.HasSubCommands() && len(args) > 0:
			return unknownSubcommandError(cmd, args[0])
		case strings.HasPrefix(text, "unknown command ") && len(args) > 0:
			return unexpectedArgError(cmd, args[0])
		case strings.HasPrefix(text, "accepts ") || strings.HasPrefix(text, "requires at least "):
			return argCountError(cmd, args)
		}
		return err
	}
}

// argCountError is the usage error of a command given too few or too many
// arguments: what is wrong, the usage line, and where the details are.
func argCountError(cmd *cobra.Command, args []string) error {
	problem := "missing argument"
	if len(args) > 0 {
		problem = fmt.Sprintf("wrong number of arguments (got %d)", len(args))
	}
	return fmt.Errorf("%s for %q\n\nUsage:\n  %s\n\nRun \"%s --help\" for details and examples", //nolint:err113 // a user-facing usage message
		problem, cmd.CommandPath(), cmd.UseLine(), cmd.CommandPath())
}

// unexpectedArgError is the usage error of a command that takes no arguments
// given one. Before v5 many commands took a config path; it is -C now.
func unexpectedArgError(cmd *cobra.Command, arg string) error {
	return oops.Hint("the project is chosen with -C <path> (or --config-dir), not an argument: ai-rulez -C "+arg+" "+strings.TrimPrefix(cmd.CommandPath(), RootCmd.Name()+" ")).
		Errorf("unexpected argument %q for %q\n\nUsage:\n  %s", arg, cmd.CommandPath(), cmd.UseLine())
}
