package commands

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// pflag reads a backquoted word in a usage string as the flag's value name, so
// "(see `ai-rulez roles list`)" renders as "--role ai-rulez roles list".
func TestFlagUsageHasNoBackquotes(t *testing.T) {
	// Arrange
	var offenders []string
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		// Act
		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			if strings.Contains(f.Usage, "`") {
				offenders = append(offenders, cmd.CommandPath()+" --"+f.Name)
			}
		})
		for _, sub := range cmd.Commands() {
			walk(sub)
		}
	}
	walk(RootCmd)

	// Assert
	if len(offenders) > 0 {
		t.Fatalf("flag usage strings must not contain backquotes (pflag turns them into value names): %v", offenders)
	}
}
