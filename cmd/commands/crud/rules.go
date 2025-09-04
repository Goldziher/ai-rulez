package crud

import (
	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/Goldziher/ai-rulez/internal/errutils"
	"github.com/spf13/cobra"
)

var (
	ruleID       string
	ruleName     string
	ruleContent  string
	rulePriority string
	ruleTargets  []string
)

// AddRuleCmd adds a new rule to the configuration
var AddRuleCmd = &cobra.Command{
	Use:   "rule [name]",
	Short: "Add a new rule to the configuration",
	Long:  `Adds a new rule to the rules list in your ai_rulez.yaml file.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ruleName = args[0]

		priority, err := config.ParsePriority(rulePriority)
		if err != nil {
			crud.FmtError("%v", err)
		}

		newRule := &config.Rule{
			ID:       ruleID,
			Name:     ruleName,
			Content:  ruleContent,
			Priority: priority,
			Targets:  ruleTargets,
		}

		crud.AddElement("rules", newRule)
	},
}

// UpdateRuleCmd updates an existing rule
var UpdateRuleCmd = &cobra.Command{
	Use:   "rule [name]",
	Short: "Update an existing rule",
	Long:  `Updates an existing rule in your ai_rulez.yaml file.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]

		updates := make(map[string]interface{})
		if cmd.Flags().Changed("id") {
			updates["ID"] = ruleID
		}
		if cmd.Flags().Changed("content") {
			updates["Content"] = ruleContent
		}
		if cmd.Flags().Changed("priority") {
			priority, err := config.ParsePriority(rulePriority)
			if err != nil {
				crud.FmtError("%v", err)
			}
			updates["Priority"] = priority
		}
		if cmd.Flags().Changed("target") {
			updates["Targets"] = ruleTargets
		}

		crud.UpdateElement("rules", name, updates)
	},
}

// DeleteRuleCmd deletes a rule
var DeleteRuleCmd = &cobra.Command{
	Use:   "rule [name]",
	Short: "Delete a rule from the configuration",
	Long:  `Deletes a rule by name from your ai_rulez.yaml file.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		crud.DeleteElement("rules", name)
	},
}

// GetRuleCmd gets a rule
var GetRuleCmd = &cobra.Command{
	Use:   "rule [name]",
	Short: "Get a rule from the configuration",
	Long:  `Retrieves a rule by name from your ai_rulez.yaml file.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		crud.GetElement("rules", name)
	},
}

// ListRulesCmd lists all rules
var ListRulesCmd = &cobra.Command{
	Use:     "rules",
	Short:   "List all rules in the configuration",
	Long:    `Lists all rules defined in your AI rules configuration.`,
	Aliases: []string{"rule"},
	Run: func(cmd *cobra.Command, args []string) {
		crud.ListElements("rules")
	},
}

func init() {
	// Flags for Add command
	AddRuleCmd.Flags().StringVar(&ruleID, "id", "", "Optional unique identifier for the rule")
	AddRuleCmd.Flags().StringVarP(&ruleContent, "content", "c", "", "Content of the rule (required)")
	errutils.LogIfErr(AddRuleCmd.MarkFlagRequired("content"))
	AddRuleCmd.Flags().StringVarP(&rulePriority, "priority", "p", "medium", "Priority of the rule (critical, high, medium, low, minimal)")
	AddRuleCmd.Flags().StringSliceVarP(&ruleTargets, "target", "t", []string{}, "Output target for this rule (can be specified multiple times)")

	// Flags for Update command
	UpdateRuleCmd.Flags().StringVar(&ruleID, "id", "", "New unique identifier for the rule")
	UpdateRuleCmd.Flags().StringVarP(&ruleContent, "content", "c", "", "New content for the rule")
	UpdateRuleCmd.Flags().StringVarP(&rulePriority, "priority", "p", "", "New priority for the rule")
	UpdateRuleCmd.Flags().StringSliceVarP(&ruleTargets, "target", "t", []string{}, "New set of output targets for the rule")
}
