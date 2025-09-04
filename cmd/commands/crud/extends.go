package crud

import (
	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/spf13/cobra"
)

var GetExtendsCmd = &cobra.Command{
	Use:   "extends",
	Short: "Get the extends property",
	Long:  `Retrieves and displays the value of the top-level extends property from your ai_rulez.yaml file.`,
	Run: func(cmd *cobra.Command, args []string) {
		crud.GetElement("extends", "")
	},
}

var SetExtendsCmd = &cobra.Command{
	Use:   "extends [path-or-url]",
	Short: "Set the extends property",
	Long:  `Sets or updates the top-level extends property in your ai_rulez.yaml file.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		pathOrURL := args[0]
		updates := map[string]interface{}{
			"Extends": pathOrURL,
		}
		crud.UpdateElement("extends", "", updates)
	},
}

var DeleteExtendsCmd = &cobra.Command{
	Use:   "extends",
	Short: "Delete the extends property",
	Long:  `Deletes the top-level extends property from your ai_rulez.yaml file.`,
	Run: func(cmd *cobra.Command, args []string) {
		crud.DeleteElement("extends", "")
	},
}
