package crud

import (
	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/Goldziher/ai-rulez/internal/utils"
	"github.com/spf13/cobra"
)

var (
	sectionID       string
	sectionName     string
	sectionContent  string
	sectionPriority string
	sectionTargets  []string
)

// AddSectionCmd adds a new section to the configuration
var AddSectionCmd = &cobra.Command{
	Use:   "section [name]",
	Short: "Add a new section to the configuration",
	Long:  `Adds a new section to the sections list in your ai_rulez.yaml file.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		sectionName = args[0]

		priority, err := config.ParsePriority(sectionPriority)
		if err != nil {
			crud.FmtError("%v", err)
		}

		newSection := &config.Section{
			ID:       sectionID,
			Name:     sectionName,
			Content:  sectionContent,
			Priority: priority,
			Targets:  sectionTargets,
		}

		crud.AddElement("sections", newSection)
	},
}

// UpdateSectionCmd updates an existing section
var UpdateSectionCmd = &cobra.Command{
	Use:   "section [name]",
	Short: "Update an existing section",
	Long:  `Updates an existing section in your ai_rulez.yaml file.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]

		updates := make(map[string]interface{})
		if cmd.Flags().Changed("id") {
			updates["ID"] = sectionID
		}
		if cmd.Flags().Changed("content") {
			updates["Content"] = sectionContent
		}
		if cmd.Flags().Changed("priority") {
			priority, err := config.ParsePriority(sectionPriority)
			if err != nil {
				crud.FmtError("%v", err)
			}
			updates["Priority"] = priority
		}
		if cmd.Flags().Changed("target") {
			updates["Targets"] = sectionTargets
		}

		crud.UpdateElement("sections", name, updates)
	},
}

// DeleteSectionCmd deletes a section
var DeleteSectionCmd = &cobra.Command{
	Use:   "section [name]",
	Short: "Delete a section from the configuration",
	Long:  `Deletes a section by name from your ai_rulez.yaml file.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		crud.DeleteElement("sections", name)
	},
}

// GetSectionCmd gets a section
var GetSectionCmd = &cobra.Command{
	Use:   "section [name]",
	Short: "Get a section from the configuration",
	Long:  `Retrieves a section by name from your ai_rulez.yaml file.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		crud.GetElement("sections", name)
	},
}

// ListSectionsCmd lists all sections
var ListSectionsCmd = &cobra.Command{
	Use:     "sections",
	Short:   "List all sections in the configuration",
	Long:    `Lists all sections defined in your AI rules configuration.`,
	Aliases: []string{"section"},
	Run: func(cmd *cobra.Command, args []string) {
		crud.ListElements("sections")
	},
}

func init() {
	// Flags for Add command
	AddSectionCmd.Flags().StringVar(&sectionID, "id", "", "Optional unique identifier for the section")
	AddSectionCmd.Flags().StringVarP(&sectionContent, "content", "c", "", "Content of the section (required)")
	utils.LogIfErr(AddSectionCmd.MarkFlagRequired("content"))
	AddSectionCmd.Flags().StringVarP(&sectionPriority, "priority", "p", "medium", "Priority of the section (critical, high, medium, low, minimal)")
	AddSectionCmd.Flags().StringSliceVarP(&sectionTargets, "target", "t", []string{}, "Output target for this section (can be specified multiple times)")

	// Flags for Update command
	UpdateSectionCmd.Flags().StringVar(&sectionID, "id", "", "New unique identifier for the section")
	UpdateSectionCmd.Flags().StringVarP(&sectionContent, "content", "c", "", "New content for the section")
	UpdateSectionCmd.Flags().StringVarP(&sectionPriority, "priority", "p", "", "New priority for the section")
	UpdateSectionCmd.Flags().StringSliceVarP(&sectionTargets, "target", "t", []string{}, "New set of output targets for the section")
}
