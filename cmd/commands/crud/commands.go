package crud

import (
	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/spf13/cobra"
)

var (
	ruleCmds      = CreateCRUDCommands(&RuleDescriptor)
	AddRuleCmd    = ruleCmds.Add
	UpdateRuleCmd = ruleCmds.Update
	DeleteRuleCmd = ruleCmds.Delete
	GetRuleCmd    = ruleCmds.Get
	ListRulesCmd  = ruleCmds.List

	sectionCmds      = CreateCRUDCommands(&SectionDescriptor)
	AddSectionCmd    = sectionCmds.Add
	UpdateSectionCmd = sectionCmds.Update
	DeleteSectionCmd = sectionCmds.Delete
	GetSectionCmd    = sectionCmds.Get
	ListSectionsCmd  = sectionCmds.List

	agentCmds      = CreateCRUDCommands(&AgentDescriptor)
	AddAgentCmd    = agentCmds.Add
	UpdateAgentCmd = agentCmds.Update
	DeleteAgentCmd = agentCmds.Delete
	GetAgentCmd    = agentCmds.Get
	ListAgentsCmd  = agentCmds.List

	outputCmds      = CreateCRUDCommands(&OutputDescriptor)
	AddOutputCmd    = outputCmds.Add
	UpdateOutputCmd = outputCmds.Update
	DeleteOutputCmd = outputCmds.Delete
	GetOutputCmd    = outputCmds.Get
	ListOutputsCmd  = outputCmds.List

	commandCmds      = CreateCRUDCommands(&CommandDescriptor)
	AddCommandCmd    = commandCmds.Add
	UpdateCommandCmd = commandCmds.Update
	DeleteCommandCmd = commandCmds.Delete
	GetCommandCmd    = commandCmds.Get
	ListCommandsCmd  = commandCmds.List

	mcpServerCmds      = CreateCRUDCommands(&MCPServerDescriptor)
	AddMCPServerCmd    = mcpServerCmds.Add
	UpdateMCPServerCmd = mcpServerCmds.Update
	DeleteMCPServerCmd = mcpServerCmds.Delete
	GetMCPServerCmd    = mcpServerCmds.Get
	ListMCPServersCmd  = mcpServerCmds.List

	AddIncludeCmd = &cobra.Command{
		Use:   "include [path-or-url]",
		Short: "Add a new include path to the configuration",
		Long:  `Adds a new file path or URL to the top-level includes list in your ai_rulez.yaml file.`,
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			pathOrURL := args[0]
			crud.AddToList("includes", pathOrURL)
		},
	}

	DeleteIncludeCmd = &cobra.Command{
		Use:   "include [path-or-url]",
		Short: "Delete an include path from the configuration",
		Long:  `Deletes a file path or URL from the top-level includes list in your ai_rulez.yaml file.`,
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			pathOrURL := args[0]
			crud.DeleteFromList("includes", pathOrURL)
		},
	}

	ListIncludesCmd = &cobra.Command{
		Use:     "includes",
		Short:   "List all include paths in the configuration",
		Long:    `Lists all file paths and URLs in the top-level includes list from your ai_rulez.yaml file.`,
		Aliases: []string{"include"},
		Run: func(cmd *cobra.Command, args []string) {
			crud.ListElements("includes")
		},
	}

	GetExtendsCmd = &cobra.Command{
		Use:   "extends",
		Short: "Get the extends property",
		Long:  `Retrieves and displays the value of the top-level extends property from your ai_rulez.yaml file.`,
		Run: func(cmd *cobra.Command, args []string) {
			crud.GetElement("extends", "")
		},
	}

	SetExtendsCmd = &cobra.Command{
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

	DeleteExtendsCmd = &cobra.Command{
		Use:   "extends",
		Short: "Delete the extends property",
		Long:  `Deletes the top-level extends property from your ai_rulez.yaml file.`,
		Run: func(cmd *cobra.Command, args []string) {
			crud.DeleteElement("extends", "")
		},
	}

	metaName        string
	metaVersion     string
	metaDescription string

	GetMetadataCmd = &cobra.Command{
		Use:   "metadata",
		Short: "Get the project metadata",
		Long:  `Retrieves and displays the project metadata (name, version, description) from your ai_rulez.yaml file.`,
		Run: func(cmd *cobra.Command, args []string) {
			crud.GetElement("metadata", "")
		},
	}

	SetMetadataCmd = &cobra.Command{
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
)

func init() {
	SetMetadataCmd.Flags().StringVar(&metaName, "name", "", "The name of your project")
	SetMetadataCmd.Flags().StringVar(&metaVersion, "version", "", "The project version (e.g., 1.0.0)")
	SetMetadataCmd.Flags().StringVar(&metaDescription, "description", "", "A brief description of the project")
}
