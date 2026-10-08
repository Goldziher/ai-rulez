package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/jsondoc"
)

// VersionCmd represents the version command
var VersionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of ai-rulez",
	Long:  `Print the version number of ai-rulez CLI tool. The line is the same as --version prints, on stdout, and -q does not suppress it.`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		if versionJSON {
			return fail(jsondoc.Write(cmd.OutOrStdout(), map[string]any{"version": Version}))
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "ai-rulez version %s\n", Version) //nolint:errcheck // nothing to do when stdout is closed
		return nil
	},
}

var versionJSON bool

func init() { addJSONFormat(VersionCmd.Flags(), &versionJSON, "") }
