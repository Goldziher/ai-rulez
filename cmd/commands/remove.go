package commands

import (
	"fmt"

	"github.com/Goldziher/ai-rulez/v5/internal/crud"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
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

Use --yes to skip confirmation prompts.`,
	Args: cobra.ExactArgs(1),
	Run:  runRemoveRule,
}

var removeContextCmd = &cobra.Command{
	Use:   "context <name>",
	Short: "Remove context",
	Long: `Remove a context file.

Use --yes to skip confirmation prompts.`,
	Args: cobra.ExactArgs(1),
	Run:  runRemoveContext,
}

var removeSkillCmd = &cobra.Command{
	Use:   "skill <name>",
	Short: "Remove a skill",
	Long: `Remove a skill directory.

Use --yes to skip confirmation prompts.`,
	Args: cobra.ExactArgs(1),
	Run:  runRemoveSkill,
}

var removeAgentCmd = &cobra.Command{
	Use:   "agent <name>",
	Short: "Remove an agent",
	Args:  cobra.ExactArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		runRemoveItem(args[0], crud.ContentTypeAgents, "agent", "agent")
	},
}

var removeCommandCmd = &cobra.Command{
	Use:   "command <name>",
	Short: "Remove a command",
	Args:  cobra.ExactArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		runRemoveItem(args[0], crud.ContentTypeCommands, "command", "command")
	},
}

var removeCheckCmd = &cobra.Command{
	Use:   "check <name>",
	Short: "Remove a code-review check",
	Args:  cobra.ExactArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		runRemoveItem(args[0], crud.ContentTypeChecks, "check", "check")
	},
}

func init() {
	RemoveCmd.AddCommand(removeAgentCmd)
	RemoveCmd.AddCommand(removeCommandCmd)
	RemoveCmd.AddCommand(removeCheckCmd)
	removeCheckCmd.Flags().StringVarP(&removeDomain, "domain", "d", "", "Domain name (optional, searches root if not specified)")
	addYesFlag(removeCheckCmd.Flags(), &removeForce, "Skip confirmation prompts")
	for _, c := range []*cobra.Command{removeRuleCmd, removeContextCmd, removeSkillCmd, removeAgentCmd, removeCommandCmd} {
		c.Flags().StringVarP(&removeDomain, "domain", "d", "", "Domain name (optional, searches root if not specified)")
		addYesFlag(c.Flags(), &removeForce, "Skip confirmation prompts")
		c.Flags().BoolVar(&removeLocal, "local", false, "Remove from the machine-local tree (.ai-rulez/local/)")
	}
	RemoveCmd.AddCommand(removeRuleCmd)
	RemoveCmd.AddCommand(removeContextCmd)
	RemoveCmd.AddCommand(removeSkillCmd)
}

func runRemoveRule(_ *cobra.Command, args []string) {
	runRemoveItem(args[0], "rules", "rule", "Rule")
}

func runRemoveContext(_ *cobra.Command, args []string) {
	runRemoveItem(args[0], "context", "context", "Context")
}

func runRemoveSkill(_ *cobra.Command, args []string) {
	runRemoveItem(args[0], "skills", "skill", "Skill")
}

// runRemoveItem removes the content file name of type ftype after confirmation.
// label names the item in the prompt and errors, doneLabel in the success line.
func runRemoveItem(name, ftype, label, doneLabel string) {
	resourceName := fmt.Sprintf("%s %s", label, name)
	if removeDomain != "" {
		resourceName = fmt.Sprintf("%s %s in domain %s", label, name, removeDomain)
	}
	confirmRemovalUnlessYes(removeForce, "", resourceName, "Operation canceled")
	op, err := newWritingOperator(removeLocal)
	if err != nil {
		fatal("Failed to create CRUD operator", err)
	}
	if err := op.RemoveFile(cmdContext(), removeDomain, ftype, name); err != nil {
		fatal("Failed to remove "+label, err)
	}
	logger.Info(doneLabel+" removed successfully", "name", name)
}
