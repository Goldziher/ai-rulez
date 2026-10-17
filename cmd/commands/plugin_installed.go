package commands

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	incl "github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// pluginFlags are the flags of the plugin subcommands.
type pluginFlags struct {
	marketplace       string
	entry             string
	source            string
	path              string
	ref               string
	layout            string
	include           []string
	exclude           []string
	namePrefix        string
	domain            string
	profiles          []string
	trust             string
	enableHooks       bool
	enableMCP         bool
	version           string
	tagPrefix         string
	includePrerelease bool
	minReleaseAge     string
	yes               bool
	asJSON            bool
}

// newPluginCmd builds `ai-rulez plugin`, which manages the plugins the project
// imports (generate --plugin builds a plugin from the project instead).
func newPluginCmd() *cobra.Command {
	f := &pluginFlags{}
	root := &cobra.Command{
		Use:   "plugin",
		Short: "Manage installed plugins imported from marketplaces or repositories",
		Long: `Manage the plugins whose skills, agents, commands, hooks and MCP servers the project
imports: Claude Code, Codex, Cursor, Gemini extension and Agent Plugins layouts, read through an
installed marketplace or straight from a repository or directory. Hooks stay off unless
--enable-hooks is given. To build a plugin from this project use "ai-rulez generate --plugin".`,
	}

	install := &cobra.Command{
		Use:   "install <name>",
		Short: "Install a plugin from a marketplace, a repository or a local directory",
		Long: `Add an [[installed_plugins]] entry to config.toml.

Use --from to install the plugin called <name> (or --entry) from an installed
marketplace, or --source for a plugin repository or directory. The plugin is fetched, scanned and
pinned by "ai-rulez lock"; its content is merged into the generated outputs.`,
		Args: cobra.ExactArgs(1),
		RunE: f.install,
	}
	flags := install.Flags()
	flags.StringVar(&f.marketplace, "from", "", "Name of an installed marketplace to install from")
	flags.StringVar(&f.entry, "entry", "", "Marketplace entry to install when it is not called like the plugin")
	flags.StringVar(&f.source, "source", "", "Git URL, GitHub owner/repo or local directory of the plugin")
	flags.StringVar(&f.path, "path", "", "Directory of the plugin inside the repository")
	flags.StringVar(&f.ref, "ref", "", "Branch, tag or commit")
	flags.StringVar(&f.layout, "layout", "", "Force the plugin layout: claude, codex, gemini, cursor or agent-plugins")
	flags.StringSliceVar(&f.include, "include", nil, "Import only these kinds: skills, agents, commands, hooks, mcp, rules")
	flags.StringSliceVar(&f.exclude, "exclude", nil, "Do not import these kinds")
	flags.StringVar(&f.namePrefix, "name-prefix", "", "Prefix for every imported name")
	flags.StringVar(&f.domain, "domain", "", "Place the imported content in this domain")
	flags.StringSliceVar(&f.profiles, "profile", nil, "Restrict the imported content to these profiles")
	flags.StringVar(&f.trust, "trust", "", "Scan level: error (default) or warn")
	flags.BoolVar(&f.enableHooks, "enable-hooks", false, "Import the plugin's hooks (they run commands on every machine)")
	flags.BoolVar(&f.enableMCP, "enable-mcp", true, "Import the plugin's MCP servers")
	flags.StringVar(&f.version, "version", "", "Version constraint for a plugin installed from --source")
	flags.StringVar(&f.tagPrefix, "tag-prefix", "", "Only tags with this prefix are version tags (needs --version)")
	flags.BoolVar(&f.includePrerelease, "include-prerelease", false, "Allow prerelease tags (needs --version)")
	flags.StringVar(&f.minReleaseAge, "min-release-age", "", "Hold back tags younger than this (needs --version)")
	addResultFormat(flags)

	remove := &cobra.Command{
		Use:   cmdUseRemoveName,
		Short: "Remove an installed plugin",
		Long:  `Remove an installed plugin from the configuration.`,
		Args:  cobra.ExactArgs(1),
		RunE:  f.remove,
	}
	addResultFormat(remove.Flags())
	addYesFlag(remove.Flags(), &f.yes, "Skip confirmation prompts")

	list := &cobra.Command{
		Use:   cmdUseList,
		Short: "List installed plugins",
		Long:  `List the plugins declared in config.toml.`,
		Args:  cobra.NoArgs,
		RunE:  f.list,
	}
	addJSONFormat(list.Flags(), &f.asJSON, "j")

	update := &cobra.Command{
		Use:   useUpdate,
		Short: "Re-pin installed plugins in ai-rulez.lock to their current remote commit",
		Long: `Resolve the named plugins (all of them without names) from their remote and record the
commit and content digest in ai-rulez.lock. A plugin that uses a version range keeps a pin that
still satisfies it: use "ai-rulez update --kind plugin" to move range pins to the newest allowed tag.`,
		RunE: func(_ *cobra.Command, args []string) error {
			lockCheck = false
			return exitStatus(runLockFor(lockfile.KindPlugin, args))
		},
	}

	root.AddCommand(install, remove, list, update)
	return root
}

func (f *pluginFlags) install(cmd *cobra.Command, args []string) error {
	op, err := newWritingOperator(false)
	if err != nil {
		return failMsg("Failed to create CRUD operator", err)
	}
	entry := &config.InstalledPluginConfig{
		Name: args[0], Marketplace: f.marketplace, Plugin: f.entry, Source: f.source, Path: f.path, Ref: f.ref, Format: f.layout,
		Include: f.include, Exclude: f.exclude, NamePrefix: f.namePrefix, Domain: f.domain, Profiles: f.profiles, Trust: f.trust,
		EnableHooks: f.enableHooks, Version: f.version, TagPrefix: f.tagPrefix, IncludePrerelease: f.includePrerelease,
		MinReleaseAge: f.minReleaseAge,
	}
	if !f.enableMCP {
		off := false
		entry.EnableMCP = &off
	}
	if err := op.InstallPlugin(cmdContext(), entry); err != nil {
		return failMsg("Failed to install plugin", err)
	}
	from := f.source
	if from == "" {
		from = f.marketplace
	}
	out := outFor(cmd)
	if !out.JSON() {
		out.Info("Installed plugin %s (from %s) in %s\n", args[0], incl.RedactURL(from), displayPath(op.ConfigFile()))
	}
	return reportChange(out, out.JSON(), changeResult{
		Status: statusCreated, Type: "installed_plugin", Name: args[0], Source: incl.RedactURL(from), Path: op.ConfigFile(),
	}, false)
}

func (f *pluginFlags) remove(cmd *cobra.Command, args []string) error {
	if err := confirmRemovalUnlessYes(f.yes, "installed plugin", args[0], "Operation canceled"); err != nil {
		return fail(err)
	}
	op, err := newWritingOperator(false)
	if err != nil {
		return failMsg("Failed to create CRUD operator", err)
	}
	if err := op.RemovePlugin(cmdContext(), args[0]); err != nil {
		return failMsg("Failed to remove plugin", err)
	}
	out := outFor(cmd)
	return reportChange(out, out.JSON(), changeResult{Status: statusRemoved, Type: "installed_plugin", Name: args[0], Path: op.ConfigFile()}, false)
}

func (f *pluginFlags) list(cmd *cobra.Command, _ []string) error {
	out := outFor(cmd)
	op, err := newContentOperator(false)
	if err != nil {
		return failMsg("Failed to create CRUD operator", err)
	}
	list, err := op.ListPlugins(cmdContext())
	if err != nil {
		return failMsg("Failed to list plugins", err)
	}
	if f.asJSON {
		rows := make([]map[string]interface{}, len(list))
		for i := range list {
			p := &list[i]
			rows[i] = map[string]interface{}{keyName: p.Name, "marketplace": p.Marketplace, keySource: incl.RedactURL(p.Source), keyPath: p.Path,
				keyRef: p.RequestedRef(), "enable_hooks": p.EnableHooks, "include": p.Include, "exclude": p.Exclude}
		}
		return writeListJSON(out.Stdout(), rows)
	}
	if len(list) == 0 {
		out.Info("No installed plugins found\n")
		return nil
	}
	out.Result("Installed plugins:\n")
	for i := range list {
		p := &list[i]
		from := "marketplace " + p.Marketplace
		if p.Source != "" {
			from = incl.RedactURL(p.Source)
		}
		out.Result("  • %s\n    From: %s\n", p.Name, from)
		if kinds := pluginKinds(p); kinds != "" {
			out.Result("    Kinds: %s\n", kinds)
		}
		if p.EnableHooks {
			out.Result("    Hooks: enabled\n")
		}
	}
	return nil
}

func pluginKinds(p *config.InstalledPluginConfig) string {
	var out []string
	for _, k := range config.PluginKinds() {
		if p.Wants(k) {
			out = append(out, k)
		}
	}
	if len(out) == len(config.PluginKinds()) {
		return ""
	}
	return strings.Join(out, ", ")
}
