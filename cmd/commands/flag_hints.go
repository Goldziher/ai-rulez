package commands

import (
	"fmt"
	"strings"

	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

// shorthandsLeft lists what is left of the single-letter flags.
const shorthandsLeft = "-C -D -T -n -o -q -y"

// flagErrorWithHint is the root's flag error function: cobra's "unknown flag"
// error for a spelling that v5 removed gets a hint that names the replacement,
// instead of leaving a script author to read --help. Nothing else is changed.
func flagErrorWithHint(cmd *cobra.Command, err error) error {
	hint := removedFlagHint(cmd, err.Error())
	if hint == "" {
		return err
	}
	return oops.Hint(hint).Wrap(err)
}

// removedFlagHint returns the migration hint for an unknown-flag message, or "".
func removedFlagHint(cmd *cobra.Command, msg string) string {
	const (
		longPrefix  = "unknown flag: --"
		shortPrefix = "unknown shorthand flag: '"
	)
	switch {
	case strings.HasPrefix(msg, longPrefix):
		return removedLongFlagHint(cmd, strings.TrimPrefix(msg, longPrefix))
	case strings.HasPrefix(msg, shortPrefix):
		letter := strings.TrimPrefix(msg, shortPrefix)[:1]
		if letter == "n" {
			return "-n is --dry-run on the commands that have it; the configuration directory is the global --config-dir"
		}
		return fmt.Sprintf("short flags were removed except %s: spell the flag out (see %s --help)", shorthandsLeft, cmd.CommandPath())
	}
	return ""
}

func removedLongFlagHint(cmd *cobra.Command, name string) string {
	switch {
	case name == "out":
		if cmd.Flags().Lookup("output-dir") != nil {
			return "--out is --output-dir (a directory)"
		}
		return "--out is --output (a file)"
	case name == "strict" && cmd.Flags().Lookup("strict-config") != nil:
		return "--strict is --strict-config here (fail on configuration keys that have no effect)"
	case name == "strict" && cmd.Flags().Lookup("refuse-findings") != nil:
		return "--strict is --refuse-findings here (fail when the security scan refuses a served skill)"
	case isPolicyFlag(name):
		return "the --policy-* flags exist on the commands that evaluate the organization policy (generate, validate, lock, ...); other commands honor AI_RULEZ_POLICY and the managed policy path"
	}
	return ""
}
