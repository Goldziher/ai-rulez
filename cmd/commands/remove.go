package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/spf13/cobra"
)

var (
	removeDomain string
	removeForce  bool
)

var RemoveCmd = &cobra.Command{
	Use:   "remove",
	Short: "Remove content from your rules",
	Long:  `Remove rules, context, or skills from your .ai-rulez/ configuration.`,
}

var removeRuleCmd = &cobra.Command{
	Use:   "rule <name>",
	Short: "Remove a rule",
	Long: `Remove a rule file.

Use --force to skip confirmation prompts.`,
	Args: cobra.ExactArgs(1),
	Run:  runRemoveRule,
}

var removeContextCmd = &cobra.Command{
	Use:   "context <name>",
	Short: "Remove context",
	Long: `Remove a context file.

Use --force to skip confirmation prompts.`,
	Args: cobra.ExactArgs(1),
	Run:  runRemoveContext,
}

var removeSkillCmd = &cobra.Command{
	Use:   "skill <name>",
	Short: "Remove a skill",
	Long: `Remove a skill directory.

Use --force to skip confirmation prompts.`,
	Args: cobra.ExactArgs(1),
	Run:  runRemoveSkill,
}

func init() {
	RemoveCmd.AddCommand(removeRuleCmd)
	RemoveCmd.AddCommand(removeContextCmd)
	RemoveCmd.AddCommand(removeSkillCmd)

	// Common flags
	removeRuleCmd.Flags().StringVar(&removeDomain, "domain", "", "Domain name (optional, searches root if not specified)")
	removeRuleCmd.Flags().BoolVar(&removeForce, "force", false, "Skip confirmation prompts")

	removeContextCmd.Flags().StringVar(&removeDomain, "domain", "", "Domain name (optional, searches root if not specified)")
	removeContextCmd.Flags().BoolVar(&removeForce, "force", false, "Skip confirmation prompts")

	removeSkillCmd.Flags().StringVar(&removeDomain, "domain", "", "Domain name (optional, searches root if not specified)")
	removeSkillCmd.Flags().BoolVar(&removeForce, "force", false, "Skip confirmation prompts")
}

func runRemoveRule(cmd *cobra.Command, args []string) {
	name := args[0]

	// Confirm removal unless --force is specified
	if !removeForce {
		resourceName := fmt.Sprintf("rule %s", name)
		if removeDomain != "" {
			resourceName = fmt.Sprintf("rule %s in domain %s", name, removeDomain)
		}
		if !confirmRemoval("", resourceName) {
			logger.Info("Operation canceled")
			return
		}
	}

	ctx := context.Background()
	op, err := crud.NewOperator(".")
	if err != nil {
		logger.Error("Failed to create CRUD operator", "error", err)
		os.Exit(1)
	}

	if err := op.RemoveFile(ctx, removeDomain, "rules", name); err != nil {
		logger.Error("Failed to remove rule", "error", err)
		os.Exit(1)
	}

	logger.Info("Rule removed successfully", "name", name)
}

func runRemoveContext(cmd *cobra.Command, args []string) {
	name := args[0]

	// Confirm removal unless --force is specified
	if !removeForce {
		resourceName := fmt.Sprintf("context %s", name)
		if removeDomain != "" {
			resourceName = fmt.Sprintf("context %s in domain %s", name, removeDomain)
		}
		if !confirmRemoval("", resourceName) {
			logger.Info("Operation canceled")
			return
		}
	}

	ctx := context.Background()
	op, err := crud.NewOperator(".")
	if err != nil {
		logger.Error("Failed to create CRUD operator", "error", err)
		os.Exit(1)
	}

	if err := op.RemoveFile(ctx, removeDomain, "context", name); err != nil {
		logger.Error("Failed to remove context", "error", err)
		os.Exit(1)
	}

	logger.Info("Context removed successfully", "name", name)
}

func runRemoveSkill(cmd *cobra.Command, args []string) {
	name := args[0]

	// Confirm removal unless --force is specified
	if !removeForce {
		resourceName := fmt.Sprintf("skill %s", name)
		if removeDomain != "" {
			resourceName = fmt.Sprintf("skill %s in domain %s", name, removeDomain)
		}
		if !confirmRemoval("", resourceName) {
			logger.Info("Operation canceled")
			return
		}
	}

	ctx := context.Background()
	op, err := crud.NewOperator(".")
	if err != nil {
		logger.Error("Failed to create CRUD operator", "error", err)
		os.Exit(1)
	}

	if err := op.RemoveFile(ctx, removeDomain, "skills", name); err != nil {
		logger.Error("Failed to remove skill", "error", err)
		os.Exit(1)
	}

	logger.Info("Skill removed successfully", "name", name)
}
