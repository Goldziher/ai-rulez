package commands

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/crud"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/jsondoc"
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
	ctx := cmdContext()
	op, err := newContentOperator(listLocal)
	if err != nil {
		fatal("Failed to create CRUD operator", err)
	}

	loadListedConfig(ctx)
	files, err := op.ListFiles(ctx, listDomain, crud.ContentTypeRules)
	if err != nil {
		fatal("Failed to list rules", err)
	}

	if len(files) == 0 && !listJSON {
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
	ctx := cmdContext()
	op, err := newContentOperator(listLocal)
	if err != nil {
		fatal("Failed to create CRUD operator", err)
	}

	loadListedConfig(ctx)
	files, err := op.ListFiles(ctx, listDomain, crud.ContentTypeContext)
	if err != nil {
		fatal("Failed to list context", err)
	}

	if len(files) == 0 && !listJSON {
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
	ctx := cmdContext()
	op, err := newContentOperator(listLocal)
	if err != nil {
		fatal("Failed to create CRUD operator", err)
	}

	loadListedConfig(ctx)
	files, err := op.ListFiles(ctx, listDomain, crud.ContentTypeSkills)
	if err != nil {
		fatal("Failed to list skills", err)
	}

	if len(files) == 0 && !listJSON {
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
	data, err := jsondoc.Marshal(output)
	if err != nil {
		fatal("Failed to marshal JSON", err)
	}
	fmt.Print(string(data))
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
		fatal("Failed to create CRUD operator", err)
	}
	loadListedConfig(cmdContext())
	files, err := op.ListFiles(cmdContext(), listDomain, ftype)
	if err != nil {
		logger.Error("Failed to list "+noun, "error", err)
		os.Exit(1)
	}
	if len(files) == 0 && !listJSON {
		logger.Info("No " + noun + " found")
		return
	}
	if listJSON {
		outputListJSON(ftype, files)
	} else {
		outputListTable(title, files)
	}
}

// loadListedConfig loads the project before a listing and exits 1 when the
// configuration does not load (a syntax error, a V2/V3 config file), the code
// every other command uses for it. The lists read the content tree only; an
// include that cannot be resolved (nothing cached, no network is used) is not an
// error here but a warning from the load, which says what the listing leaves out.
// The machine-local tree (--local) gets the check without the include warnings.
func loadListedConfig(ctx context.Context) {
	ctx = config.WithOfflineIncludes(config.WithUnresolvedIncludesTolerated(ctx))
	opts := []config.LoadOption{config.WithoutLocal()}
	if listLocal {
		opts = append(opts, config.WithoutRemote())
	}
	if _, err := loadConfigForCommand(ctx, nil, opts...); err != nil {
		fatal("Failed to load config", err)
	}
}

func runListRoot(cmd *cobra.Command, _ []string) {
	if !listPlacement {
		// Nothing was asked for: say what can be listed, as an error, not as a
		// help page a script cannot tell from success.
		fmt.Fprintf(os.Stderr, "Error: specify what to list: rules, context, skills, agents, commands or checks\n\nUsage:\n  %s <rules|context|skills|agents|commands|checks> [flags]\n\nRun \"%s --help\" for details and examples\n",
			cmd.CommandPath(), cmd.CommandPath())
		os.Exit(1)
	}
	cfg, err := loadConfigForCommand(cmdContext(), nil, pluginLoadOptions(true)...)
	if err != nil {
		fatal("Failed to load config", err)
	}
	report, err := generator.NewGenerator(cfg).PlacementReport(listProfile)
	if err != nil {
		fatal("Failed to resolve placement", err)
	}
	if listJSON {
		data, _ := jsondoc.Marshal(report) //nolint:errcheck // plain structs always marshal
		fmt.Print(string(data))
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
