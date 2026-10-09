package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/crud"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/spf13/cobra"
)

var (
	addDomain   string
	addPriority string
	addTargets  string
	addContent  string
	addDesc     string
	addLocal    bool
)

const addLocalUsage = "Write to .ai-rulez/local/ as a machine-local item (gitignored); combine with --domain for a local domain"

var AddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add content to your rules",
	Long:  `Add rules, context, or skills to your .ai-rulez/ configuration.`,
}

var addRuleCmd = &cobra.Command{
	Use:   "rule <name>",
	Short: "Add a new rule",
	Long: `Add a new rule file with optional metadata.

Rules contain instructions and guidelines for AI assistants.
You can specify domain, priority level, and target providers.`,
	Args: cobra.ExactArgs(1),
	RunE: runAddRule,
}

var addContextCmd = &cobra.Command{
	Use:   "context <name>",
	Short: "Add new context",
	Long: `Add a new context file with optional metadata.

Context provides background information, architecture details, or project-specific information.`,
	Args: cobra.ExactArgs(1),
	RunE: runAddContext,
}

var addSkillCmd = &cobra.Command{
	Use:   "skill <name>",
	Short: "Add a new skill",
	Long: `Add a new skill directory with SKILL.md.

Skills define specialized capabilities or personas for AI assistants.`,
	Args: cobra.ExactArgs(1),
	RunE: runAddSkill,
}

var addAgentCmd = &cobra.Command{
	Use:   "agent <name>",
	Short: "Add a new agent",
	Long: `Add a new agent file.

Agents define specialized sub-agents for tools that support them.`,
	Args: cobra.ExactArgs(1),
	RunE: runAddAgent,
}

var addCommandCmd = &cobra.Command{
	Use:   "command <name>",
	Short: "Add a new command",
	Long: `Add a new command file.

Commands define reusable slash commands for tools that support them.`,
	Args: cobra.ExactArgs(1),
	RunE: runAddCommand,
}

var addCheckCmd = &cobra.Command{
	Use:   "check <name>",
	Short: "Add a new code-review check",
	Long: `Add a new check file.

Checks are code-review guidelines (.ai-rulez/checks/<name>.md) rendered for the
review tools that support them. Frontmatter: description, severity
(low|medium|high|critical), tools, targets.`,
	Args: cobra.ExactArgs(1),
	RunE: runAddCheck,
}

func init() {
	AddCmd.AddCommand(addAgentCmd)
	AddCmd.AddCommand(addCommandCmd)
	AddCmd.AddCommand(addCheckCmd)
	for _, c := range []*cobra.Command{addAgentCmd, addCommandCmd} {
		c.Flags().StringVarP(&addDomain, "domain", "d", "", "Domain name (optional, uses root if not specified)")
		c.Flags().StringVarP(&addDesc, "description", "s", "", "Description")
		c.Flags().StringVarP(&addContent, "content", "c", "", "File content (uses template if not specified)")
		c.Flags().BoolVar(&addLocal, "local", false, addLocalUsage)
	}
	// Checks are shared review guidance: there is no machine-local check tree.
	addCheckCmd.Flags().StringVarP(&addDomain, "domain", "d", "", "Domain name (optional, uses root if not specified)")
	addCheckCmd.Flags().StringVarP(&addDesc, "description", "s", "", "Description")
	addCheckCmd.Flags().StringVarP(&addContent, "content", "c", "", "File content (uses template if not specified)")
	AddCmd.AddCommand(addRuleCmd)
	AddCmd.AddCommand(addContextCmd)
	AddCmd.AddCommand(addSkillCmd)

	// Common flags for all add commands
	addRuleCmd.Flags().StringVarP(&addDomain, "domain", "d", "", "Domain name (optional, uses root if not specified)")
	addRuleCmd.Flags().StringVarP(&addPriority, "priority", "p", "medium", "Priority level: critical|high|medium|low|minimal")
	addRuleCmd.Flags().StringVarP(&addTargets, "targets", "t", "", "Comma-separated list of target providers (e.g., claude,cursor)")
	addRuleCmd.Flags().StringVarP(&addContent, "content", "c", "", "File content (uses template if not specified)")
	addRuleCmd.Flags().BoolVar(&addLocal, "local", false, addLocalUsage)

	addContextCmd.Flags().StringVarP(&addDomain, "domain", "d", "", "Domain name (optional, uses root if not specified)")
	addContextCmd.Flags().StringVarP(&addPriority, "priority", "p", "medium", "Priority level: critical|high|medium|low|minimal")
	addContextCmd.Flags().StringVarP(&addContent, "content", "c", "", "File content (uses template if not specified)")
	addContextCmd.Flags().BoolVar(&addLocal, "local", false, addLocalUsage)

	addSkillCmd.Flags().StringVarP(&addDomain, "domain", "d", "", "Domain name (optional, uses root if not specified)")
	addSkillCmd.Flags().StringVarP(&addDesc, "description", "s", "", "Skill description")
	addSkillCmd.Flags().BoolVar(&addLocal, "local", false, addLocalUsage)
	addSkillCmd.Flags().StringVarP(&addContent, "content", "c", "", "File content (uses template if not specified)")
}

func runAddRule(cmd *cobra.Command, args []string) error {
	name := args[0]

	// Parse targets
	var targets []string
	if addTargets != "" {
		for _, t := range strings.Split(addTargets, ",") {
			if t = strings.TrimSpace(t); t != "" {
				targets = append(targets, t)
			}
		}
	}

	ctx := cmdContext()
	op, err := newWritingOperator(addLocal)
	if err != nil {
		return failMsg("Failed to create CRUD operator", err)
	}

	req := &crud.AddFileRequest{
		Domain:   addDomain,
		Type:     crud.ContentTypeRules,
		Name:     name,
		Content:  addContent,
		Priority: addPriority,
		Targets:  targets,
	}

	result, err := op.AddRule(ctx, req)
	if err != nil {
		return failMsg("Failed to add rule", err)
	}

	output := map[string]interface{}{
		keySuccess: true,
		keyType:    "rule",
		keyName:    result.Name,
		keyPath:    result.FullPath,
	}
	if result.Domain != "" {
		output["domain"] = result.Domain
	}

	jsonOutput, _ := json.MarshalIndent(output, "", "  ")
	logger.Info(fmt.Sprintf("Rule added successfully: %s", result.FullPath))
	logger.Debug(string(jsonOutput))
	return nil
}

func runAddContext(cmd *cobra.Command, args []string) error {
	name := args[0]

	ctx := cmdContext()
	op, err := newWritingOperator(addLocal)
	if err != nil {
		return failMsg("Failed to create CRUD operator", err)
	}

	req := &crud.AddFileRequest{
		Domain:   addDomain,
		Type:     crud.ContentTypeContext,
		Name:     name,
		Content:  addContent,
		Priority: addPriority,
	}

	result, err := op.AddContext(ctx, req)
	if err != nil {
		return failMsg("Failed to add context", err)
	}

	output := map[string]interface{}{
		keySuccess: true,
		keyType:    crud.ContentTypeContext,
		keyName:    result.Name,
		keyPath:    result.FullPath,
	}
	if result.Domain != "" {
		output["domain"] = result.Domain
	}

	jsonOutput, _ := json.MarshalIndent(output, "", "  ")
	logger.Info(fmt.Sprintf("Context added successfully: %s", result.FullPath))
	logger.Debug(string(jsonOutput))
	return nil
}

func runAddSkill(cmd *cobra.Command, args []string) error {
	name := args[0]

	ctx := cmdContext()
	op, err := newWritingOperator(addLocal)
	if err != nil {
		return failMsg("Failed to create CRUD operator", err)
	}

	// For skills, we need to handle the skill-specific format
	// Skills use a different structure (directory with SKILL.md)
	req := &crud.AddFileRequest{
		Domain:      addDomain,
		Type:        crud.ContentTypeSkills,
		Name:        name,
		Description: addDesc,
		Content:     addContent,
	}

	result, err := op.AddSkill(ctx, req)
	if err != nil {
		return failMsg("Failed to add skill", err)
	}

	output := map[string]interface{}{
		keySuccess: true,
		keyType:    kindSkill,
		keyName:    result.Name,
		keyPath:    result.FullPath,
	}
	if result.Domain != "" {
		output["domain"] = result.Domain
	}

	jsonOutput, _ := json.MarshalIndent(output, "", "  ")
	logger.Info(fmt.Sprintf("Skill added successfully: %s", result.FullPath))
	logger.Debug(string(jsonOutput))
	return nil
}

func runAddAgent(cmd *cobra.Command, args []string) error {
	return runAddItem(args[0], crud.ContentTypeAgents, "agent", (*crud.OperatorImpl).AddAgent)
}

func runAddCommand(cmd *cobra.Command, args []string) error {
	return runAddItem(args[0], crud.ContentTypeCommands, "command", (*crud.OperatorImpl).AddCommand)
}

func runAddCheck(cmd *cobra.Command, args []string) error {
	return runAddItem(args[0], crud.ContentTypeChecks, "check", (*crud.OperatorImpl).AddCheck)
}

func runAddItem(name, ftype, label string,
	add func(*crud.OperatorImpl, context.Context, *crud.AddFileRequest) (*crud.FileResult, error),
) error {
	op, err := newWritingOperator(addLocal)
	if err != nil {
		return failMsg("Failed to create CRUD operator", err)
	}
	result, err := add(op, cmdContext(), &crud.AddFileRequest{
		Domain: addDomain, Type: ftype, Name: name, Description: addDesc, Content: addContent,
	})
	if err != nil {
		return failMsg("Failed to add "+label, err)
	}
	logger.Info(fmt.Sprintf("%s added successfully: %s", strings.ToUpper(label[:1])+label[1:], result.FullPath))
	return nil
}
