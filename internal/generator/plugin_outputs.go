package generator

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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
	outputs, obsolete, err := g.resolvePluginOutputs(profile)
	if err != nil {
		return 0, err
	}
	if err := g.refuseSymlinkedOutputs(outputs); err != nil {
		return 0, err
	}
	stale, err := g.stalePluginDirs()
	if err != nil {
		return 0, err
	}
	if err := g.writeOutputs(outputs); err != nil {
		return 0, err
	}
	g.removeStalePluginDirs(stale)
	g.applyPluginPrune(obsolete)
	if err := g.removeUnrecordedSkillFiles(outputs, obsolete); err != nil {
		return 0, err
	}
	written := 0
	for _, output := range outputs {
		if !output.IsDir {
			written++
		}
	}
	g.log().Info("Plugin generation complete", "files", written)
	return written, nil
}

// GenerateMemberFiles renders this plugin-authoring root as the marketplace member
// that an enclosing root lists: the same bundle `generate --plugin` writes for it,
// without the marketplace index the enclosing root owns. A recursive run otherwise
// renders the member standalone, which adds indexes that are neither tracked nor
// ignored and churns the Source-Hash of every bundle file (#516).
func (g *Generator) GenerateMemberFiles(profile string) (int, error) {
	g.diagnostics()
	outputs, err := g.memberBundleOutputs(profile)
	if err != nil {
		return 0, err
	}
	if err := g.refuseSymlinkedOutputs(outputs); err != nil {
		return 0, err
	}
	if err := g.writeOutputs(outputs); err != nil {
		return 0, err
	}
	written := 0
	for _, output := range outputs {
		if !output.IsDir {
			written++
		}
	}
	g.log().Info("Plugin member generation complete", "files", written)
	return written, nil
}

// DryRunMember lists what GenerateMemberFiles would write.
func (g *Generator) DryRunMember(profile string) ([]string, error) {
	g.diagnostics()
	outputs, err := g.memberBundleOutputs(profile)
	if err != nil {
		return nil, err
	}
	if err := g.refuseSymlinkedOutputs(outputs); err != nil {
		return nil, err
	}
	lines := make([]string, 0, len(outputs)+1)
	lines = append(lines, "plugin member bundle:")
	for _, output := range outputs {
		lines = append(lines, "write-file: "+filepath.ToSlash(g.convertToRelativePath(g.absOutputPath(output.Path))))
	}
	return lines, nil
}

// memberBundleOutputs builds this root's plugin manifest and renders the member
// bundle, without the marketplace indexes a standalone plugin render adds.
func (g *Generator) memberBundleOutputs(profile string) ([]config.OutputFile, error) {
	manifest, err := g.buildPluginManifest(profile)
	if err != nil {
		return nil, err
	}
	outputs, err := plugin.GenerateMember(manifest, g.config.BaseDir)
	if err != nil {
		return nil, oops.With("plugin", manifest.Name).Wrapf(err, "render the plugin member bundle")
	}
	return g.withoutAgentsSourcePluginOutputs(outputs), nil
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

// The kinds of PluginDrift.
const (
	// PluginDriftChanged: the file differs from what generate would write.
	PluginDriftChanged = "changed"
	// PluginDriftMissing: the file is not on disk or cannot be read.
	PluginDriftMissing = "missing"
	// PluginDriftStaleSource: the provenance sidecar no longer matches the bundle.
	PluginDriftStaleSource = "stale-source"
	// PluginDriftObsolete: a file the previous run generated that the sources no longer produce.
	PluginDriftObsolete = "obsolete"
	// PluginDriftExtra: a file inside a generated skill directory that the sidecar does not record.
	PluginDriftExtra = "extra"
	// PluginDriftSymlink: a recorded output that is a symlink, not a regular file.
	PluginDriftSymlink = "symlink"
	// PluginDriftMode: the content is right but the executable bit differs from
	// the one generate would set (and the sidecar recorded).
	PluginDriftMode = "mode"
)

// PluginDrift is one path where a plugin bundle differs from its sources, with
// the kind of difference. Path is project-relative with forward slashes.
//
// For a root that defines several plugins or marketplaces, Plugin and Marketplace
// name the ones the path belongs to (a path in a bundle names both; a marketplace
// index names the marketplace only). They are empty for a single [plugin].
type PluginDrift struct {
	Path        string `json:"path"`
	Kind        string `json:"kind"`
	Plugin      string `json:"plugin,omitempty"`
	Marketplace string `json:"marketplace,omitempty"`
}

type pluginDriftError struct {
	err   error
	drift []PluginDrift
}

func (e pluginDriftError) Error() string      { return e.err.Error() }
func (e pluginDriftError) Unwrap() error      { return e.err }
func (pluginDriftError) Is(target error) bool { return target == ErrPluginDrift }

func asPluginDrift(err error, drift ...PluginDrift) error {
	if err == nil {
		return nil
	}
	return pluginDriftError{err: err, drift: drift}
}

// PluginDriftOf returns the drifted paths a VerifyPlugin failure carries, or nil
// when err is not a drift failure or names no path.
func PluginDriftOf(err error) []PluginDrift {
	var drift pluginDriftError
	if errors.As(err, &drift) {
		return drift.drift
	}
	return nil
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
// sidecars without regenerating or modifying files. Each drifted path of a root
// that defines several plugins names its plugin and marketplace.
func (g *Generator) VerifyPlugin(profile string) error {
	err := g.verifyPlugin(profile)
	var drift pluginDriftError
	if errors.As(err, &drift) && g.config.HasPluginSet() {
		drift.drift = g.tagDrift(drift.drift)
		return drift
	}
	return err
}

// ownerDir is a directory a plugin or a marketplace index is written to.
type ownerDir struct {
	dir         string
	plugin      string
	marketplace string
}

// tagDrift fills Plugin and Marketplace on each entry from the directories the
// last collection wrote: the deepest directory holding the path owns it.
func (g *Generator) tagDrift(items []PluginDrift) []PluginDrift {
	out := make([]PluginDrift, len(items))
	for i, item := range items {
		abs := filepath.Join(g.config.BaseDir, filepath.FromSlash(item.Path))
		best := -1
		for j, owner := range g.ownerDirs {
			rel, err := filepath.Rel(owner.dir, abs)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				continue
			}
			if best < 0 || len(owner.dir) > len(g.ownerDirs[best].dir) {
				best = j
			}
		}
		if best >= 0 {
			item.Plugin, item.Marketplace = g.ownerDirs[best].plugin, g.ownerDirs[best].marketplace
		}
		out[i] = item
	}
	return out
}

func (g *Generator) verifyPlugin(profile string) error {
	g.diagnostics() // the run's warnings share one collector from the start
	expected, obsolete, err := g.resolvePluginOutputs(profile)
	if err != nil {
		return oops.Wrapf(err, "render expected plugin outputs")
	}
	if err := checkPluginGenerated(expected); err != nil {
		return err
	}
	drift := g.verifyPluginOutputs(expected)
	for _, link := range g.symlinkedOutputs(expected) {
		item := PluginDrift{Path: filepath.ToSlash(g.convertToRelativePath(link)), Kind: PluginDriftSymlink}
		if !slices.Contains(drift, item) {
			drift = append(drift, item)
		}
	}
	for _, item := range obsolete {
		drift = append(drift, PluginDrift{Path: item.rel, Kind: PluginDriftObsolete})
	}
	extras, err := g.unrecordedSkillFiles(expected, obsolete)
	if err != nil {
		return err
	}
	for _, path := range extras {
		drift = append(drift, PluginDrift{Path: filepath.ToSlash(g.convertToRelativePath(path)), Kind: PluginDriftExtra})
	}
	if len(obsolete) > 0 {
		return asPluginDrift(obsoleteFilesError(obsolete), drift...)
	}
	if len(drift) > 0 {
		return asPluginDrift(pluginOutputsError(drift), drift...)
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
	return g.verifyPluginProvenance(expected)
}

// verifyPluginProvenance checks the provenance sidecars of the member plugins,
// the domain plugins or the single plugin bundle at the base directory.
func (g *Generator) verifyPluginProvenance(expected []config.OutputFile) error {
	if marketplace := g.config.Marketplace; marketplace != nil && len(marketplace.Members) > 0 {
		for _, member := range marketplace.Members {
			if err := plugin.VerifyProvenance(filepath.Join(g.config.BaseDir, member)); err != nil {
				return asPluginDrift(oops.With("member", member).Wrapf(err, "verify member plugin bundle"),
					g.provenanceDrift(filepath.Join(g.config.BaseDir, member), err))
			}
		}
	}
	if g.hasPlannedPlugins() {
		return g.domainProvenanceDrift(expected)
	}
	if err := plugin.VerifyProvenance(g.config.BaseDir); err != nil {
		return asPluginDrift(err, g.provenanceDrift(g.config.BaseDir, err))
	}
	return nil
}

// provenanceDrift is the drift entry for a VerifyProvenance failure of the bundle
// at bundleDir: the file whose mode differs, or else the stale sidecar.
func (g *Generator) provenanceDrift(bundleDir string, err error) PluginDrift {
	var mode *plugin.ModeDriftError
	if errors.As(err, &mode) {
		path := filepath.Join(bundleDir, filepath.FromSlash(mode.Path))
		return PluginDrift{Path: filepath.ToSlash(g.convertToRelativePath(path)), Kind: PluginDriftMode}
	}
	return g.staleSource(bundleDir)
}

// staleSource is the drift entry for a bundle whose provenance sidecar fails.
func (g *Generator) staleSource(bundleDir string) PluginDrift {
	sidecar := filepath.Join(bundleDir, plugin.ProvenanceFileName)
	return PluginDrift{Path: filepath.ToSlash(g.convertToRelativePath(sidecar)), Kind: PluginDriftStaleSource}
}

// domainProvenanceDrift verifies the domain-plugin bundle roots and names the
// sidecar of the first one that fails.
func (g *Generator) domainProvenanceDrift(expected []config.OutputFile) error {
	for _, output := range expected {
		if filepath.Base(output.Path) != plugin.ProvenanceFileName {
			continue
		}
		dir := filepath.Dir(g.absOutputPath(output.Path))
		if err := plugin.VerifyProvenance(dir); err != nil {
			return asPluginDrift(oops.With("bundle", dir).Wrapf(err, "verify domain plugin bundle"), g.provenanceDrift(dir, err))
		}
	}
	return nil
}

// verifyPluginOutputs compares every expected plugin file with what is on disk
// and returns every path that differs, in output order.
func (g *Generator) verifyPluginOutputs(expected []config.OutputFile) []PluginDrift {
	var drift []PluginDrift
	for _, output := range expected {
		if output.IsDir {
			continue
		}
		rel := filepath.ToSlash(g.convertToRelativePath(g.absOutputPath(output.Path)))
		if info, statErr := os.Lstat(output.Path); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
			drift = append(drift, PluginDrift{Path: rel, Kind: PluginDriftSymlink})
			continue
		}
		actual, readErr := os.ReadFile(output.Path)
		if readErr != nil {
			drift = append(drift, PluginDrift{Path: rel, Kind: PluginDriftMissing})
			continue
		}
		expectedBytes := output.RawContent
		if expectedBytes == nil {
			expectedBytes = []byte(output.Content)
		}
		if !bytes.Equal(actual, expectedBytes) {
			if filepath.Base(output.Path) == plugin.ProvenanceFileName && plugin.SidecarMatchesWithoutModes(expectedBytes, actual) {
				continue // written before modes were recorded; the next generate adds them
			}
			drift = append(drift, PluginDrift{Path: rel, Kind: PluginDriftChanged})
			continue
		}
		if info, statErr := os.Stat(output.Path); statErr == nil && runtime.GOOS != "windows" &&
			(output.Mode^info.Mode())&0o111 != 0 {
			drift = append(drift, PluginDrift{Path: rel, Kind: PluginDriftMode})
		}
	}
	return drift
}

// pluginOutputsError is the verify failure for plugin files that differ from
// what generate would write; the paths are in the PluginDrift list.
func pluginOutputsError(drift []PluginDrift) error {
	paths := make([]string, 0, len(drift))
	for _, item := range drift {
		paths = append(paths, item.Path)
	}
	hint := "Run ai-rulez generate --plugin and commit the regenerated output"
	onlyExtra := true
	for _, item := range drift {
		if item.Kind != PluginDriftExtra {
			onlyExtra = false
		}
		if item.Kind == PluginDriftMissing {
			hint = "Run ai-rulez generate --plugin to restore the missing file and commit the regenerated output"
		}
	}
	if onlyExtra {
		return oops.With("files", strings.Join(paths, ", ")).
			Hint("Run ai-rulez generate --plugin to remove files the bundle does not record").
			Errorf("unrecorded file in a generated plugin skill directory: %s", strings.Join(paths, ", "))
	}
	return oops.With("files", strings.Join(paths, ", ")).Hint(hint).
		Errorf("generated plugin output is stale")
}

// DryRunPlugin returns the plugin generation plan without writing files.
func (g *Generator) DryRunPlugin(profile string) ([]string, error) {
	g.diagnostics() // the run's warnings share one collector from the start
	outputs, err := g.collectPluginOutputs(profile)
	if err != nil {
		return nil, err
	}
	if err := g.refuseSymlinkedOutputs(outputs); err != nil {
		return nil, err
	}
	lines := make([]string, 0, len(outputs)+1)
	lines = append(lines, "plugin bundle:")
	for _, output := range outputs {
		lines = append(lines, "write-file: "+filepath.ToSlash(g.convertToRelativePath(g.absOutputPath(output.Path))))
	}
	stale, err := g.stalePluginDirs()
	if err != nil {
		return nil, err
	}
	for _, dir := range stale {
		lines = append(lines, "delete-stale: "+filepath.ToSlash(g.convertToRelativePath(dir)))
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
	extras, err := g.unrecordedSkillFiles(outputs, obsolete)
	if err != nil {
		return nil, err
	}
	for _, path := range extras {
		lines = append(lines, "delete-stale: "+filepath.ToSlash(g.convertToRelativePath(path)))
	}
	return lines, nil
}

// collectPluginOutputs resolves the content tree and MCP servers, builds the
// plugin manifest, and renders all requested runtime bundles + marketplace. When
// the config is a marketplace root ([marketplace].members or domain plugins), it
// instead renders each plugin's bundle plus the aggregate marketplace index.
func (g *Generator) collectPluginOutputs(profile string) ([]config.OutputFile, error) {
	outputs, err := g.renderPluginOutputs(profile)
	if err != nil {
		return nil, err
	}
	return g.withoutAgentsSourcePluginOutputs(outputs), nil
}

func (g *Generator) renderPluginOutputs(profile string) ([]config.OutputFile, error) {
	if g.config.HasPluginSet() {
		return g.collectPluginSetOutputs(profile)
	}

	if g.config.Plugin == nil {
		return nil, oops.
			Hint("Add a [plugin] block, [[plugins_authoring]] entries, or a [marketplace] with members, plugins or from_domains to your config").
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

	manifest, err := plugin.BuildManifest(&tempCfg, contentTree)
	if err != nil {
		return nil, err
	}
	manifest.Ctx = g.ctx
	return manifest, nil
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

// stalePluginDirs lists the generated plugin directories whose plugin is no
// longer planned for its marketplace (its domain disappeared, or its declaration
// was removed). Only directories carrying ai-rulez's provenance sidecar qualify.
// The plan is made over the unfiltered content tree: a profile that leaves a
// domain out must not delete the plugin another profile generated for it. A run
// limited to some plugins removes nothing.
func (g *Generator) stalePluginDirs() ([]string, error) {
	if len(g.pluginNames) > 0 || !g.hasPlannedPlugins() {
		return nil, nil
	}
	planned, err := plugin.PlanDomainPlugins(g.config, g.config.Content)
	if err != nil {
		return nil, err
	}
	targets, err := g.selectedTargets()
	if err != nil {
		return nil, err
	}
	var stale []string
	for i := range targets {
		t := &targets[i]
		keep := map[string]bool{}
		listed := listedPlugins(planned, t)
		for i := range listed {
			keep[listed[i].Name] = true
		}
		dirs, err := plugin.StalePluginDirs(g.targetRoot(t), keep)
		if err != nil {
			return nil, err
		}
		stale = append(stale, dirs...)
	}
	return stale, nil
}

// hasPlannedPlugins reports whether the root generates plugins from the content
// tree (from_domains, [[marketplace.plugins]] or [[plugins_authoring]]) rather
// than only from members.
func (g *Generator) hasPlannedPlugins() bool {
	return g.config.Marketplace.HasDomainPlugins() || len(g.config.PluginDefinitions()) > 0
}

// listedPlugins is the part of the plan a marketplace target lists: all of it
// for the single [marketplace] block, else the plugins the target names.
func listedPlugins(planned []plugin.PlannedPlugin, t *config.MarketplaceTarget) []plugin.PlannedPlugin {
	if t.Plugins == nil {
		return planned
	}
	var out []plugin.PlannedPlugin
	for i := range planned {
		if slices.Contains(t.Plugins, planned[i].Name) {
			out = append(out, planned[i])
		}
	}
	return out
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

// targetRoot is the directory a marketplace index and its plugins/<name>/ bundles
// are written to: the target's output_dir, else the project root.
func (g *Generator) targetRoot(t *config.MarketplaceTarget) string {
	if t.OutputDir == "" {
		return g.config.BaseDir
	}
	return filepath.Join(g.config.BaseDir, t.OutputDir)
}

// selectedTargets returns the marketplace indexes this run writes: all of them, or
// the ones WithMarketplaceNames limits it to. An unknown name is an error naming the
// marketplaces that exist.
func (g *Generator) selectedTargets() ([]config.MarketplaceTarget, error) {
	targets := g.config.MarketplaceTargets()
	if len(g.marketplaceNames) == 0 {
		return targets, nil
	}
	var picked []config.MarketplaceTarget
	have := make([]string, 0, len(targets))
	for i := range targets {
		have = append(have, targets[i].Name)
		if slices.Contains(g.marketplaceNames, targets[i].Name) {
			picked = append(picked, targets[i])
		}
	}
	for _, want := range g.marketplaceNames {
		if !slices.Contains(have, want) {
			return nil, oops.With("marketplace", want).
				Errorf("no marketplace named %q (marketplaces: %s)", want, strings.Join(have, ", "))
		}
	}
	return picked, nil
}

// WithPluginNames limits generate --plugin and verify --plugin to the named plugins
// (members and planned plugins). The marketplace indexes are left alone, since a
// partial index would not match the full one. It returns g.
func (g *Generator) WithPluginNames(names []string) *Generator {
	g.pluginNames = append([]string(nil), names...)
	return g
}

// WithMarketplaceNames limits generate --plugin and verify --plugin to the named
// marketplaces (their indexes and plugin bundles). It returns g.
func (g *Generator) WithMarketplaceNames(names []string) *Generator {
	g.marketplaceNames = append([]string(nil), names...)
	return g
}

// pluginSelected reports whether the plugin takes part in this run.
func (g *Generator) pluginSelected(name string) bool {
	return len(g.pluginNames) == 0 || slices.Contains(g.pluginNames, name)
}

// collectPluginSetOutputs renders every plugin of a root that defines several:
// the monorepo members under their source directories, the planned plugins (from
// from_domains, [[marketplace.plugins]] and [[plugins_authoring]]) under
// <output_dir>/plugins/<name>, and one index per marketplace.
func (g *Generator) collectPluginSetOutputs(profile string) ([]config.OutputFile, error) {
	targets, err := g.selectedTargets()
	if err != nil {
		return nil, err
	}
	var planned []plugin.PlannedPlugin
	if g.hasPlannedPlugins() {
		if planned, err = g.planDomainPlugins(profile); err != nil {
			return nil, err
		}
	}
	cfg, err := g.pluginSetConfig(planned)
	if err != nil {
		return nil, err
	}
	var outputs []config.OutputFile
	matched := map[string]bool{}
	g.ownerDirs = nil
	for i := range targets {
		targetOutputs, err := g.collectTargetOutputs(&targets[i], planned, cfg, matched)
		if err != nil {
			return nil, err
		}
		outputs = append(outputs, targetOutputs...)
	}
	for _, want := range g.pluginNames {
		if !matched[want] {
			return nil, oops.With("plugin", want).
				Hint("Plugins: "+strings.Join(g.pluginNamesOf(planned), ", ")).
				Errorf("no plugin named %q (plugins: %s)", want, strings.Join(g.pluginNamesOf(planned), ", "))
		}
	}
	if err := checkDuplicateOutputs(outputs); err != nil {
		return nil, err
	}
	return outputs, nil
}

// pluginNamesOf lists the planned plugin names and the monorepo members' paths.
func (g *Generator) pluginNamesOf(planned []plugin.PlannedPlugin) []string {
	var names []string
	if m := g.config.Marketplace; m != nil {
		names = append(names, m.Members...)
	}
	for i := range planned {
		names = append(names, planned[i].Name)
	}
	sort.Strings(names)
	return names
}

// pluginSetConfig is the configuration the planned plugins' manifests are built
// from: with the MCP servers their definitions name resolved. A definition that
// names a server the root does not define is an error.
func (g *Generator) pluginSetConfig(planned []plugin.PlannedPlugin) (*config.Config, error) {
	need := false
	for i := range planned {
		for _, name := range planned[i].MCPServers {
			if _, ok := g.config.MCPServers[name]; !ok {
				return nil, oops.With("plugin", planned[i].Name).With("server", name).
					Errorf("plugin %q bundles MCP server %q, which is not defined (servers: %s)",
						planned[i].Name, name, strings.Join(sortedServerNames(g.config), ", "))
			}
			need = true
		}
	}
	if !need {
		return g.config, nil
	}
	if err := g.resolveMCPEnvForPlugin(); err != nil {
		return nil, err
	}
	return g.config, nil
}

func sortedServerNames(cfg *config.Config) []string {
	names := make([]string, 0, len(cfg.MCPServers))
	for name := range cfg.MCPServers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// collectTargetOutputs renders one marketplace: the members of the single
// [marketplace] block, the plugins it lists and its index files.
func (g *Generator) collectTargetOutputs(t *config.MarketplaceTarget, planned []plugin.PlannedPlugin, cfg *config.Config, matched map[string]bool) ([]config.OutputFile, error) {
	var outputs []config.OutputFile
	var entries []plugin.MemberEntry
	g.ownerDirs = append(g.ownerDirs, ownerDir{dir: g.targetRoot(t), marketplace: t.Name})
	if t.Classic != nil {
		for _, member := range t.Classic.Members {
			memberOutputs, entry, err := g.collectMemberOutputs(member)
			if err != nil {
				return nil, err
			}
			if !g.pluginSelected(entry.Name) {
				continue
			}
			matched[entry.Name] = true
			g.ownerDirs = append(g.ownerDirs, ownerDir{
				dir: filepath.Join(g.config.BaseDir, member), plugin: entry.Name, marketplace: t.Name})
			outputs = append(outputs, memberOutputs...)
			entries = append(entries, entry)
		}
	}
	codexEntries := slices.Clone(entries) // members are always Codex-capable
	root := g.targetRoot(t)
	codex := len(entries) > 0 || (t.Classic != nil && len(t.Classic.Members) > 0)

	listed, err := g.listedOrError(t, planned)
	if err != nil {
		return nil, err
	}
	for i := range listed {
		p := &listed[i]
		if !g.pluginSelected(p.Name) {
			continue
		}
		matched[p.Name] = true
		g.ownerDirs = append(g.ownerDirs, ownerDir{dir: p.Dir(root), plugin: p.Name, marketplace: t.Name})
		domainManifest := plugin.BuildDomainManifest(cfg, p)
		domainManifest.Ctx = g.ctx
		bundle, err := plugin.GenerateMember(domainManifest, p.Dir(root))
		if err != nil {
			return nil, oops.With("plugin", p.Name).Wrapf(err, "generate domain plugin bundle")
		}
		outputs = append(outputs, bundle...)
		entry := plugin.MemberEntryFor(p)
		entries = append(entries, entry)
		if entry.Codex {
			codex = true
			codexEntries = append(codexEntries, entry)
		}
	}
	if err := checkDuplicatePlugins(t.Name, entries); err != nil {
		return nil, err
	}
	if len(g.pluginNames) > 0 {
		return outputs, nil // a partial index would not match the full one
	}
	return g.withMarketplaceIndex(t, outputs, entries, codexEntries, codex)
}

// listedOrError returns the planned plugins a target lists. A marketplace that
// names a plugin the plan does not produce (no definition, or one that bundles
// nothing) is an error.
func (g *Generator) listedOrError(t *config.MarketplaceTarget, planned []plugin.PlannedPlugin) ([]plugin.PlannedPlugin, error) {
	listed := listedPlugins(planned, t)
	if t.Plugins == nil {
		return listed, nil
	}
	have := make([]string, 0, len(planned))
	for i := range planned {
		have = append(have, planned[i].Name)
	}
	for _, name := range t.Plugins {
		if !slices.Contains(have, name) {
			return nil, oops.With("marketplace", t.Name).With("plugin", name).
				Errorf("marketplace %q lists plugin %q, which no plugin definition provides or which bundles no content (plugins: %s)",
					t.Name, name, strings.Join(have, ", "))
		}
	}
	return listed, nil
}

// checkDuplicatePlugins rejects two plugins of one marketplace with one name,
// naming where each comes from.
func checkDuplicatePlugins(marketplace string, entries []plugin.MemberEntry) error {
	seen := make(map[string]string, len(entries))
	for i := range entries {
		e := &entries[i]
		if first, dup := seen[e.Name]; dup {
			return oops.With("marketplace", marketplace).With("plugin", e.Name).
				Hint("Rename one of them; plugin names are unique within a marketplace").
				Errorf("marketplace %q has two plugins named %q: %s and %s", marketplace, e.Name, first, e.Source)
		}
		seen[e.Name] = e.Source
	}
	return nil
}

// checkDuplicateOutputs rejects two outputs written to one path: two plugins (or
// two marketplaces) that would overwrite each other's files.
func checkDuplicateOutputs(outputs []config.OutputFile) error {
	seen := make(map[string]bool, len(outputs))
	for i := range outputs {
		o := &outputs[i]
		if o.IsDir {
			continue
		}
		path := filepath.Clean(o.Path)
		if seen[path] {
			return oops.With("path", path).
				Hint("Give each plugin a distinct name and each marketplace a distinct output_dir").
				Errorf("two plugin outputs are written to %s", path)
		}
		seen[path] = true
	}
	return nil
}

// withMarketplaceIndex adds a marketplace's index files and provenance sidecar to
// the bundle outputs.
func (g *Generator) withMarketplaceIndex(t *config.MarketplaceTarget, outputs []config.OutputFile, entries, codexEntries []plugin.MemberEntry, codex bool) ([]config.OutputFile, error) {
	root := g.targetRoot(t)
	market := plugin.MarketInfo{Name: t.Name, Description: t.Description, Owner: t.Owner}
	if t.Classic != nil {
		market = plugin.ResolveMarketInfo(t.Classic)
	}
	if market.Owner == nil && g.config.Plugin != nil && (t.Classic == nil || g.hasPlannedPlugins()) {
		market.Owner = g.config.Plugin.Author // Claude Code requires an owner
	}
	if market.Owner == nil {
		for _, def := range g.config.PluginDefinitions() {
			if def.Author != nil && slices.ContainsFunc(entries, func(e plugin.MemberEntry) bool { return e.Name == def.Name }) {
				market.Owner = def.Author
				break
			}
		}
	}
	marketplaceOutput, err := plugin.RenderMonorepoMarketplace(market, entries, root)
	if err != nil {
		return nil, oops.Wrapf(err, "render monorepo marketplace")
	}
	rootFiles := []config.OutputFile{marketplaceOutput}
	extraIndexes, err := g.renderExtraMarketplaces(t.CursorIndex, market, entries, codexEntries, root, codex)
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
// plugin-set root: the Codex index when some plugin ships a Codex bundle, and
// the Cursor index when cursor_index is set.
func (g *Generator) renderExtraMarketplaces(cursorIndex bool, market plugin.MarketInfo, entries, codexEntries []plugin.MemberEntry, root string, codex bool) ([]config.OutputFile, error) {
	var files []config.OutputFile
	if codex {
		out, err := plugin.RenderCodexMonorepoMarketplace(market, codexEntries, root)
		if err != nil {
			return nil, oops.Wrapf(err, "render Codex monorepo marketplace")
		}
		files = append(files, out)
	}
	if cursorIndex {
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
	manifest, memberCfg, err := g.memberManifest(member)
	if err != nil {
		return nil, plugin.MemberEntry{}, err
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

// memberManifest loads one monorepo member's configuration and builds the
// manifest of its plugin bundle.
func (g *Generator) memberManifest(member string) (*plugin.Manifest, *config.Config, error) {
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
		return nil, nil, oops.With("member", member).Wrapf(err, "load monorepo member config")
	}
	if err := g.nestedPolicy(memberCfg, true); err != nil {
		return nil, nil, oops.With("member", member).Wrapf(err, "monorepo member config")
	}
	memberCfg.Diag = g.config.Diag
	if memberCfg.Plugin == nil {
		return nil, nil, oops.
			With("member", member).
			Hint("Each monorepo member must define its own [plugin] block").
			Errorf("monorepo member %q has no [plugin] block", member)
	}
	if len(g.memberRuntimes) > 0 {
		if err := limitMemberRuntimes(memberCfg, member, g.memberRuntimes); err != nil {
			return nil, nil, err
		}
	}

	manifest, err := NewGenerator(memberCfg).buildPluginManifest("")
	if err != nil {
		return nil, nil, oops.With("member", member).Wrapf(err, "build member manifest")
	}
	return manifest, memberCfg, nil
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

// PluginStatus is the verification result of one plugin of a root that defines
// several, as `verify --plugin --format json` reports it.
type PluginStatus struct {
	Name        string `json:"name"`
	Marketplace string `json:"marketplace,omitempty"`
	Status      string `json:"status"`
}

// MarketplaceStatus is the verification result of one marketplace: drift when its
// index or any plugin it lists drifted.
type MarketplaceStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

// Verification outcomes of a plugin or marketplace.
const (
	PluginStatusOK    = "ok"
	PluginStatusDrift = "drift"
)

// PluginStatuses reports, after VerifyPlugin, every plugin and marketplace of a
// root that defines several, each ok or drift given the drift VerifyPlugin found.
// A root with a single [plugin] has none.
func (g *Generator) PluginStatuses(drift []PluginDrift) (plugins []PluginStatus, marketplaces []MarketplaceStatus) {
	if !g.config.HasPluginSet() {
		return nil, nil
	}
	status := func(hit bool) string {
		if hit {
			return PluginStatusDrift
		}
		return PluginStatusOK
	}
	for _, owner := range g.ownerDirs {
		if owner.plugin == "" {
			continue
		}
		hit := slices.ContainsFunc(drift, func(d PluginDrift) bool {
			return d.Plugin == owner.plugin && d.Marketplace == owner.marketplace
		})
		plugins = append(plugins, PluginStatus{Name: owner.plugin, Marketplace: owner.marketplace, Status: status(hit)})
	}
	seen := map[string]bool{}
	for _, owner := range g.ownerDirs {
		if seen[owner.marketplace] {
			continue
		}
		seen[owner.marketplace] = true
		hit := slices.ContainsFunc(drift, func(d PluginDrift) bool { return d.Marketplace == owner.marketplace })
		marketplaces = append(marketplaces, MarketplaceStatus{Name: owner.marketplace, Status: status(hit)})
	}
	return plugins, marketplaces
}
