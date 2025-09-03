package crud

import (
	"fmt"
	"os"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/spf13/cobra"
)

var (
	ruleName     string
	ruleContent  string
	rulePriority string
	newRuleName  string
	ruleTargets  []string
)

// AddRuleCmd adds a new rule to the configuration
var AddRuleCmd = &cobra.Command{
	Use:   "rule",
	Short: "Add a new rule to the configuration",
	Long:  `Add a new rule with name, content, and optional priority to your ai-rulez configuration.`,
	Run:   runAddRule,
}

// UpdateRuleCmd updates an existing rule
var UpdateRuleCmd = &cobra.Command{
	Use:   "rule",
	Short: "Update an existing rule",
	Long:  `Update an existing rule's name, content, or priority in your ai-rulez configuration.`,
	Run:   runUpdateRule,
}

// DeleteRuleCmd deletes a rule
var DeleteRuleCmd = &cobra.Command{
	Use:   "rule <name>",
	Short: "Delete a rule from the configuration",
	Long:  `Delete a rule by name from your ai-rulez configuration.`,
	Args:  cobra.ExactArgs(1),
	Run:   runDeleteRule,
}

func init() {
	AddRuleCmd.Flags().StringVar(&ruleName, "name", "", "Name of the rule (required)")
	AddRuleCmd.Flags().StringVar(&ruleContent, "content", "", "Content of the rule (required)")
	AddRuleCmd.Flags().StringVar(&rulePriority, "priority", "medium", "Priority of the rule (critical/high/medium/low/minimal)")
	AddRuleCmd.Flags().StringSliceVar(&ruleTargets, "targets", []string{}, "Target file patterns (glob patterns)")
	if err := AddRuleCmd.MarkFlagRequired("name"); err != nil {
		panic(err)
	}
	if err := AddRuleCmd.MarkFlagRequired("content"); err != nil {
		panic(err)
	}

	UpdateRuleCmd.Flags().StringVar(&ruleName, "name", "", "Name of the rule to update (required)")
	UpdateRuleCmd.Flags().StringVar(&newRuleName, "new-name", "", "New name for the rule")
	UpdateRuleCmd.Flags().StringVar(&ruleContent, "content", "", "New content for the rule")
	UpdateRuleCmd.Flags().StringVar(&rulePriority, "priority", "", "New priority for the rule (critical/high/medium/low/minimal)")
	UpdateRuleCmd.Flags().StringSliceVar(&ruleTargets, "targets", []string{}, "Target file patterns (glob patterns)")
	if err := UpdateRuleCmd.MarkFlagRequired("name"); err != nil {
		panic(err)
	}
}

func runAddRule(cmd *cobra.Command, args []string) {
	configPath, cfg := loadConfiguration()

	for _, rule := range cfg.Rules {
		if rule.Name == ruleName {
			fmt.Printf("❌ Rule '%s' already exists\n", ruleName)
			os.Exit(1)
		}
	}

	priority, err := config.ParsePriority(rulePriority)
	if err != nil {
		fmt.Printf("❌ Invalid priority '%s'. Valid values: critical, high, medium, low, minimal\n", rulePriority)
		os.Exit(1)
	}

	newRule := config.Rule{
		Name:     ruleName,
		Content:  ruleContent,
		Priority: priority,
		Targets:  ruleTargets,
	}
	cfg.Rules = append(cfg.Rules, newRule)

	saveConfiguration(cfg, configPath)
	fmt.Printf("✅ Added rule '%s' successfully\n", ruleName)
}

func runUpdateRule(cmd *cobra.Command, args []string) {
	configPath, cfg := loadConfiguration()

	ruleIndex := -1
	for i, rule := range cfg.Rules {
		if rule.Name == ruleName {
			ruleIndex = i
			break
		}
	}

	if ruleIndex == -1 {
		fmt.Printf("❌ Rule '%s' not found\n", ruleName)
		os.Exit(1)
	}

	if newRuleName != "" {
		for i, rule := range cfg.Rules {
			if i != ruleIndex && rule.Name == newRuleName {
				fmt.Printf("❌ Rule '%s' already exists\n", newRuleName)
				os.Exit(1)
			}
		}
		cfg.Rules[ruleIndex].Name = newRuleName
	}

	if ruleContent != "" {
		cfg.Rules[ruleIndex].Content = ruleContent
	}

	if rulePriority != "" {
		priority, err := config.ParsePriority(rulePriority)
		if err != nil {
			fmt.Printf("❌ Invalid priority '%s'. Valid values: critical, high, medium, low, minimal\n", rulePriority)
			return
		}
		cfg.Rules[ruleIndex].Priority = priority
	}

	if len(ruleTargets) > 0 {
		cfg.Rules[ruleIndex].Targets = ruleTargets
	}

	saveConfiguration(cfg, configPath)
	fmt.Printf("✅ Updated rule '%s' successfully\n", ruleName)
}

func runDeleteRule(cmd *cobra.Command, args []string) {
	ruleName := args[0]
	configPath, cfg := loadConfiguration()

	ruleIndex := -1
	for i, rule := range cfg.Rules {
		if rule.Name == ruleName {
			ruleIndex = i
			break
		}
	}

	if ruleIndex == -1 {
		fmt.Printf("❌ Rule '%s' not found\n", ruleName)
		os.Exit(1)
	}

	cfg.Rules = append(cfg.Rules[:ruleIndex], cfg.Rules[ruleIndex+1:]...)

	saveConfiguration(cfg, configPath)
	fmt.Printf("✅ Deleted rule '%s' successfully\n", ruleName)
}
