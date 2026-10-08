package generator

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/plugin"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/presets"   // Register remaining legacy preset generators
	"github.com/Goldziher/ai-rulez/v5/internal/generator/providers" // Register DSL-backed preset generators (overrides legacy registrations where they overlap)
)

// GeneratePlugin packages the project into distributable plugin bundles plus a
// marketplace index for the runtimes named in the [plugin] block. Unlike the
// normal generate path, plugin outputs are written verbatim (RawContent) and do
// not participate in the generated-manifest / stale-file bookkeeping.
func (g *Generator) GeneratePlugin(profile string) error {
	_, err := g.GeneratePluginFiles(profile)
	return err
}

// GeneratePluginFiles is GeneratePlugin returning the number of files written,
// directories excluded.
func (g *Generator) GeneratePluginFiles(profile string) (int, error) {
	g.diagnostics() // the run's warnings share one collector from the start
	outputs, err := g.collectPluginOutputs(profile)
	if err != nil {
		return 0, err
	}
	stale, err := g.stalePluginDirs()
	if err != nil {
		return 0, err
	}
	obsolete, err := g.planPluginPrune(outputs)
	if err != nil {
		return 0, err
	}
	if err := g.writeOutputs(outputs); err != nil {
		return 0, err
	}
	g.removeStalePluginDirs(stale)
	g.applyPluginPrune(obsolete)
	written := 0
	for _, output := range outputs {
		if !output.IsDir {
			written++
		}
	}
	g.log().Info("Plugin generation complete", "files", written)
	return written, nil
}

// ErrPluginNotGenerated is returned by VerifyPlugin when none of the expected
// plugin files exist on disk, so a caller can tell "never generated" from "stale".
var ErrPluginNotGenerated = errors.New("plugin bundle not generated")

// pluginNotGeneratedError carries the actionable message once; errors.Is
// matches ErrPluginNotGenerated so --if-generated can still skip it.
type pluginNotGeneratedError struct{}

func (pluginNotGeneratedError) Error() string {
	return "plugin bundle not generated; run ai-rulez generate --plugin"
}

func (pluginNotGeneratedError) Is(target error) bool {
	return target == ErrPluginNotGenerated || target == ErrPluginDrift
}

// ErrPluginDrift is matched by every VerifyPlugin failure that means the
// bundle on disk differs from what generate would write (missing, stale,
// obsolete or hash-mismatched files), as opposed to the check not being able
// to run. verify --plugin maps it to exit 2.
var ErrPluginDrift = errors.New("plugin bundle differs from its sources")

type pluginDriftError struct{ err error }

func (e pluginDriftError) Error() string      { return e.err.Error() }
func (e pluginDriftError) Unwrap() error      { return e.err }
func (pluginDriftError) Is(target error) bool { return target == ErrPluginDrift }

func asPluginDrift(err error) error {
	if err == nil {
		return nil
	}
	return pluginDriftError{err}
}

// checkPluginGenerated fails with ErrPluginNotGenerated, and the command that
// fixes it, when no expected plugin file exists.
func checkPluginGenerated(expected []config.OutputFile) error {
	files, present := 0, 0
	for _, output := range expected {
		if output.IsDir {
			continue
		}
		files++
		if _, err := os.Stat(output.Path); err == nil {
			present++
		}
	}
	if files > 0 && present == 0 {
		return oops.Hint("Run ai-rulez generate --plugin, then commit the bundle (or pass --if-generated to skip verification until it exists)").
			Wrap(pluginNotGeneratedError{})
	}
	return nil
}

// VerifyPlugin verifies the generated plugin bundles against their provenance
// sidecars without regenerating or modifying files.
func (g *Generator) VerifyPlugin(profile string) error {
	g.diagnostics() // the run's warnings share one collector from the start
	expected, err := g.collectPluginOutputs(profile)
	if err != nil {
		return oops.Wrapf(err, "render expected plugin outputs")
	}
	if err := checkPluginGenerated(expected); err != nil {
		return err
	}
	obsolete, err := g.planPluginPrune(expected)
	if err != nil {
		return err
	}
	if len(obsolete) > 0 {
		return asPluginDrift(obsoleteFilesError(obsolete))
	}
	if err := verifyPluginOutputs(expected); err != nil {
		return asPluginDrift(err)
	}
	stale, err := g.stalePluginDirs()
	if err != nil {
		return err
	}
	if len(stale) > 0 {
		return asPluginDrift(oops.With("dirs", strings.Join(stale, ", ")).
			Hint("Run ai-rulez generate --plugin to remove the plugin directories of domains that no longer exist").
			Errorf("stale generated plugin directory"))
	}
	if marketplace := g.config.Marketplace; marketplace != nil && len(marketplace.Members) > 0 {
		for _, member := range marketplace.Members {
			if err := plugin.VerifyProvenance(filepath.Join(g.config.BaseDir, member)); err != nil {
				return asPluginDrift(oops.With("member", member).Wrapf(err, "verify member plugin bundle"))
			}
		}
	}
	if marketplace := g.config.Marketplace; marketplace != nil && marketplace.HasDomainPlugins() {
		return asPluginDrift(g.verifyDomainPluginProvenance(expected))
	}
	return asPluginDrift(plugin.VerifyProvenance(g.config.BaseDir))
}

// verifyPluginOutputs compares every expected plugin file with what is on disk.
func verifyPluginOutputs(expected []config.OutputFile) error {
	for _, output := range expected {
		if output.IsDir {
			continue
		}
		actual, readErr := os.ReadFile(output.Path)
		if readErr != nil {
			return oops.With("path", output.Path).
				Hint("Run ai-rulez generate --plugin to restore the missing file").
				Wrapf(readErr, "read generated plugin output")
		}
		expectedBytes := output.RawContent
		if expectedBytes == nil {
			expectedBytes = []byte(output.Content)
		}
		if !bytes.Equal(actual, expectedBytes) {
			return oops.With("path", output.Path).
				Hint("Run ai-rulez generate --plugin and commit the regenerated output").
				Errorf("generated plugin output is stale")
		}
	}
	return nil
}

// verifyDomainPluginProvenance verifies every bundle root the domain-plugin
// marketplace wrote: the marketplace root and each plugin directory.
func (g *Generator) verifyDomainPluginProvenance(expected []config.OutputFile) error {
	for _, output := range expected {
		if filepath.Base(output.Path) != plugin.ProvenanceFileName {
			continue
		}
		dir := filepath.Dir(output.Path)
		if err := plugin.VerifyProvenance(dir); err != nil {
			return oops.With("bundle", dir).Wrapf(err, "verify domain plugin bundle")
		}
	}
	return nil
}

// DryRunPlugin returns the plugin generation plan without writing files.
func (g *Generator) DryRunPlugin(profile string) ([]string, error) {
	g.diagnostics() // the run's warnings share one collector from the start
	outputs, err := g.collectPluginOutputs(profile)
	if err != nil {
		return nil, err
	}
	lines := make([]string, 0, len(outputs)+1)
	lines = append(lines, "plugin bundle:")
	for _, output := range outputs {
		lines = append(lines, "write-file: "+g.convertToRelativePath(g.absOutputPath(output.Path)))
	}
	stale, err := g.stalePluginDirs()
	if err != nil {
		return nil, err
	}
	for _, dir := range stale {
		lines = append(lines, "delete-stale: "+g.convertToRelativePath(dir))
	}
	obsolete, err := g.planPluginPrune(outputs)
	if err != nil {
		return nil, err
	}
	for _, item := range obsolete {
		if item.reason == "" {
			lines = append(lines, "delete-stale: "+item.rel)
		} else {
			lines = append(lines, "keep-obsolete: "+item.rel+" ("+item.reason+")")
		}
	}
	return lines, nil
}

// collectPluginOutputs resolves the content tree and MCP servers, builds the
// plugin manifest, and renders all requested runtime bundles + marketplace. When
// the config is a marketplace root ([marketplace].members or domain plugins), it
// instead renders each plugin's bundle plus the aggregate marketplace index.
func (g *Generator) collectPluginOutputs(profile string) ([]config.OutputFile, error) {
	if mkt := g.config.Marketplace; mkt != nil && (len(mkt.Members) > 0 || mkt.HasDomainPlugins()) {
		return g.collectMonorepoOutputs(mkt, profile)
	}

	if g.config.Plugin == nil {
		return nil, oops.
			Hint("Add a [plugin] block (or a [marketplace] with members) to your config").
			Errorf("no [plugin] block configured; nothing to generate with --plugin")
	}

	manifest, err := g.buildPluginManifest(profile)
	if err != nil {
		return nil, err
	}
	return plugin.Generate(manifest, g.config.BaseDir)
}

// buildPluginManifest resolves the content tree and MCP servers for the active
// profile and builds the plugin manifest for the current config.
func (g *Generator) buildPluginManifest(profile string) (*plugin.Manifest, error) {
	if err := g.resolveMCPEnvForPlugin(); err != nil {
		return nil, err
	}

	activeProfile := g.resolveProfile(profile)
	contentTree, err := g.getContentForProfile(activeProfile)
	if err != nil {
		return nil, err
	}
	if g.config.Plugin.ContentRoot != "" {
		contentDir := filepath.Join(g.config.BaseDir, g.config.Plugin.ContentRoot)
		contentTree, err = config.ScanContentTreeIn(g.ctx, g.config.ViewFor(contentDir), contentDir)
		if err != nil {
			return nil, oops.With("content_root", g.config.Plugin.ContentRoot).Wrapf(err, "scan plugin content")
		}
	}

	mcpServers := g.collectMCPServersForContent(contentTree, "")

	tempCfg := *g.config
	tempCfg.Content = contentTree
	tempCfg.MCPServers = mcpServers

	return plugin.BuildManifest(&tempCfg, contentTree)
}

// warnIgnoredPlugins warns that [[plugins]] no longer writes any file. v4 wrote
// .claude/plugins.json and .codex/plugins.json, which no tool reads. Claude Code
// records plugins in .claude/settings.json and Codex in .codex/config.toml.
func (g *Generator) warnIgnoredPlugins() {
	if len(g.config.Plugins) == 0 {
		return
	}
	g.log().Warn("[[plugins]] has no effect in v5 and can be removed. " +
		"For Claude Code set [claude.settings] manage = true with enable_plugins (writes enabledPlugins in .claude/settings.json); " +
		"for Codex enable plugins with [plugins.\"name@marketplace\"] enabled = true in .codex/config.toml")
}

// warnLegacyFiles warns about 4.x files beside config.toml that v5 no longer
// reads: without it their settings and MCP servers vanish without a word.
func (g *Generator) warnLegacyFiles() {
	for _, name := range config.LegacyFilesBeside(g.manifestDir()) {
		g.log().Warn(name + " is no longer read by v5 and is ignored; run `" + config.MigrateCommandHint + "` to fold it into config.toml, then delete it")
	}
}

// stalePluginDirs lists the generated domain-plugin directories whose plugin is
// no longer planned (its domain disappeared, or its declaration was removed).
// Only directories carrying ai-rulez's provenance sidecar qualify. The plan is
// made over the unfiltered content tree: a profile that leaves a domain out
// must not delete the plugin another profile generated for it.
func (g *Generator) stalePluginDirs() ([]string, error) {
	mkt := g.config.Marketplace
	if mkt == nil || !mkt.HasDomainPlugins() {
		return nil, nil
	}
	planned, err := plugin.PlanDomainPlugins(g.config, g.config.Content)
	if err != nil {
		return nil, err
	}
	keep := make(map[string]bool, len(planned))
	for i := range planned {
		keep[planned[i].Name] = true
	}
	return plugin.StalePluginDirs(g.marketplaceRoot(mkt), keep)
}

// removeStalePluginDirs deletes the generated files of each stale plugin
// directory and nothing else. A failure is reported and does not stop the run.
func (g *Generator) removeStalePluginDirs(dirs []string) {
	for _, dir := range dirs {
		kept, err := plugin.RemoveGeneratedPluginDir(dir)
		rel := g.convertToRelativePath(dir)
		switch {
		case err != nil:
			g.log().Warn("Could not remove a stale plugin directory", "dir", rel, "error", err)
		case len(kept) > 0:
			g.log().Warn("Removed the generated files of a stale plugin directory; files that are not generated were kept",
				"dir", rel, "kept", strings.Join(kept, ", "))
		default:
			g.log().Info("Removed stale plugin directory", "dir", rel)
		}
	}
}

// marketplaceRoot is the directory the marketplace index is written to:
// [marketplace].output_dir for domain plugins, the project root otherwise.
func (g *Generator) marketplaceRoot(mkt *config.MarketplaceAuthoring) string {
	if mkt.OutputDir == "" {
		return g.config.BaseDir
	}
	return filepath.Join(g.config.BaseDir, mkt.OutputDir)
}

// collectMonorepoOutputs renders every member plugin under its source directory,
// every domain plugin under <output_dir>/plugins/<name>, and emits the aggregate
// root marketplace index. Each member is an independent ai-rulez project loaded
// from <baseDir>/<member>.
func (g *Generator) collectMonorepoOutputs(mkt *config.MarketplaceAuthoring, profile string) ([]config.OutputFile, error) {
	var outputs []config.OutputFile
	entries := make([]plugin.MemberEntry, 0, len(mkt.Members))

	for _, member := range mkt.Members {
		memberOutputs, entry, err := g.collectMemberOutputs(member)
		if err != nil {
			return nil, err
		}
		outputs = append(outputs, memberOutputs...)
		entries = append(entries, entry)
	}
	codexEntries := slices.Clone(entries) // members are always Codex-capable

	root := g.marketplaceRoot(mkt)
	codex := len(mkt.Members) > 0
	if mkt.HasDomainPlugins() {
		bundles, domainEntries, err := g.collectDomainPluginOutputs(profile, root)
		if err != nil {
			return nil, err
		}
		outputs = append(outputs, bundles...)
		entries = append(entries, domainEntries...)
		for i := range domainEntries {
			if domainEntries[i].Codex {
				codex = true
				codexEntries = append(codexEntries, domainEntries[i])
			}
		}
	}

	market := plugin.ResolveMarketInfo(mkt)
	if market.Owner == nil && mkt.HasDomainPlugins() && g.config.Plugin != nil {
		market.Owner = g.config.Plugin.Author // Claude Code requires an owner
	}
	marketplaceOutput, err := plugin.RenderMonorepoMarketplace(market, entries, root)
	if err != nil {
		return nil, oops.Wrapf(err, "render monorepo marketplace")
	}
	rootFiles := []config.OutputFile{marketplaceOutput}
	extraIndexes, err := g.renderExtraMarketplaces(mkt, market, entries, codexEntries, root, codex)
	if err != nil {
		return nil, err
	}
	rootFiles = append(rootFiles, extraIndexes...)
	rootOutputs, err := plugin.AddProvenance(rootFiles, root)
	if err != nil {
		return nil, oops.Wrapf(err, "add marketplace provenance")
	}
	return append(outputs, rootOutputs...), nil
}

// renderExtraMarketplaces renders the non-Claude marketplace indexes of a
// monorepo or domain-plugin root: the Codex index when some plugin ships a Codex
// bundle, and the Cursor index when [marketplace] cursor_index is set.
func (g *Generator) renderExtraMarketplaces(mkt *config.MarketplaceAuthoring, market plugin.MarketInfo, entries, codexEntries []plugin.MemberEntry, root string, codex bool) ([]config.OutputFile, error) {
	var files []config.OutputFile
	if codex {
		out, err := plugin.RenderCodexMonorepoMarketplace(market, codexEntries, root)
		if err != nil {
			return nil, oops.Wrapf(err, "render Codex monorepo marketplace")
		}
		files = append(files, out)
	}
	if mkt.CursorIndex {
		out, err := plugin.RenderCursorMarketplace(g.log(), market, entries, root)
		if err != nil {
			return nil, oops.Wrapf(err, "render Cursor marketplace")
		}
		files = append(files, out)
	}
	return files, nil
}

// withCatalogSkill adds the generated plugin-catalog skill to a copy of tree
// when [marketplace.catalog_skill] is enabled. A root skill of the same name
// wins. The catalog goes to the preset outputs only, never into plugin bundles.
func (g *Generator) withCatalogSkill(tree *config.ContentTree) (*config.ContentTree, error) {
	mkt := g.config.Marketplace
	if mkt == nil || mkt.CatalogSkill == nil || !mkt.CatalogSkill.Enabled {
		return tree, nil
	}
	plan, err := plugin.PlanDomainPlugins(g.config, tree)
	if err != nil {
		return nil, oops.Wrapf(err, "plan domain plugins for the catalog skill")
	}
	skill := plugin.CatalogSkill(g.config, plan)
	for i := range tree.Skills {
		if tree.Skills[i].Name == skill.Name {
			g.log().Warn("A skill named like the catalog skill exists; not generating the catalog", "skill", skill.Name)
			return tree, nil
		}
	}
	withCatalog := *tree
	withCatalog.Skills = append(slices.Clone(tree.Skills), skill)
	return &withCatalog, nil
}

// collectMemberOutputs renders one monorepo member and returns its marketplace
// entry.
func (g *Generator) collectMemberOutputs(member string) ([]config.OutputFile, plugin.MemberEntry, error) {
	memberDir := filepath.Join(g.config.BaseDir, member)
	// The member is read the way the root was: through its workspace, host and
	// registry, within the run's context.
	loadOpts := []config.LoadOption{
		config.WithoutLocal(),
		config.WithResolvers(g.config.Resolve),
		config.WithHost(g.host()),
		config.WithRegistry(g.config.Registry),
		config.WithPolicy(g.config.Policy()),
		config.WithLockPolicy(g.config.LockPolicy),
	}
	if g.config.Workspace != nil {
		loadOpts = append(loadOpts, config.WithWorkspace(g.config.Workspace))
	}
	memberCfg, err := config.LoadConfig(g.ctx, memberDir, loadOpts...)
	if err != nil {
		return nil, plugin.MemberEntry{}, oops.With("member", member).Wrapf(err, "load monorepo member config")
	}
	if err := g.nestedPolicy(memberCfg, true); err != nil {
		return nil, plugin.MemberEntry{}, oops.With("member", member).Wrapf(err, "monorepo member config")
	}
	memberCfg.Diag = g.config.Diag
	if memberCfg.Plugin == nil {
		return nil, plugin.MemberEntry{}, oops.
			With("member", member).
			Hint("Each monorepo member must define its own [plugin] block").
			Errorf("monorepo member %q has no [plugin] block", member)
	}
	if len(g.memberRuntimes) > 0 {
		if err := limitMemberRuntimes(memberCfg, member, g.memberRuntimes); err != nil {
			return nil, plugin.MemberEntry{}, err
		}
	}

	manifest, err := NewGenerator(memberCfg).buildPluginManifest("")
	if err != nil {
		return nil, plugin.MemberEntry{}, oops.With("member", member).Wrapf(err, "build member manifest")
	}
	memberOutputs, err := plugin.GenerateMember(manifest, memberCfg.BaseDir)
	if err != nil {
		return nil, plugin.MemberEntry{}, oops.With("member", member).Wrapf(err, "generate member bundle")
	}
	return memberOutputs, plugin.MemberEntry{
		Name:        manifest.Name,
		Description: manifest.Description,
		Source:      "./" + filepath.ToSlash(member),
		Category:    manifest.Category,
	}, nil
}

// MemberRuntimeError is returned when a [marketplace] member ships none of the runtimes a bundle was limited to
// with WithMemberRuntimes: publishing less than the marketplace index lists must never be silent.
type MemberRuntimeError struct {
	Member    string
	Requested []string
	Ships     []string
}

func (e *MemberRuntimeError) Error() string {
	return fmt.Sprintf("marketplace member %s ships %s, none of the requested runtimes %s", e.Member, strings.Join(e.Ships, ", "), strings.Join(e.Requested, ", "))
}

// WithMemberRuntimes limits the bundle of every [marketplace] member to the given runtimes (a member keeps only
// those of its own [plugin] runtimes that are listed; one that keeps none is a *MemberRuntimeError). It returns g.
func (g *Generator) WithMemberRuntimes(runtimes []string) *Generator {
	g.memberRuntimes = append([]string(nil), runtimes...)
	return g
}

// limitMemberRuntimes narrows the member's [plugin] runtimes to the requested ones.
func limitMemberRuntimes(memberCfg *config.Config, member string, requested []string) error {
	ships := memberCfg.Plugin.ResolvedRuntimes()
	keep := make([]string, 0, len(ships))
	for _, r := range ships {
		if slices.Contains(requested, r) {
			keep = append(keep, r)
		}
	}
	if len(keep) == 0 {
		return &MemberRuntimeError{Member: member, Requested: requested, Ships: slices.Clone(ships)}
	}
	p := *memberCfg.Plugin
	p.Runtimes = keep
	memberCfg.Plugin = &p
	return nil
}

// collectDomainPluginOutputs renders every planned domain plugin under
// <root>/plugins/<name> and returns their marketplace entries.
func (g *Generator) collectDomainPluginOutputs(profile, root string) ([]config.OutputFile, []plugin.MemberEntry, error) {
	planned, err := g.planDomainPlugins(profile)
	if err != nil {
		return nil, nil, err
	}
	var outputs []config.OutputFile
	entries := make([]plugin.MemberEntry, 0, len(planned))
	for i := range planned {
		p := &planned[i]
		bundle, err := plugin.GenerateMember(plugin.BuildDomainManifest(g.config, p), p.Dir(root))
		if err != nil {
			return nil, nil, oops.With("plugin", p.Name).Wrapf(err, "generate domain plugin bundle")
		}
		outputs = append(outputs, bundle...)
		entries = append(entries, plugin.MemberEntryFor(p))
	}
	return outputs, entries, nil
}

// warnUnbundledPluginOnly warns about skills and commands that placement keeps
// out of .claude/skills although no configured plugin bundles them, which would
// make them unreachable.
func (g *Generator) warnUnbundledPluginOnly(tree *config.ContentTree) {
	if g.config.Placement == nil && !hasPlacementFrontmatter(tree) {
		return
	}
	var plugged []struct{ typ, name string }
	collect := func(typ string, items []config.ContentFile) {
		for i := range items {
			if providers.ResolvePlacement(g.config, typ, items[i], tree) == config.PlacementPlugin {
				plugged = append(plugged, struct{ typ, name string }{typ, items[i].Name})
			}
		}
	}
	collect(providers.OutputTypeSkills, presets.AllSkills(tree))
	collect(providers.OutputTypeCommands, presets.AllCommands(tree))
	if len(plugged) == 0 {
		return
	}
	plan, err := plugin.PlanDomainPlugins(g.config, tree)
	if err != nil {
		return // reported by the plugin run itself
	}
	skills, commands := plugin.BundledNames(g.config, tree, plan)
	var missing []string
	for _, item := range plugged {
		bundled := skills
		if item.typ == providers.OutputTypeCommands {
			bundled = commands
		}
		if !bundled[item.name] {
			missing = append(missing, item.name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		g.log().Warn("Plugin-only skills or commands are in no plugin and are not generated anywhere",
			"count", len(missing), "names", strings.Join(missing, ", "))
	}
}

func hasPlacementFrontmatter(tree *config.ContentTree) bool {
	has := func(items []config.ContentFile) bool {
		for i := range items {
			if items[i].Metadata != nil && items[i].Metadata.Extra["placement"] != "" {
				return true
			}
		}
		return false
	}
	if has(tree.Skills) || has(tree.Commands) {
		return true
	}
	for _, d := range tree.Domains {
		if has(d.Skills) || has(d.Commands) {
			return true
		}
	}
	return false
}

// planDomainPlugins resolves the domain plugins from the content tree of the
// active profile.
func (g *Generator) planDomainPlugins(profile string) ([]plugin.PlannedPlugin, error) {
	tree, err := g.getContentForProfile(g.resolveProfile(profile))
	if err != nil {
		return nil, err
	}
	return plugin.PlanDomainPlugins(g.config, tree)
}
