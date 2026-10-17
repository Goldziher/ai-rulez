package commands

import (
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	incl "github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// marketplaceFlags are the flags of the marketplace subcommands.
type marketplaceFlags struct {
	source            string
	path              string
	ref               string
	version           string
	tagPrefix         string
	includePrerelease bool
	minReleaseAge     string
	trust             string
	yes               bool
	asJSON            bool
}

// newMarketplaceCmd builds `ai-rulez marketplace`, which manages the plugin
// marketplaces the project reads.
func newMarketplaceCmd() *cobra.Command {
	f := &marketplaceFlags{}
	root := &cobra.Command{
		Use:   "marketplace",
		Short: "Manage installed plugin marketplaces",
		Long: `Manage the plugin marketplaces the project reads: a Claude Code
(.claude-plugin/marketplace.json), Codex (.agents/plugins/marketplace.json) or Cursor
(.cursor-plugin/marketplace.json) index in a repository, a local directory or at an https URL.
A marketplace installs nothing by itself: "ai-rulez plugin install --from" imports a plugin from one.`,
	}

	add := &cobra.Command{
		Use:   "add <name>",
		Short: "Add a plugin marketplace from a repository, a local directory or an https URL",
		Long: `Add an [[installed_marketplaces]] entry to config.toml.

--source is a git URL, a GitHub owner/repo, a local directory (./path, inside the project)
or an https URL of a marketplace.json. A repository can be pinned with --ref or a version
range (--version ^1.2); "ai-rulez lock" records the commit and the tree digest.`,
		Args: cobra.ExactArgs(1),
		RunE: f.add,
	}
	flags := add.Flags()
	flags.StringVar(&f.source, "source", "", "Git URL, GitHub owner/repo, local directory or https URL of a marketplace.json (required)")
	flags.StringVar(&f.path, "path", "", "Directory of the repository that holds the index (default: the root)")
	flags.StringVar(&f.ref, "ref", "", "Branch, tag or commit")
	flags.StringVar(&f.version, "version", "", "Version constraint resolved against the repository's tags (^1.2, ~2.1.0)")
	flags.StringVar(&f.tagPrefix, "tag-prefix", "", "Only tags with this prefix are version tags (needs --version)")
	flags.BoolVar(&f.includePrerelease, "include-prerelease", false, "Allow prerelease tags (needs --version)")
	flags.StringVar(&f.minReleaseAge, "min-release-age", "", "Hold back tags younger than this (7d, 12h; needs --version)")
	flags.StringVar(&f.trust, "trust", "", "Scan level of the plugins read through it: error (default) or warn")
	_ = add.MarkFlagRequired("source") //nolint:errcheck // the flag exists
	addResultFormat(flags)

	remove := &cobra.Command{
		Use:   cmdUseRemoveName,
		Short: "Remove an installed marketplace",
		Long:  `Remove an installed marketplace from the configuration. A marketplace an installed plugin still names is refused.`,
		Args:  cobra.ExactArgs(1),
		RunE:  f.remove,
	}
	addResultFormat(remove.Flags())
	addYesFlag(remove.Flags(), &f.yes, "Skip confirmation prompts")

	list := &cobra.Command{
		Use:   cmdUseList,
		Short: "List installed marketplaces",
		Long:  `List the marketplaces declared in config.toml.`,
		Args:  cobra.NoArgs,
		RunE:  f.list,
	}
	addJSONFormat(list.Flags(), &f.asJSON, "j")

	update := &cobra.Command{
		Use:   useUpdate,
		Short: "Re-pin installed marketplaces (and their plugins) in ai-rulez.lock to the current remote commit",
		Long: `Resolve the named marketplaces (all of them without names) from their remote and
record the commit and content digest in ai-rulez.lock; the plugins installed from them are
re-pinned with them. A marketplace that uses a version range keeps a pin that still satisfies it:
use "ai-rulez update --kind marketplace" to move range pins to the newest allowed tag.`,
		RunE: func(_ *cobra.Command, args []string) error {
			lockCheck = false
			return exitStatus(runLockFor(lockfile.KindMarketplace, args))
		},
	}

	root.AddCommand(add, remove, list, update)
	return root
}

func (f *marketplaceFlags) add(cmd *cobra.Command, args []string) error {
	op, err := newWritingOperator(false)
	if err != nil {
		return failMsg("Failed to create CRUD operator", err)
	}
	entry := &config.InstalledMarketplaceConfig{
		Name: args[0], Source: f.source, Path: f.path, Ref: f.ref, Version: f.version, TagPrefix: f.tagPrefix,
		IncludePrerelease: f.includePrerelease, MinReleaseAge: f.minReleaseAge, Trust: f.trust,
	}
	if err := op.AddMarketplace(cmdContext(), entry); err != nil {
		return failMsg("Failed to add marketplace", err)
	}
	out := outFor(cmd)
	if !out.JSON() {
		out.Info("Added marketplace %s (%s) to %s\n", args[0], incl.RedactURL(f.source), displayPath(op.ConfigFile()))
	}
	return reportChange(out, out.JSON(), changeResult{
		Status: statusCreated, Type: "installed_marketplace", Name: args[0], Source: incl.RedactURL(f.source), Path: op.ConfigFile(),
	}, false)
}

func (f *marketplaceFlags) remove(cmd *cobra.Command, args []string) error {
	if err := confirmRemovalUnlessYes(f.yes, "installed marketplace", args[0], "Operation canceled"); err != nil {
		return fail(err)
	}
	op, err := newWritingOperator(false)
	if err != nil {
		return failMsg("Failed to create CRUD operator", err)
	}
	if err := op.RemoveMarketplace(cmdContext(), args[0]); err != nil {
		return failMsg("Failed to remove marketplace", err)
	}
	out := outFor(cmd)
	return reportChange(out, out.JSON(), changeResult{Status: statusRemoved, Type: "installed_marketplace", Name: args[0], Path: op.ConfigFile()}, false)
}

func (f *marketplaceFlags) list(cmd *cobra.Command, _ []string) error {
	out := outFor(cmd)
	op, err := newContentOperator(false)
	if err != nil {
		return failMsg("Failed to create CRUD operator", err)
	}
	list, err := op.ListMarketplaces(cmdContext())
	if err != nil {
		return failMsg("Failed to list marketplaces", err)
	}
	if f.asJSON {
		rows := make([]map[string]interface{}, len(list))
		for i := range list {
			m := &list[i]
			rows[i] = map[string]interface{}{keyName: m.Name, keySource: incl.RedactURL(m.Source), keyPath: m.Path, keyRef: m.RequestedRef(),
				keyType: config.ClassifyPluginSource(m.Source)}
		}
		return writeListJSON(out.Stdout(), rows)
	}
	if len(list) == 0 {
		out.Info("No installed marketplaces found\n")
		return nil
	}
	out.Result("Installed marketplaces:\n")
	for i := range list {
		m := &list[i]
		out.Result("  • %s [%s]\n    Source: %s\n", m.Name, config.ClassifyPluginSource(m.Source), incl.RedactURL(m.Source))
		if ref := m.RequestedRef(); ref != "" {
			out.Result("    Ref: %s\n", ref)
		}
	}
	return nil
}
