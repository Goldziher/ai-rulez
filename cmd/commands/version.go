package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

// VersionCmd represents the version command
var VersionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of ai-rulez",
	Long:  `Print the version number of ai-rulez CLI tool. The line is the same as --version prints, on stdout, and -q does not suppress it.`,
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "ai-rulez version %s\n", Version) //nolint:errcheck // nothing to do when stdout is closed
	},
}
