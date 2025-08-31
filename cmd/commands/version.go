package commands

import (
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/spf13/cobra"
)

// VersionCmd represents the version command
var VersionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of ai-rulez",
	Long:  `Print the version number of ai-rulez CLI tool.`,
	Run: func(cmd *cobra.Command, args []string) {
		logger.Info("ai-rulez version", "version", Version)
	},
}
