package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/crud"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/spf13/cobra"
)

var (
	listDomain string
	listJSON   bool
	listLocal  bool

	listPlacement bool
	listProfile   string
)

var ListCmd = &cobra.Command{
	Use:   cmdUseList,
	Short: "List rules, context, and skills",
	Long: `List rules, context, or skills in your .ai-rulez/ configuration.

With --placement, print where every skill and command ends up: core (generated
into the assistants' own directories) or plugin-only (shipped by plugins), with
the plugins that bundle it and a flag on plugin-only items nobody can reach.`,
	Args: cobra.NoArgs,
	Run:  runListRoot,
}

var listRulesCmd = &cobra.Command{
	Use:   crud.ContentTypeRules,
	Short: "List all rules",
	Long: `List all rules in your .ai-rulez/ configuration.

You can filter by domain using --domain flag.`,
	Args: cobra.NoArgs,
	Run:  runListRules,
}

var listContextCmd = &cobra.Command{
	Use:   crud.ContentTypeContext,
	Short: "List all context files",
	Long: `List all context files in your .ai-rulez/ configuration.

You can filter by domain using --domain flag.`,
	Args: cobra.NoArgs,
	Run:  runListContext,
}

var listSkillsCmd = &cobra.Command{
	Use:   crud.ContentTypeSkills,
	Short: "List all skills",
	Long: `List all skills in your .ai-rulez/ configuration.

You can filter by domain using --domain flag.`,
	Args: cobra.NoArgs,
	Run:  runListSkills,
}

var listAgentsCmd = &cobra.Command{
	Use:   crud.ContentTypeAgents,
	Short: "List all agents",
	Args:  cobra.NoArgs,
	Run:   func(_ *cobra.Command, _ []string) { runListItems(crud.ContentTypeAgents, "Agents", "agents") },
}

var listCommandsCmd = &cobra.Command{
	Use:   crud.ContentTypeCommands,
	Short: "List all commands",
	Args:  cobra.NoArgs,
	Run:   func(_ *cobra.Command, _ []string) { runListItems(crud.ContentTypeCommands, "Commands", "commands") },
}

var listChecksCmd = &cobra.Command{
	Use:     crud.ContentTypeChecks,
	Aliases: []string{"check"},
	Short:   "List all code-review checks",
	Args:    cobra.NoArgs,
	Run:     func(_ *cobra.Command, _ []string) { runListItems(crud.ContentTypeChecks, "Checks", "checks") },
}

func init() {
	ListCmd.Flags().BoolVar(&listPlacement, "placement", false, "Report where each skill and command is placed: core or plugin-only, and which plugins bundle it")
	ListCmd.Flags().StringVarP(&listProfile, "profile", "p", "", "Profile for --placement (default: from config or 'default')")
	addJSONFormat(ListCmd.Flags(), &listJSON, "j")
	ListCmd.AddCommand(listAgentsCmd)
	ListCmd.AddCommand(listCommandsCmd)
	ListCmd.AddCommand(listChecksCmd)
	listChecksCmd.Flags().StringVarP(&listDomain, "domain", "d", "", "Filter by domain (shows all if not specified)")
	addJSONFormat(listChecksCmd.Flags(), &listJSON, "j")
	for _, c := range []*cobra.Command{listRulesCmd, listContextCmd, listSkillsCmd, listAgentsCmd, listCommandsCmd} {
		c.Flags().StringVarP(&listDomain, "domain", "d", "", "Filter by domain (shows all if not specified)")
		addJSONFormat(c.Flags(), &listJSON, "j")
		c.Flags().BoolVar(&listLocal, "local", false, "List the machine-local tree (.ai-rulez/local/)")
	}
	ListCmd.AddCommand(listRulesCmd)
	ListCmd.AddCommand(listContextCmd)
	ListCmd.AddCommand(listSkillsCmd)
}

func runListRules(cmd *cobra.Command, args []string) {
	ctx := context.Background()
	op, err := newContentOperator(listLocal)
	if err != nil {
		logger.Error("Failed to create CRUD operator", "error", err)
		os.Exit(1)
	}

	files, err := op.ListFiles(ctx, listDomain, crud.ContentTypeRules)
	if err != nil {
		logger.Error("Failed to list rules", "error", err)
		os.Exit(1)
	}

	if len(files) == 0 {
		logger.Info("No rules found")
		return
	}

	if listJSON {
		outputListJSON(crud.ContentTypeRules, files)
	} else {
		outputListTable("Rules", files)
	}
}

func runListContext(cmd *cobra.Command, args []string) {
	ctx := context.Background()
	op, err := newContentOperator(listLocal)
	if err != nil {
		logger.Error("Failed to create CRUD operator", "error", err)
		os.Exit(1)
	}

	files, err := op.ListFiles(ctx, listDomain, crud.ContentTypeContext)
	if err != nil {
		logger.Error("Failed to list context", "error", err)
		os.Exit(1)
	}

	if len(files) == 0 {
		logger.Info("No context files found")
		return
	}

	if listJSON {
		outputListJSON(crud.ContentTypeContext, files)
	} else {
		outputListTable("Context", files)
	}
}

func runListSkills(cmd *cobra.Command, args []string) {
	ctx := context.Background()
	op, err := newContentOperator(listLocal)
	if err != nil {
		logger.Error("Failed to create CRUD operator", "error", err)
		os.Exit(1)
	}

	files, err := op.ListFiles(ctx, listDomain, crud.ContentTypeSkills)
	if err != nil {
		logger.Error("Failed to list skills", "error", err)
		os.Exit(1)
	}

	if len(files) == 0 {
		logger.Info("No skills found")
		return
	}

	if listJSON {
		outputListJSON(crud.ContentTypeSkills, files)
	} else {
		outputListTable("Skills", files)
	}
}

// outputListJSON outputs file list as JSON
func outputListJSON(fileType string, files []crud.FileInfo) {
	output := make([]map[string]interface{}, len(files))
	for i, file := range files {
		output[i] = map[string]interface{}{
			keyName:    file.Name,
			keyType:    file.Type,
			"domain":   file.Domain,
			keyPath:    file.Path,
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

func runListItems(ftype, title, noun string) {
	op, err := newContentOperator(listLocal)
	if err != nil {
		logger.Error("Failed to create CRUD operator", "error", err)
		os.Exit(1)
	}
	files, err := op.ListFiles(context.Background(), listDomain, ftype)
	if err != nil {
		logger.Error("Failed to list "+noun, "error", err)
		os.Exit(1)
	}
	if len(files) == 0 {
		logger.Info("No " + noun + " found")
		return
	}
	if listJSON {
		outputListJSON(ftype, files)
	} else {
		outputListTable(title, files)
	}
}

func runListRoot(cmd *cobra.Command, _ []string) {
	if !listPlacement {
		if err := cmd.Help(); err != nil {
			logger.Error("Failed to print help", "error", err)
		}
		return
	}
	cfg, err := loadConfigForCommand(context.Background(), nil, pluginLoadOptions(true)...)
	if err != nil {
		logger.Error("Failed to load config", "error", err)
		os.Exit(1)
	}
	report, err := generator.NewGenerator(cfg).PlacementReport(listProfile)
	if err != nil {
		logger.Error("Failed to resolve placement", "error", err)
		os.Exit(1)
	}
	if listJSON {
		data, _ := json.MarshalIndent(report, "", "  ") //nolint:errcheck // plain structs always marshal
		fmt.Println(string(data))
		return
	}
	printPlacementReport(report)
}

func printPlacementReport(report *generator.PlacementReport) {
	fmt.Printf("Placement for profile %q\n\n", report.Profile)
	if len(report.Items) == 0 {
		fmt.Println("No skills or commands.")
		return
	}
	fmt.Printf("%-8s %-32s %-16s %-7s %s\n", "TYPE", "NAME", "DOMAIN", "WHERE", "PLUGINS")
	for _, it := range report.Items {
		domain := it.Domain
		if domain == "" {
			domain = "-"
		}
		plugins := "-"
		if len(it.Plugins) > 0 {
			plugins = strings.Join(it.Plugins, ", ")
		}
		fmt.Printf("%-8s %-32s %-16s %-7s %s\n", it.Type, it.Name, domain, it.Destination, plugins)
	}
	if issues := report.Issues(); len(issues) > 0 {
		fmt.Println()
		for _, it := range issues {
			fmt.Printf("warning: %s %q is plugin-only but %s\n", it.Type, it.Name, it.Issue)
		}
	}
}
