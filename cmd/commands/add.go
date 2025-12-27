package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/spf13/cobra"
)

var (
	addDomain   string
	addPriority string
	addTargets  string
	addContent  string
	addDesc     string
)

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
	Run:  runAddRule,
}

var addContextCmd = &cobra.Command{
	Use:   "context <name>",
	Short: "Add new context",
	Long: `Add a new context file with optional metadata.

Context provides background information, architecture details, or project-specific information.`,
	Args: cobra.ExactArgs(1),
	Run:  runAddContext,
}

var addSkillCmd = &cobra.Command{
	Use:   "skill <name>",
	Short: "Add a new skill",
	Long: `Add a new skill directory with SKILL.md.

Skills define specialized capabilities or personas for AI assistants.`,
	Args: cobra.ExactArgs(1),
	Run:  runAddSkill,
}

func init() {
	AddCmd.AddCommand(addRuleCmd)
	AddCmd.AddCommand(addContextCmd)
	AddCmd.AddCommand(addSkillCmd)

	// Common flags for all add commands
	addRuleCmd.Flags().StringVar(&addDomain, "domain", "", "Domain name (optional, uses root if not specified)")
	addRuleCmd.Flags().StringVar(&addPriority, "priority", "medium", "Priority level: critical|high|medium|low")
	addRuleCmd.Flags().StringVar(&addTargets, "targets", "", "Comma-separated list of target providers (e.g., claude,cursor)")
	addRuleCmd.Flags().StringVar(&addContent, "content", "", "File content (uses template if not specified)")

	addContextCmd.Flags().StringVar(&addDomain, "domain", "", "Domain name (optional, uses root if not specified)")
	addContextCmd.Flags().StringVar(&addPriority, "priority", "medium", "Priority level: critical|high|medium|low")
	addContextCmd.Flags().StringVar(&addContent, "content", "", "File content (uses template if not specified)")

	addSkillCmd.Flags().StringVar(&addDomain, "domain", "", "Domain name (optional, uses root if not specified)")
	addSkillCmd.Flags().StringVar(&addDesc, "description", "", "Skill description")
	addSkillCmd.Flags().StringVar(&addContent, "content", "", "File content (uses template if not specified)")
}

func runAddRule(cmd *cobra.Command, args []string) {
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

	ctx := context.Background()
	op, err := crud.NewOperator(".")
	if err != nil {
		logger.Error("Failed to create CRUD operator", "error", err)
		os.Exit(1)
	}

	req := &crud.AddFileRequest{
		Domain:   addDomain,
		Type:     "rules",
		Name:     name,
		Content:  addContent,
		Priority: addPriority,
		Targets:  targets,
	}

	result, err := op.AddRule(ctx, req)
	if err != nil {
		logger.Error("Failed to add rule", "error", err)
		os.Exit(1)
	}

	output := map[string]interface{}{
		"success": true,
		"type":    "rule",
		"name":    result.Name,
		"path":    result.FullPath,
	}
	if result.Domain != "" {
		output["domain"] = result.Domain
	}

	jsonOutput, _ := json.MarshalIndent(output, "", "  ")
	logger.Info(fmt.Sprintf("Rule added successfully: %s", result.FullPath))
	logger.Debug(string(jsonOutput))
}

func runAddContext(cmd *cobra.Command, args []string) {
	name := args[0]

	ctx := context.Background()
	op, err := crud.NewOperator(".")
	if err != nil {
		logger.Error("Failed to create CRUD operator", "error", err)
		os.Exit(1)
	}

	req := &crud.AddFileRequest{
		Domain:   addDomain,
		Type:     "context",
		Name:     name,
		Content:  addContent,
		Priority: addPriority,
	}

	result, err := op.AddContext(ctx, req)
	if err != nil {
		logger.Error("Failed to add context", "error", err)
		os.Exit(1)
	}

	output := map[string]interface{}{
		"success": true,
		"type":    "context",
		"name":    result.Name,
		"path":    result.FullPath,
	}
	if result.Domain != "" {
		output["domain"] = result.Domain
	}

	jsonOutput, _ := json.MarshalIndent(output, "", "  ")
	logger.Info(fmt.Sprintf("Context added successfully: %s", result.FullPath))
	logger.Debug(string(jsonOutput))
}

func runAddSkill(cmd *cobra.Command, args []string) {
	name := args[0]

	ctx := context.Background()
	op, err := crud.NewOperator(".")
	if err != nil {
		logger.Error("Failed to create CRUD operator", "error", err)
		os.Exit(1)
	}

	// For skills, we need to handle the skill-specific format
	// Skills use a different structure (directory with SKILL.md)
	req := &crud.AddFileRequest{
		Domain:  addDomain,
		Type:    "skills",
		Name:    name,
		Content: addContent,
	}

	result, err := op.AddSkill(ctx, req)
	if err != nil {
		logger.Error("Failed to add skill", "error", err)
		os.Exit(1)
	}

	output := map[string]interface{}{
		"success": true,
		"type":    "skill",
		"name":    result.Name,
		"path":    result.FullPath,
	}
	if result.Domain != "" {
		output["domain"] = result.Domain
	}

	jsonOutput, _ := json.MarshalIndent(output, "", "  ")
	logger.Info(fmt.Sprintf("Skill added successfully: %s", result.FullPath))
	logger.Debug(string(jsonOutput))
}
