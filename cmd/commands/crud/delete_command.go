package crud

import (
	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/spf13/cobra"
)

// DeleteCommandCmd represents the command to delete a custom command
var DeleteCommandCmd = &cobra.Command{
	Use:     "command [name]",
	Short:   "Delete a custom command from the configuration",
	Long:    `Deletes a custom slash command from the commands list in your ai_rulez.yaml file.`,
	Aliases: []string{"cmd"},
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		crud.DeleteElement("commands", name)
	},
}
