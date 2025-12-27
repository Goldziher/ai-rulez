package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/Goldziher/ai-rulez/internal/crud"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/spf13/cobra"
)

var (
	listDomain string
	listJSON   bool
)

var ListCmd = &cobra.Command{
	Use:   "list",
	Short: "List rules, context, and skills",
	Long:  `List rules, context, or skills in your .ai-rulez/ configuration.`,
}

var listRulesCmd = &cobra.Command{
	Use:   "rules",
	Short: "List all rules",
	Long: `List all rules in your .ai-rulez/ configuration.

You can filter by domain using --domain flag.`,
	Args: cobra.NoArgs,
	Run:  runListRules,
}

var listContextCmd = &cobra.Command{
	Use:   "context",
	Short: "List all context files",
	Long: `List all context files in your .ai-rulez/ configuration.

You can filter by domain using --domain flag.`,
	Args: cobra.NoArgs,
	Run:  runListContext,
}

var listSkillsCmd = &cobra.Command{
	Use:   "skills",
	Short: "List all skills",
	Long: `List all skills in your .ai-rulez/ configuration.

You can filter by domain using --domain flag.`,
	Args: cobra.NoArgs,
	Run:  runListSkills,
}

func init() {
	ListCmd.AddCommand(listRulesCmd)
	ListCmd.AddCommand(listContextCmd)
	ListCmd.AddCommand(listSkillsCmd)

	// Common flags
	listRulesCmd.Flags().StringVar(&listDomain, "domain", "", "Filter by domain (shows all if not specified)")
	listRulesCmd.Flags().BoolVar(&listJSON, "json", false, "Output as JSON")

	listContextCmd.Flags().StringVar(&listDomain, "domain", "", "Filter by domain (shows all if not specified)")
	listContextCmd.Flags().BoolVar(&listJSON, "json", false, "Output as JSON")

	listSkillsCmd.Flags().StringVar(&listDomain, "domain", "", "Filter by domain (shows all if not specified)")
	listSkillsCmd.Flags().BoolVar(&listJSON, "json", false, "Output as JSON")
}

func runListRules(cmd *cobra.Command, args []string) {
	ctx := context.Background()
	op, err := crud.NewOperator(".")
	if err != nil {
		logger.Error("Failed to create CRUD operator", "error", err)
		os.Exit(1)
	}

	files, err := op.ListFiles(ctx, listDomain, "rules")
	if err != nil {
		logger.Error("Failed to list rules", "error", err)
		os.Exit(1)
	}

	if len(files) == 0 {
		logger.Info("No rules found")
		return
	}

	if listJSON {
		outputListJSON("rules", files)
	} else {
		outputListTable("Rules", files)
	}
}

func runListContext(cmd *cobra.Command, args []string) {
	ctx := context.Background()
	op, err := crud.NewOperator(".")
	if err != nil {
		logger.Error("Failed to create CRUD operator", "error", err)
		os.Exit(1)
	}

	files, err := op.ListFiles(ctx, listDomain, "context")
	if err != nil {
		logger.Error("Failed to list context", "error", err)
		os.Exit(1)
	}

	if len(files) == 0 {
		logger.Info("No context files found")
		return
	}

	if listJSON {
		outputListJSON("context", files)
	} else {
		outputListTable("Context", files)
	}
}

func runListSkills(cmd *cobra.Command, args []string) {
	ctx := context.Background()
	op, err := crud.NewOperator(".")
	if err != nil {
		logger.Error("Failed to create CRUD operator", "error", err)
		os.Exit(1)
	}

	files, err := op.ListFiles(ctx, listDomain, "skills")
	if err != nil {
		logger.Error("Failed to list skills", "error", err)
		os.Exit(1)
	}

	if len(files) == 0 {
		logger.Info("No skills found")
		return
	}

	if listJSON {
		outputListJSON("skills", files)
	} else {
		outputListTable("Skills", files)
	}
}

// outputListJSON outputs file list as JSON
func outputListJSON(fileType string, files []crud.FileInfo) {
	output := make([]map[string]interface{}, len(files))
	for i, file := range files {
		output[i] = map[string]interface{}{
			"name":     file.Name,
			"type":     file.Type,
			"domain":   file.Domain,
			"path":     file.Path,
			"priority": file.Priority,
			"targets":  file.Targets,
		}
	}
	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		logger.Error("Failed to marshal JSON", "error", err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}

// outputListTable outputs file list as human-readable table
func outputListTable(title string, files []crud.FileInfo) {
	logger.Info(fmt.Sprintf("%s:", title))

	// Group by domain
	byDomain := make(map[string][]crud.FileInfo)
	for _, file := range files {
		if file.Domain == "" {
			byDomain["(root)"] = append(byDomain["(root)"], file)
		} else {
			byDomain[file.Domain] = append(byDomain[file.Domain], file)
		}
	}

	// Output grouped by domain
	for domain, domainFiles := range byDomain {
		logger.Info(fmt.Sprintf("  %s:", domain))
		for _, file := range domainFiles {
			info := fmt.Sprintf("    • %s", file.Name)
			if file.Priority != "" {
				info += fmt.Sprintf(" [%s]", file.Priority)
			}
			if len(file.Targets) > 0 {
				info += fmt.Sprintf(" (targets: %v)", file.Targets)
			}
			logger.Info(info)
		}
	}
}
