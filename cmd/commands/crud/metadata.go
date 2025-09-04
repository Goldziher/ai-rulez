package crud

import (
	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/spf13/cobra"
)

var (
	metaName        string
	metaVersion     string
	metaDescription string
)

var GetMetadataCmd = &cobra.Command{
	Use:   "metadata",
	Short: "Get the project metadata",
	Long:  `Retrieves and displays the project metadata (name, version, description) from your ai_rulez.yaml file.`,
	Run: func(cmd *cobra.Command, args []string) {
		crud.GetElement("metadata", "")
	},
}

var SetMetadataCmd = &cobra.Command{
	Use:   "metadata",
	Short: "Set the project metadata",
	Long:  `Sets or updates the project metadata (name, version, description) in your ai_rulez.yaml file.`,
	Run: func(cmd *cobra.Command, args []string) {
		updates := make(map[string]interface{})
		if cmd.Flags().Changed("name") {
			updates["Name"] = metaName
		}
		if cmd.Flags().Changed("version") {
			updates["Version"] = metaVersion
		}
		if cmd.Flags().Changed("description") {
			updates["Description"] = metaDescription
		}

		crud.UpdateElement("metadata", "", updates)
	},
}

func init() {
	SetMetadataCmd.Flags().StringVar(&metaName, "name", "", "The name of your project")
	SetMetadataCmd.Flags().StringVar(&metaVersion, "version", "", "The project version (e.g., 1.0.0)")
	SetMetadataCmd.Flags().StringVar(&metaDescription, "description", "", "A brief description of the project")
}
