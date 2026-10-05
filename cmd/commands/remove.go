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
	removeLocal  bool
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

var removeAgentCmd = &cobra.Command{
	Use:   "agent <name>",
	Short: "Remove an agent",
	Args:  cobra.ExactArgs(1),
	Run:   func(_ *cobra.Command, args []string) { runRemoveItem(args[0], crud.ContentTypeAgents, "agent") },
}

var removeCommandCmd = &cobra.Command{
	Use:   "command <name>",
	Short: "Remove a command",
	Args:  cobra.ExactArgs(1),
	Run:   func(_ *cobra.Command, args []string) { runRemoveItem(args[0], crud.ContentTypeCommands, "command") },
}

var removeCheckCmd = &cobra.Command{
	Use:   "check <name>",
	Short: "Remove a code-review check",
	Args:  cobra.ExactArgs(1),
	Run:   func(_ *cobra.Command, args []string) { runRemoveItem(args[0], crud.ContentTypeChecks, "check") },
}

func init() {
	RemoveCmd.AddCommand(removeAgentCmd)
	RemoveCmd.AddCommand(removeCommandCmd)
	RemoveCmd.AddCommand(removeCheckCmd)
	removeCheckCmd.Flags().StringVarP(&removeDomain, "domain", "d", "", "Domain name (optional, searches root if not specified)")
	removeCheckCmd.Flags().BoolVarP(&removeForce, "force", "f", false, "Skip confirmation prompts")
	for _, c := range []*cobra.Command{removeRuleCmd, removeContextCmd, removeSkillCmd, removeAgentCmd, removeCommandCmd} {
		c.Flags().StringVarP(&removeDomain, "domain", "d", "", "Domain name (optional, searches root if not specified)")
		c.Flags().BoolVarP(&removeForce, "force", "f", false, "Skip confirmation prompts")
		c.Flags().BoolVar(&removeLocal, "local", false, "Remove from the machine-local tree (.ai-rulez/local/)")
	}
	RemoveCmd.AddCommand(removeRuleCmd)
	RemoveCmd.AddCommand(removeContextCmd)
	RemoveCmd.AddCommand(removeSkillCmd)
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
	op, err := newContentOperator(removeLocal)
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
	op, err := newContentOperator(removeLocal)
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
	op, err := newContentOperator(removeLocal)
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

func runRemoveItem(name, ftype, label string) {
	if !removeForce {
		resourceName := fmt.Sprintf("%s %s", label, name)
		if removeDomain != "" {
			resourceName = fmt.Sprintf("%s %s in domain %s", label, name, removeDomain)
		}
		if !confirmRemoval("", resourceName) {
			logger.Info("Operation canceled")
			return
		}
	}
	op, err := newContentOperator(removeLocal)
	if err != nil {
		logger.Error("Failed to create CRUD operator", "error", err)
		os.Exit(1)
	}
	if err := op.RemoveFile(context.Background(), removeDomain, ftype, name); err != nil {
		logger.Error("Failed to remove "+label, "error", err)
		os.Exit(1)
	}
	logger.Info(label+" removed successfully", "name", name)
}
