package crud

import (
	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/Goldziher/ai-rulez/internal/errutils"
	"github.com/spf13/cobra"
)

var (
	agentID            string
	agentName          string
	agentDescription   string
	agentPriority      string
	agentTools         []string
	agentSystemPrompt  string
	agentTemplateType  string
	agentTemplateValue string
	agentTargets       []string
)

var AddAgentCmd = &cobra.Command{
	Use:   "agent [name]",
	Short: "Add a new agent to the configuration",
	Long:  `Adds a new agent to the agents list in your ai_rulez.yaml file.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		agentName = args[0]

		priority, err := config.ParsePriority(agentPriority)
		if err != nil {
			crud.FmtError("%v", err)
		}

		newAgent := &config.Agent{
			ID:           agentID,
			Name:         agentName,
			Description:  agentDescription,
			Priority:     priority,
			Tools:        agentTools,
			SystemPrompt: agentSystemPrompt,
			Template:     crud.CreateTemplateConfig(agentTemplateType, agentTemplateValue),
			Targets:      agentTargets,
		}

		crud.AddElement("agents", newAgent)
	},
}

var UpdateAgentCmd = &cobra.Command{
	Use:   "agent [name]",
	Short: "Update an existing agent",
	Long:  `Updates an existing agent in your ai_rulez.yaml file.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]

		updates := make(map[string]interface{})
		if cmd.Flags().Changed("id") {
			updates["ID"] = agentID
		}
		if cmd.Flags().Changed("description") {
			updates["Description"] = agentDescription
		}
		if cmd.Flags().Changed("priority") {
			priority, err := config.ParsePriority(agentPriority)
			if err != nil {
				crud.FmtError("%v", err)
			}
			updates["Priority"] = priority
		}
		if cmd.Flags().Changed("tools") {
			updates["Tools"] = agentTools
		}
		if cmd.Flags().Changed("system-prompt") {
			updates["SystemPrompt"] = agentSystemPrompt
		}
		if cmd.Flags().Changed("template-type") || cmd.Flags().Changed("template-value") {
			updates["Template"] = crud.CreateTemplateConfig(agentTemplateType, agentTemplateValue)
		}
		if cmd.Flags().Changed("target") {
			updates["Targets"] = agentTargets
		}

		crud.UpdateElement("agents", name, updates)
	},
}

var DeleteAgentCmd = &cobra.Command{
	Use:   "agent [name]",
	Short: "Delete an agent from the configuration",
	Long:  `Deletes an agent by name from your ai_rulez.yaml file.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		crud.DeleteElement("agents", name)
	},
}

var GetAgentCmd = &cobra.Command{
	Use:   "agent [name]",
	Short: "Get an agent from the configuration",
	Long:  `Retrieves an agent by name from your ai_rulez.yaml file.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		crud.GetElement("agents", name)
	},
}

var ListAgentsCmd = &cobra.Command{
	Use:     "agents",
	Short:   "List all agents in the configuration",
	Long:    `Lists all agents defined in your AI rules configuration.`,
	Aliases: []string{"agent"},
	Run: func(cmd *cobra.Command, args []string) {
		crud.ListElements("agents")
	},
}

func init() {
	AddAgentCmd.Flags().StringVar(&agentID, "id", "", "Optional unique identifier for the agent")
	AddAgentCmd.Flags().StringVarP(&agentDescription, "description", "d", "", "Description of the agent (required)")
	errutils.LogIfErr(AddAgentCmd.MarkFlagRequired("description"))
	AddAgentCmd.Flags().StringVarP(&agentPriority, "priority", "p", "medium", "Priority of the agent (critical, high, medium, low, minimal)")
	AddAgentCmd.Flags().StringSliceVar(&agentTools, "tools", []string{}, "Comma-separated list of tools the agent can use")
	AddAgentCmd.Flags().StringVarP(&agentSystemPrompt, "system-prompt", "s", "", "System prompt for the agent")
	AddAgentCmd.Flags().StringVar(&agentTemplateType, "template-type", "", "Template type for the agent: builtin, file, or inline")
	AddAgentCmd.Flags().StringVar(&agentTemplateValue, "template-value", "", "Template value (name, path, or content)")
	AddAgentCmd.Flags().StringSliceVarP(&agentTargets, "target", "t", []string{}, "Output target for this agent (can be specified multiple times)")

	UpdateAgentCmd.Flags().StringVar(&agentID, "id", "", "New unique identifier for the agent")
	UpdateAgentCmd.Flags().StringVarP(&agentDescription, "description", "d", "", "New description for the agent")
	UpdateAgentCmd.Flags().StringVarP(&agentPriority, "priority", "p", "", "New priority for the agent")
	UpdateAgentCmd.Flags().StringSliceVar(&agentTools, "tools", []string{}, "New comma-separated list of tools")
	UpdateAgentCmd.Flags().StringVarP(&agentSystemPrompt, "system-prompt", "s", "", "New system prompt for the agent")
	UpdateAgentCmd.Flags().StringVar(&agentTemplateType, "template-type", "", "New template type for the agent")
	UpdateAgentCmd.Flags().StringVar(&agentTemplateValue, "template-value", "", "New template value for the agent")
	UpdateAgentCmd.Flags().StringSliceVarP(&agentTargets, "target", "t", []string{}, "New set of output targets for the agent")
}
