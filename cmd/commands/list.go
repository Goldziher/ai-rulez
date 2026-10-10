package commands

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/crud"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/jsondoc"
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
	RunE: runListRoot,
}

var listRulesCmd = &cobra.Command{
	Use:   crud.ContentTypeRules,
	Short: "List all rules",
	Long: `List all rules in your .ai-rulez/ configuration.

You can filter by domain using --domain flag.`,
	Args: cobra.NoArgs,
	RunE: runListRules,
}

var listContextCmd = &cobra.Command{
	Use:   crud.ContentTypeContext,
	Short: "List all context files",
	Long: `List all context files in your .ai-rulez/ configuration.

You can filter by domain using --domain flag.`,
	Args: cobra.NoArgs,
	RunE: runListContext,
}

var listSkillsCmd = &cobra.Command{
	Use:   crud.ContentTypeSkills,
	Short: "List all skills",
	Long: `List all skills in your .ai-rulez/ configuration.

You can filter by domain using --domain flag.`,
	Args: cobra.NoArgs,
	RunE: runListSkills,
}

var listAgentsCmd = &cobra.Command{
	Use:   crud.ContentTypeAgents,
	Short: "List all agents",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runListItems(cmd, crud.ContentTypeAgents, "Agents", "agents")
	},
}

var listCommandsCmd = &cobra.Command{
	Use:   crud.ContentTypeCommands,
	Short: "List all commands",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runListItems(cmd, crud.ContentTypeCommands, "Commands", "commands")
	},
}

var listChecksCmd = &cobra.Command{
	Use:   crud.ContentTypeChecks,
	Short: "List all code-review checks",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runListItems(cmd, crud.ContentTypeChecks, "Checks", "checks")
	},
}

func init() {
	ListCmd.Flags().BoolVar(&listPlacement, "placement", false, "Report where each skill and command is placed: core or plugin-only, and which plugins bundle it")
	specProfile.String(ListCmd.Flags(), &listProfile, "Profile for --placement (default: from config or 'default')")
	addJSONFormat(ListCmd.Flags(), &listJSON, "j")
	ListCmd.AddCommand(listAgentsCmd)
	ListCmd.AddCommand(listCommandsCmd)
	ListCmd.AddCommand(listChecksCmd)
	specDomain.String(listChecksCmd.Flags(), &listDomain, "Filter by domain (shows all if not specified)")
	addJSONFormat(listChecksCmd.Flags(), &listJSON, "j")
	for _, c := range []*cobra.Command{listRulesCmd, listContextCmd, listSkillsCmd, listAgentsCmd, listCommandsCmd} {
		specDomain.String(c.Flags(), &listDomain, "Filter by domain (shows all if not specified)")
		addJSONFormat(c.Flags(), &listJSON, "j")
		specLocal.Bool(c.Flags(), &listLocal, "List the machine-local tree (.ai-rulez/local/)")
	}
	ListCmd.AddCommand(listRulesCmd)
	ListCmd.AddCommand(listContextCmd)
	ListCmd.AddCommand(listSkillsCmd)
}

func runListRules(cmd *cobra.Command, _ []string) error {
	return runListItems(cmd, crud.ContentTypeRules, "Rules", "rules")
}

func runListContext(cmd *cobra.Command, _ []string) error {
	return runListItems(cmd, crud.ContentTypeContext, "Context", "context files")
}

func runListSkills(cmd *cobra.Command, _ []string) error {
	return runListItems(cmd, crud.ContentTypeSkills, "Skills", "skills")
}

// outputListJSON writes the file list as a JSON document to w.
func outputListJSON(w io.Writer, files []crud.FileInfo) error {
	output := make([]map[string]interface{}, len(files))
	for i, file := range files {
		output[i] = map[string]interface{}{
			keyName:         file.Name,
			keyType:         file.Type,
			flagServeDomain: file.Domain,
			keyPath:         file.Path,
			"priority":      file.Priority,
			"targets":       file.Targets,
		}
	}
	data, err := jsondoc.Marshal(output)
	if err != nil {
		return failMsg("Failed to marshal JSON", err)
	}
	_, err = w.Write(data)
	return err //nolint:wrapcheck // a write failure
}

// outputListTable writes the file list, grouped by domain, as a human-readable table to w.
func outputListTable(w io.Writer, title string, files []crud.FileInfo) {
	writef(w, "%s:\n", title)

	byDomain := make(map[string][]crud.FileInfo)
	for _, file := range files {
		if file.Domain == "" {
			byDomain["(root)"] = append(byDomain["(root)"], file)
		} else {
			byDomain[file.Domain] = append(byDomain[file.Domain], file)
		}
	}
	domains := make([]string, 0, len(byDomain))
	for d := range byDomain {
		domains = append(domains, d)
	}
	sort.Strings(domains)

	for _, domain := range domains {
		writef(w, "  %s:\n", domain)
		for _, file := range byDomain[domain] {
			info := fmt.Sprintf("    • %s", file.Name)
			if file.Priority != "" {
				info += fmt.Sprintf(" [%s]", file.Priority)
			}
			if len(file.Targets) > 0 {
				info += fmt.Sprintf(" (targets: %v)", file.Targets)
			}
			writeln(w, info)
		}
	}
}

// runListItems lists one content type. The list is the command's result and
// goes to stdout, so -q never hides it; "none found" is a diagnostic.
func runListItems(cmd *cobra.Command, ftype, title, noun string) error {
	out := outFor(cmd)
	ctx := cmdContext()
	op, err := newContentOperator(listLocal)
	if err != nil {
		return failMsg("Failed to create CRUD operator", err)
	}
	if err := loadListedConfig(ctx); err != nil {
		return err
	}
	files, err := op.ListFiles(ctx, listDomain, ftype)
	if err != nil {
		return failMsg("Failed to list "+noun, err)
	}
	if listJSON {
		return outputListJSON(out.Stdout(), files)
	}
	if len(files) == 0 {
		out.Info("No %s found\n", noun)
		return nil
	}
	outputListTable(out.Stdout(), title, files)
	return nil
}

// loadListedConfig loads the project before a listing and fails (exit 1) when the
// configuration does not load (a syntax error, a V2/V3 config file), the code
// every other command uses for it. The lists read the content tree only; an
// include that cannot be resolved (nothing cached, no network is used) is not an
// error here but a warning from the load, which says what the listing leaves out.
// The machine-local tree (--local) gets the check without the include warnings.
func loadListedConfig(ctx context.Context) error {
	ctx = config.WithOfflineIncludes(config.WithUnresolvedIncludesTolerated(ctx))
	opts := []config.LoadOption{config.WithoutLocal()}
	if listLocal {
		opts = append(opts, config.WithoutRemote())
	}
	cfg, err := loadConfigForCommand(ctx, opts...)
	if err != nil {
		return failMsg("Failed to load config", err)
	}
	warnLegacyLayout(cfg.ConfigDir)
	return nil
}

func runListRoot(cmd *cobra.Command, _ []string) error {
	if !listPlacement {
		// Nothing was asked for: like every other group, say what can be listed.
		return cmd.Help()
	}
	out := outFor(cmd)
	cfg, err := loadConfigForCommand(cmdContext(), pluginLoadOptions(true)...)
	if err != nil {
		return failMsg("Failed to load config", err)
	}
	warnLegacyLayout(cfg.ConfigDir)
	report, err := generator.NewGenerator(cfg).PlacementReport(listProfile)
	if err != nil {
		return failMsg("Failed to resolve placement", err)
	}
	if listJSON {
		data, _ := jsondoc.Marshal(report) //nolint:errcheck // plain structs always marshal
		out.Result("%s", data)
		return nil
	}
	printPlacementReport(out.Stdout(), report)
	return nil
}

func printPlacementReport(w io.Writer, report *generator.PlacementReport) {
	writef(w, "Placement for profile %q\n\n", report.Profile)
	if len(report.Items) == 0 {
		writeln(w, "No skills or commands.")
		return
	}
	writef(w, "%-8s %-32s %-16s %-7s %s\n", "TYPE", "NAME", "DOMAIN", "WHERE", "PLUGINS")
	for _, it := range report.Items {
		domain := it.Domain
		if domain == "" {
			domain = "-"
		}
		plugins := "-"
		if len(it.Plugins) > 0 {
			plugins = strings.Join(it.Plugins, ", ")
		}
		writef(w, "%-8s %-32s %-16s %-7s %s\n", it.Type, it.Name, domain, it.Destination, plugins)
	}
	if issues := report.Issues(); len(issues) > 0 {
		writeln(w)
		for _, it := range issues {
			writef(w, "warning: %s %q is plugin-only but %s\n", it.Type, it.Name, it.Issue)
		}
	}
}
