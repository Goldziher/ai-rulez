package crud

import (
	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/spf13/cobra"
)

// GetCommandCmd represents the command to get a specific custom command
var GetCommandCmd = &cobra.Command{
	Use:   "command [name]",
	Short: "Get details of a specific custom command",
	Long:  `Retrieves and displays the configuration of a specific custom command from your ai_rulez.yaml file.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		crud.GetElement("commands", name)
	},
}
