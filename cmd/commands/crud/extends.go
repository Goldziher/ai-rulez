package crud

import (
	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/spf13/cobra"
)

// GetExtendsCmd represents the command to get the extends property
var GetExtendsCmd = &cobra.Command{
	Use:   "extends",
	Short: "Get the extends property",
	Long:  `Retrieves and displays the value of the top-level extends property from your ai_rulez.yaml file.`,
	Run: func(cmd *cobra.Command, args []string) {
		crud.GetElement("extends", "") // Name is not used for singleton
	},
}

// SetExtendsCmd represents the command to set the extends property
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
		crud.UpdateElement("extends", "", updates) // Name is not used for singleton
	},
}

// DeleteExtendsCmd represents the command to delete the extends property
var DeleteExtendsCmd = &cobra.Command{
	Use:   "extends",
	Short: "Delete the extends property",
	Long:  `Deletes the top-level extends property from your ai_rulez.yaml file.`,
	Run: func(cmd *cobra.Command, args []string) {
		crud.DeleteElement("extends", "") // Name is not used for singleton
	},
}
