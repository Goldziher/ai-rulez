package crud

import (
	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/spf13/cobra"
)

// AddIncludeCmd represents the command to add a new include path
var AddIncludeCmd = &cobra.Command{
	Use:   "include [path-or-url]",
	Short: "Add a new include path to the configuration",
	Long:  `Adds a new file path or URL to the top-level includes list in your ai_rulez.yaml file.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		pathOrURL := args[0]
		crud.AddToList("includes", pathOrURL)
	},
}

// DeleteIncludeCmd represents the command to delete an include path
var DeleteIncludeCmd = &cobra.Command{
	Use:   "include [path-or-url]",
	Short: "Delete an include path from the configuration",
	Long:  `Deletes a file path or URL from the top-level includes list in your ai_rulez.yaml file.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		pathOrURL := args[0]
		crud.DeleteFromList("includes", pathOrURL)
	},
}

// ListIncludesCmd represents the command to list all include paths
var ListIncludesCmd = &cobra.Command{
	Use:     "includes",
	Short:   "List all include paths in the configuration",
	Long:    `Lists all file paths and URLs in the top-level includes list from your ai_rulez.yaml file.`,
	Aliases: []string{"include"},
	Run: func(cmd *cobra.Command, args []string) {
		crud.ListElements("includes")
	},
}
