package commands

import (
	"fmt"

	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/spf13/cobra"
)

var MigrateCmd = &cobra.Command{
	Use:   "migrate [version]",
	Short: "Migrate configuration to a newer format",
	Long:  "Migrate your ai-rulez configuration to a newer format version.",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		targetVersion := args[0]
		switch targetVersion {
		case "v4", "4", "4.0":
			fmt.Println("migrate v4 (YAML→TOML) is not yet implemented")
		default:
			logger.Error("Unsupported migration target", "version", targetVersion)
			fmt.Println("Supported targets: v4")
		}
	},
}

func init() {
	RootCmd.AddCommand(MigrateCmd)
}
