package crud

import (
	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/spf13/cobra"
)

// ListCommandsCmd represents the command to list all custom commands
var ListCommandsCmd = &cobra.Command{
	Use:     "commands",
	Short:   "List all configured custom commands",
	Long:    `Lists all the custom slash commands currently defined in your ai_rulez.yaml file.`,
	Aliases: []string{"command"},
	Run: func(cmd *cobra.Command, args []string) {
		crud.ListElements("commands")
	},
}
