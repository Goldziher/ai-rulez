package commands

import (
	"fmt"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/crud"
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
	Long: `Remove rules, context, skills, agents, commands or checks from your .ai-rulez/ configuration.

The item must exist: that is checked before anything is asked. The path that was
removed is printed on stdout; --format json prints a document with the type, name,
domain and path instead.`,
}

var removeRuleCmd = &cobra.Command{
	Use:   "rule <name>",
	Short: "Remove a rule",
	Long: `Remove a rule file.

Use --yes to skip confirmation prompts.`,
	Args: cobra.ExactArgs(1),
	RunE: runRemoveRule,
}

var removeContextCmd = &cobra.Command{
	Use:   "context <name>",
	Short: "Remove context",
	Long: `Remove a context file.

Use --yes to skip confirmation prompts.`,
	Args: cobra.ExactArgs(1),
	RunE: runRemoveContext,
}

var removeSkillCmd = &cobra.Command{
	Use:   "skill <name>",
	Short: "Remove a skill",
	Long: `Remove a skill directory.

Use --yes to skip confirmation prompts.`,
	Args: cobra.ExactArgs(1),
	RunE: runRemoveSkill,
}

var removeAgentCmd = &cobra.Command{
	Use:   "agent <name>",
	Short: "Remove an agent",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRemoveItem(cmd, args[0], crud.ContentTypeAgents, "agent")
	},
}

var removeCommandCmd = &cobra.Command{
	Use:   "command <name>",
	Short: "Remove a command",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRemoveItem(cmd, args[0], crud.ContentTypeCommands, kindCommand)
	},
}

var removeCheckCmd = &cobra.Command{
	Use:   "check <name>",
	Short: "Remove a code-review check",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRemoveItem(cmd, args[0], crud.ContentTypeChecks, "check")
	},
}

func init() {
	RemoveCmd.AddCommand(removeRuleCmd, removeContextCmd, removeSkillCmd, removeAgentCmd, removeCommandCmd, removeCheckCmd)
	for _, c := range []*cobra.Command{removeRuleCmd, removeContextCmd, removeSkillCmd, removeAgentCmd, removeCommandCmd, removeCheckCmd} {
		c.Flags().StringVarP(&removeDomain, "domain", "d", "", "Domain name (optional, searches root if not specified)")
		addYesFlag(c.Flags(), &removeForce, "Skip confirmation prompts")
		addResultFormat(c.Flags())
	}
	for _, c := range []*cobra.Command{removeRuleCmd, removeContextCmd, removeSkillCmd, removeAgentCmd, removeCommandCmd} {
		c.Flags().BoolVar(&removeLocal, "local", false, "Remove from the machine-local tree (.ai-rulez/local/)")
	}
}

func runRemoveRule(cmd *cobra.Command, args []string) error {
	return runRemoveItem(cmd, args[0], crud.ContentTypeRules, "rule")
}

func runRemoveContext(cmd *cobra.Command, args []string) error {
	return runRemoveItem(cmd, args[0], crud.ContentTypeContext, "context")
}

func runRemoveSkill(cmd *cobra.Command, args []string) error {
	return runRemoveItem(cmd, args[0], crud.ContentTypeSkills, "skill")
}

// runRemoveItem removes the content item name of type ftype. The item is
// checked first, so a name that does not exist fails before the confirmation
// instead of after it; the removed path is the result.
func runRemoveItem(cmd *cobra.Command, name, ftype, label string) error {
	out := outFor(cmd)
	op, err := newWritingOperator(removeLocal)
	if err != nil {
		return failMsg("Failed to create CRUD operator", err)
	}
	ctx := cmdContext()
	if err := op.RequireContent(ctx, removeDomain, ftype, name); err != nil {
		return failMsg("Failed to remove "+label, err)
	}
	resourceName := fmt.Sprintf("%s %s", label, name)
	if removeDomain != "" {
		resourceName = fmt.Sprintf("%s %s in domain %s", label, name, removeDomain)
	}
	if err := confirmRemovalUnlessYes(removeForce, "", resourceName, "Operation canceled"); err != nil {
		return fail(err)
	}
	path := op.ContentPath(removeDomain, ftype, name)
	if err := op.RemoveFile(ctx, removeDomain, ftype, name); err != nil {
		return failMsg("Failed to remove "+label, err)
	}
	out.Info("%s removed successfully\n", strings.ToUpper(label[:1])+label[1:])
	return reportChange(out, out.JSON(), changeResult{
		Status: statusRemoved, Type: label, Name: name, Domain: removeDomain, Path: path, Local: removeLocal,
	}, true)
}
