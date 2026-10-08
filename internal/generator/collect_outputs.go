package generator

import (
	"bytes"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/diag"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/presets" // Register remaining legacy preset generators
	"github.com/Goldziher/ai-rulez/v5/internal/generator/rulefiles"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/settings"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

func (g *Generator) collectOutputs(profile string) ([]config.OutputFile, string, error) {
	render, err := g.renderPresets(profile)
	if err != nil {
		return nil, "", err
	}
	activeProfile, contentTree, run := render.profile, render.content, render.run

	// Flatten outputs for writing, detecting conflicts and deduplicating
	flatOutputs, err := flattenPresetOutputs(g.config.Diag, g.log(), g.config.ReadExisting, render.byPreset)
	if err != nil {
		return nil, "", err
	}
	if index, ok := g.skillsIndexOutput(contentTree, render.byPreset); ok {
		flatOutputs = append(flatOutputs, index)
	}
	manifest, ok, err := g.rolesManifestOutput()
	if err != nil {
		return nil, "", err
	}
	if ok {
		flatOutputs = append(flatOutputs, manifest)
	}

	scopedOutputs, err := g.generateScopedOutputs(activeProfile, contentTree, run)
	if err != nil {
		return nil, "", err
	}
	flatOutputs = append(flatOutputs, scopedOutputs...)
	g.disambiguateRuleCollisions(flatOutputs)
	g.markPreexistingDocuments(flatOutputs)
	g.carryClaimAnnotations(flatOutputs)
	g.dropUserHeldClaims(flatOutputs)
	g.reclaimStaleMembers(flatOutputs)
	g.warnInstructionSizes(flatOutputs)

	return flatOutputs, activeProfile, nil
}

// presetRender is what rendering every configured preset for one profile yields,
// before the outputs of the presets are merged into one list.
type presetRender struct {
	byPreset map[string][]config.OutputFile
	profile  string
	content  *config.ContentTree
	run      *config.RunState
}

// renderPresets renders every configured preset, the auto-generated MCP output
// and the machine-local variants, keyed by preset name.
func (g *Generator) renderPresets(profile string) (*presetRender, error) {
	if err := g.resolveMCPEnv(); err != nil {
		return nil, err
	}

	activeProfile := g.resolveProfile(profile)

	contentTree, err := g.getContentForProfile(activeProfile)
	if err != nil {
		return nil, err
	}

	contentTree, err = g.withCatalogSkill(contentTree)
	if err != nil {
		return nil, err
	}

	g.log().Debug("Content scanned",
		"rules", len(contentTree.Rules),
		"context", len(contentTree.Context),
		"skills", len(contentTree.Skills),
		"agents", len(contentTree.Agents),
		"domains", len(contentTree.Domains))

	presets.WarnDuplicateContent(g.log(), contentTree)
	g.warnUnbundledPluginOnly(contentTree)
	g.warnIgnoredPlugins()
	g.warnLegacyFiles()
	for _, diagnostic := range settings.UnsupportedDiagnostics(g.config) {
		g.config.Diag.Warn(diagnostic)
	}

	// Collect MCP servers based on the resolved content tree and active profile
	mcpServers := g.collectMCPServersForContent(contentTree, activeProfile)

	// Resolve the run's header timestamp once, before any renderer reads it, so
	// every file this run writes carries the same value. Each preset used to call
	// time.Now() for itself, which made CLAUDE.md and AGENTS.md — byte-identical
	// otherwise — disagree whenever the two renders straddled a second boundary.
	// A caller that set GeneratedAt explicitly keeps its value.
	if g.config.GeneratedAt.IsZero() {
		g.config.GeneratedAt = config.ResolveGenerationTimeIn(g.host())
	}

	// Create a temporary config with the filtered content and MCP servers
	tempCfg := *g.config
	tempCfg.Content = contentTree
	tempCfg.MCPServers = mcpServers
	// Rule files of the root and of every scope land in the same rules folders;
	// the run state carries what each preset has claimed so far.
	run := config.NewRunState()
	run.SetPreviouslyGenerated(g.previousManifestFiles())
	run.SetPreviousMerged(g.previousMergedClaims())
	tempCfg.Run = run

	// Compute a single source hash covering all profile-relevant inputs.
	// Embedded in every output's header so subsequent runs can detect "no source
	// change" cheaply, and combined with Content-Hash for skip decisions.
	// We set it on both tempCfg (used by preset rendering) and g.config (used
	// by writeOutput's skip decision and hash injection).
	sourceHash := computeSourceHash(&tempCfg, contentTree)
	tempCfg.SourceHash = sourceHash
	g.config.SourceHash = sourceHash

	// Generate outputs for all presets using the existing infrastructure
	allOutputs, err := config.GeneratePresets(&tempCfg)
	if err != nil {
		return nil, oops.Wrapf(err, "generate presets")
	}

	applySharedOutputs(allOutputs, &tempCfg, contentTree)

	// Auto-generate MCP output if servers exist. The MCP preset is now
	// DSL-driven (internal/generator/providers/builtin/mcp.toml); fetch it
	// from the registry rather than instantiating a hand-written generator.
	if len(mcpServers) > 0 || g.config.HasSelfServer() {
		mcpGen, err := g.config.Registry.Generator("mcp")
		if err != nil {
			g.log().Warn("Failed to resolve MCP preset generator", "error", err)
		} else {
			mcpOutputs, err := mcpGen.Generate(contentTree, g.config.BaseDir, &tempCfg)
			if err != nil {
				g.log().Warn("Failed to generate MCP output", "error", err)
			} else if len(mcpOutputs) > 0 {
				allOutputs["mcp"] = mcpOutputs
				g.log().Debug("Auto-generated MCP output", "count", len(mcpOutputs))
			}
		}
	}

	// Append machine-local root variants (CLAUDE.local.md, AGENTS.local.md, ...)
	// keyed by preset so they flow through the same flatten/manifest/stale path:
	// duplicate local paths (codex + opencode both emit AGENTS.local.md) collapse,
	// and removed local content deletes the file via stale-manifest cleanup.
	if err := g.appendLocalOutputs(allOutputs, &tempCfg, activeProfile); err != nil {
		return nil, err
	}

	return &presetRender{byPreset: allOutputs, profile: activeProfile, content: contentTree, run: run}, nil
}

// appendLocalOutputs renders the machine-local outputs of every configured
// built-in preset, appending them to allOutputs under that preset's key. Local
// content is the .ai-rulez/local/ tree with the active profile applied, so local
// domains are selected like shared ones.
//
// Presets that implement config.LocalRuleProvider write local rules as personal
// rule files ("<rulesdir>/<id>.local<ext>") when their routing sends them to rule
// files; every other local rule, and local context, goes to the preset's ".local"
// root (config.LocalRootProvider, or a config.LocalRootRenderer for presets whose
// root is not plain markdown). Local content a preset has no place for is
// reported once with a warning. Local skills, agents and commands are written as
// per-item files only (see appendLocalItemOutputs). It is a no-op when there is
// no machine-local content. cfg is the profile-resolved temp config (carries
// Name / header style / compact used by the renderer). Custom provider presets get
// no local outputs.
func (g *Generator) appendLocalOutputs(allOutputs map[string][]config.OutputFile, cfg *config.Config, profile string) error {
	if g.config.LocalContent == nil || g.config.LocalContent.IsEmpty() {
		return nil
	}
	local, err := selectProfileContent(g.config, g.config.LocalContent, profile)
	if err != nil {
		return oops.Wrapf(err, "select local content for the profile")
	}
	allRules := presets.AllInlineRules(local)
	allContext := presets.AllInlineContext(local)
	done := make(map[string]bool)
	for _, preset := range g.config.Presets {
		name := preset.GetName()
		if !preset.IsBuiltIn() || done[name] {
			continue
		}
		done[name] = true
		generator, err := g.config.Registry.Generator(preset.BuiltIn)
		if err != nil {
			g.log().Debug("Skipping local outputs for unknown preset", "preset", name, "error", err)
			continue
		}
		rules := allRules
		if provider, ok := generator.(config.LocalRuleProvider); ok {
			var files []config.OutputFile
			files, rules, err = provider.LocalRuleOutputs(allRules, g.config.BaseDir, cfg)
			if err != nil {
				return oops.With("preset", name).Wrapf(err, "render local rule files")
			}
			allOutputs[name] = append(allOutputs[name], files...)
		}
		if len(rules) == 0 && len(allContext) == 0 {
			continue
		}
		root, ok, err := g.localRootOutput(generator, local, rules, cfg)
		if err != nil {
			return oops.With("preset", name).Wrapf(err, "render local root file")
		}
		if !ok {
			if !readsAgentsOverride(g.config, name) {
				g.warnDroppedLocal(name, rules, allContext)
			}
			continue
		}
		allOutputs[name] = append(allOutputs[name], root)
	}
	g.appendAgentsOverride(allOutputs, local, allRules, cfg)
	return g.appendLocalItemOutputs(allOutputs, cfg, local)
}

// readsAgentsOverride reports whether a built-in preset loads AGENTS.override.md
// in place of AGENTS.md: Codex always, Hermes when agents_md makes it read the
// AGENTS chain (without it .hermes.md wins over the chain and no local file is read).
func readsAgentsOverride(cfg *config.Config, preset string) bool {
	return preset == string(config.PresetCodex) || (preset == string(config.PresetHermes) && cfg.AgentsMD)
}

// appendAgentsOverride writes the machine-local AGENTS.override.md for the
// configured presets that load it (see readsAgentsOverride). The file replaces
// AGENTS.md for them, so it repeats the root AGENTS.md as written this run and
// appends the local rules and context. Only the project root has one: scope runs
// never reach this function.
func (g *Generator) appendAgentsOverride(allOutputs map[string][]config.OutputFile, local *config.ContentTree,
	rules []config.ContentFile, cfg *config.Config,
) {
	var readers []string
	for _, preset := range g.config.Presets {
		if preset.IsBuiltIn() && readsAgentsOverride(g.config, preset.BuiltIn) && !slices.Contains(readers, preset.BuiltIn) {
			readers = append(readers, preset.BuiltIn)
		}
	}
	if len(readers) == 0 {
		return
	}
	owners := append(rulefiles.RootOwners(string(config.SharedAgentsMD)), readers...)
	if _, has := presets.RenderAgentsOverride("", local, rules, cfg, owners); !has {
		return
	}
	names := strings.Join(readers, ", ")
	overridePath := filepath.Join(g.config.BaseDir, presets.AgentsOverrideFile)
	if g.isHandWritten(overridePath) {
		g.config.Diag.Warn(presets.AgentsOverrideFile+" exists and was not written by ai-rulez, so it is left alone and "+
			"the presets that read it ("+names+") do not load your machine-local content",
			"hint", "move or delete the file to let ai-rulez write it", "path", presets.AgentsOverrideFile)
		return
	}
	agentsMD, ok := rootAgentsMD(g.config.Diag, allOutputs, g.config.BaseDir)
	if !ok {
		g.config.Diag.Warn("machine-local content exists for "+names+", but this run produces no AGENTS.md, so "+
			presets.AgentsOverrideFile+" is not written and the presets that read it do not load it",
			"hint", "enable a preset that writes AGENTS.md, or set agents_md = true")
		return
	}
	// The override carries the AGENTS.md body without its generated banner: the
	// file has a banner of its own, and a second one nested inside it would carry
	// the per-run header stamp into the body and rewrite the file every run.
	content, ok := presets.RenderAgentsOverride(stripHeader(agentsMD, string(config.SharedAgentsMD)), local, rules, cfg, owners)
	if !ok {
		return
	}
	allOutputs[readers[0]] = append(allOutputs[readers[0]], config.OutputFile{
		Path:      overridePath,
		Content:   content,
		LocalOnly: true,
	})
}

// isHandWritten reports whether the file at path exists and is not one ai-rulez
// wrote: it is in neither manifest and carries no generated banner or hashes.
func (g *Generator) isHandWritten(path string) bool {
	if !g.pathIsFile(path) {
		return false
	}
	rel := filepath.ToSlash(g.convertToRelativePath(path))
	if slices.Contains(g.previousManifestFiles(), rel) {
		return false
	}
	return !g.looksGenerated(path)
}

// rootAgentsMD returns the content of the project's AGENTS.md as written this
// run: the winner of flattenPresetOutputs, which is the shared one with agents_md
// and otherwise the last preset in name order to render one.
func rootAgentsMD(d *diag.Collector, allOutputs map[string][]config.OutputFile, baseDir string) (string, bool) {
	path := filepath.Join(baseDir, string(config.SharedAgentsMD))
	// A conflict is reported by collectOutputs; the files found so far still tell
	// what AGENTS.md says.
	flat, err := flattenPresetOutputs(d, nil, nil, allOutputs)
	if err != nil && len(flat) == 0 {
		return "", false
	}
	for _, o := range flat {
		if !o.IsDir && o.RawContent == nil && samePath(o.Path, path) {
			return o.Content, true
		}
	}
	return "", false
}

// localRootOutput renders a preset's machine-local root file; ok is false when
// the preset has none.
func (g *Generator) localRootOutput(generator config.PresetGenerator, local *config.ContentTree,
	rules []config.ContentFile, cfg *config.Config,
) (out config.OutputFile, ok bool, err error) {
	if renderer, isRenderer := generator.(config.LocalRootRenderer); isRenderer {
		out, err = renderer.RenderLocalRoot(local, rules, g.config.BaseDir, cfg)
		return out, err == nil, err
	}
	rootProvider, isProvider := generator.(config.LocalRootProvider)
	if !isProvider || rootProvider.LocalRootFile() == "" {
		return out, false, nil
	}
	localFile := rootProvider.LocalRootFile()
	content := presets.RenderLocalRootRules(local, rules, cfg, localFile)
	if standIn, ok := generator.(config.LocalRootStandIn); ok {
		content = presets.RenderLocalRootRulesFor(local, rules, cfg, localFile, standIn.LocalRootStandsIn())
	}
	return config.OutputFile{
		Path:      filepath.Join(g.config.BaseDir, localFile),
		Content:   content,
		LocalOnly: true,
	}, true, nil
}

// droppedLocalItems labels local rules and context for the dropped-content warning.
func droppedLocalItems(rules, contexts []config.ContentFile) []string {
	var items []string
	for i := range rules {
		r := rules[i]
		items = append(items, "rule "+r.Name)
	}
	for i := range contexts {
		c := contexts[i]
		items = append(items, "context "+c.Name)
	}
	return items
}

// warnDroppedLocal warns about local rules and context a preset has no output
// for: it writes no local root file and does not route them to rule files.
func (g *Generator) warnDroppedLocal(preset string, rules, contexts []config.ContentFile) {
	items := droppedLocalItems(rules, contexts)
	if len(items) == 0 {
		return
	}
	g.log().Warn("Machine-local content has no output for this preset and was not written",
		"preset", preset, "items", strings.Join(items, ", "))
}

// resolveProfile determines which profile to use. A composed value
// ("base,backend") is canonicalized but kept composed: it is resolved to the union
// of its elements' domains later, and reporting it verbatim keeps the log and the
// dry-run header honest about what was asked for.
func (g *Generator) resolveProfile(profile string) string {
	// 1. Use provided profile if specified
	if profile != "" {
		return config.CanonicalProfile(profile)
	}

	// 2. Use default profile from config if specified
	if g.config.Default != "" {
		return config.CanonicalProfile(g.config.Default)
	}

	// 3. Use "default" as fallback
	return defaultProfileName
}

// getContentForProfile returns the shared content tree for a specific profile.
func (g *Generator) getContentForProfile(profile string) (*config.ContentTree, error) {
	if g.role != nil {
		return g.config.FilterTreeForRole(g.config.Content, g.role)
	}
	return selectProfileContent(g.config, g.config.Content, profile)
}

// selectProfileContent applies profile selection to a content tree: the built-in
// "default" profile rules, named and composed profiles, and the unknown-profile
// error. The shared tree and the machine-local one (.ai-rulez/local/) go through
// it, so a local domain is selected exactly like a shared one.
func selectProfileContent(cfg *config.Config, content *config.ContentTree, profile string) (*config.ContentTree, error) {
	// Guard against nil content to avoid panics when Generator is created
	// with a Config that hasn't been fully loaded.
	if content == nil {
		return nil, config.ErrNoContent
	}

	// Special case: "default" profile
	if profile == defaultProfileName {
		// If the user explicitly defined profiles["default"], honor it like any
		// other named profile rather than applying the built-in fallback logic.
		if cfg.HasProfile(defaultProfileName) {
			return cfg.SelectContentForProfile(content, defaultProfileName)
		}

		// When no profiles are defined, "default" should include all content
		// (root + all domains). This is important for consumers that rely on
		// includes and don't define their own profiles.
		if len(cfg.Profiles) == 0 {
			// Shallow-copy domains to avoid exposing internal map for mutation.
			domainsCopy := make(map[string]*config.Domain, len(content.Domains))
			for name, domain := range content.Domains {
				domainsCopy[name] = domain
			}

			return &config.ContentTree{
				Rules:    content.Rules,
				Context:  content.Context,
				Skills:   content.Skills,
				Agents:   content.Agents,
				Commands: content.Commands,
				Checks:   content.Checks,
				Domains:  domainsCopy,
			}, nil
		}

		// When profiles are defined in the config, keep the previous
		// behavior where the built-in "default" profile only sees
		// root content, globally-active built-in domains, and FromInclude
		// domains. A builtin scoped to a named profile via `builtin:<name>`
		// is not global, so it is excluded unless `default` names it.
		defaultDomains := make(map[string]*config.Domain)
		for name, domain := range content.Domains {
			if domain.FromInclude || (domain.Builtin && !domain.BuiltinScoped) {
				defaultDomains[name] = domain
			}
		}
		return &config.ContentTree{
			Rules:    content.Rules,
			Context:  content.Context,
			Skills:   content.Skills,
			Agents:   content.Agents,
			Commands: content.Commands,
			Checks:   content.Checks,
			Domains:  defaultDomains,
		}, nil
	}

	// Check if profile exists
	if !cfg.HasProfile(profile) {
		availableProfiles := make([]string, 0, len(cfg.Profiles))
		for name := range cfg.Profiles {
			availableProfiles = append(availableProfiles, name)
		}
		sort.Strings(availableProfiles)

		// Name the elements that are actually unknown. For a single name that is
		// the value itself; for a composed value it is the difference between
		// "one of these three is wrong" and knowing which.
		unknown := cfg.UnknownProfileNames(profile)

		return nil, oops.
			With("profile", profile).
			With("unknown_profiles", unknown).
			With("available_profiles", availableProfiles).
			Hint(fmt.Sprintf(
				"Available profiles: %v\nUse 'default' for the built-in profile (all content when no profiles are defined; root content plus builtin and FromInclude domains when profiles are defined).",
				availableProfiles,
			)).
			Errorf("profile not found: %s", strings.Join(unknown, ", "))
	}

	// Get content for the profile (includes root + specified domains)
	return cfg.SelectContentForProfile(content, profile)
}

// collectMCPServersForContent collects the enabled root MCP servers active for
// a profile. A server restricted with `profiles` is included only when the
// active profile names it; an empty profile (no profiles configured) includes
// every server.
func (g *Generator) collectMCPServersForContent(content *config.ContentTree, profile string) map[string]*config.MCPServer {
	collected := make(map[string]*config.MCPServer)

	// Include root servers (if enabled and active for the profile)
	for name, server := range g.config.MCPServers {
		if server.IsEnabled() && config.ProfileMatches(profile, server.Profiles) {
			collected[name] = server
		}
	}

	return collected
}

// flattenPresetOutputs merges outputs from all presets, deduplicating directories.
// Several presets legitimately write one path (AGENTS.md, .mcp.json,
// .agents/skills/*), but only with identical content: the file is kept once, so
// two presets rendering different bytes to a path would let the one sorted last
// silently replace the other's. That is reported as an error naming the presets;
// the outputs found so far (first writer of each path) are returned with it.
//
// Two divergences are resolved instead. Presets that write one merged document
// (or one owned hooks file) with different keys are combined: their owned keys are
// unioned and the document rendered once (see unionOutputs). And a root file that omits the rules its tool
// reads from a rules folder (OutputFile.OmitsRules) yields to the same file with
// every rule inlined, because dropping the inlined rules would silently take
// them from the tools that have no folder. The choice does not depend on the
// order of the preset names, and a warning names the presets.
func flattenPresetOutputs(d *diag.Collector, log logger.Logger, read jsonmerge.Reader, allOutputs map[string][]config.OutputFile) ([]config.OutputFile, error) {
	f := &presetFlattener{
		read:        read,
		seenPaths:   make(map[string]presetClaim),
		seenDirs:    make(map[string]bool),
		conflicting: make(map[string][]string),
		omitting:    make(map[string][]string),
	}
	presetNames := make([]string, 0, len(allOutputs))
	for presetName := range allOutputs {
		presetNames = append(presetNames, presetName)
	}
	sort.Strings(presetNames)
	for _, presetName := range presetNames {
		outputs := allOutputs[presetName]
		logger.Or(log).Debug("Generated outputs for preset", "preset", presetName, "count", len(outputs))
		for _, output := range outputs {
			f.add(presetName, output)
		}
	}
	f.warnOmitting(d)
	return f.flat, f.conflictError()
}

// presetClaim records which preset first wrote a path and where its output sits
// in presetFlattener.flat.
type presetClaim struct {
	preset string
	index  int // position in flat
}

// presetFlattener accumulates the outputs of every preset into one list with a
// single entry per path, remembering which presets diverged or yielded.
type presetFlattener struct {
	read          jsonmerge.Reader
	flat          []config.OutputFile
	seenPaths     map[string]presetClaim
	seenDirs      map[string]bool
	conflicting   map[string][]string // path -> presets that differ from its first writer
	conflictPaths []string
	omitting      map[string][]string // path -> presets whose rule-less version yielded
	omittingPaths []string
}

// add folds one preset's output into the flattened list.
func (f *presetFlattener) add(presetName string, output config.OutputFile) {
	if output.IsDir {
		if !f.seenDirs[output.Path] {
			f.seenDirs[output.Path] = true
			f.flat = append(f.flat, output)
		}
		return
	}
	prev, ok := f.seenPaths[output.Path]
	if !ok {
		f.seenPaths[output.Path] = presetClaim{preset: presetName, index: len(f.flat)}
		f.flat = append(f.flat, output)
		return
	}
	kept := f.flat[prev.index]
	var united config.OutputFile
	var unionable bool
	if !sameOutputContent(kept, output) {
		united, unionable = unionOutputs(f.read, kept, output)
	}
	switch {
	case sameOutputContent(kept, output):
	case unionable:
		f.flat[prev.index] = united
	case kept.OmitsRules && !output.OmitsRules:
		f.noteOmitting(output.Path, prev.preset)
		f.flat[prev.index] = output
		f.seenPaths[output.Path] = presetClaim{preset: presetName, index: prev.index}
	case !kept.OmitsRules && output.OmitsRules:
		f.noteOmitting(output.Path, presetName)
	default:
		if _, seen := f.conflicting[output.Path]; !seen {
			f.conflictPaths = append(f.conflictPaths, output.Path)
		}
		f.conflicting[output.Path] = append(f.conflicting[output.Path], presetName)
	}
}

// noteOmitting records that preset's rule-less version of path yielded.
func (f *presetFlattener) noteOmitting(path, preset string) {
	f.omitting[path] = append(f.omitting[path], preset)
	if len(f.omitting[path]) == 1 {
		f.omittingPaths = append(f.omittingPaths, path)
	}
}

// warnOmitting names, once per path, the presets whose rule-less file yielded.
func (f *presetFlattener) warnOmitting(d *diag.Collector) {
	for _, path := range f.omittingPaths {
		// Through the shared sink, which says each message once per run: this
		// function runs more than once (rootAgentsMD, scopes) and clean silences it.
		d.Warn("Presets with a rules folder and presets without one write the same file ("+path+"); "+
			"keeping the version that inlines every rule. Set agents_md = true or rules.mode = \"inline\" to share it",
			"path", path, "kept_from", f.seenPaths[path].preset, "rules_in_folder", strings.Join(f.omitting[path], ", "))
	}
}

// conflictError returns the error naming every path that presets rendered
// differently, or nil when there is none.
func (f *presetFlattener) conflictError() error {
	if len(f.conflictPaths) == 0 {
		return nil
	}
	conflicts := make([]string, 0, len(f.conflictPaths))
	for _, path := range f.conflictPaths {
		conflicts = append(conflicts, fmt.Sprintf("%s (%s differs from %s)",
			path, strings.Join(f.conflicting[path], ", "), f.seenPaths[path].preset))
	}
	hint := "Presets that write the same file must render identical content; " +
		"drop one of the presets or report the divergence"
	if slices.Contains(f.conflicting[".mcp.json"], "qoder") || f.seenPaths[".mcp.json"].preset == "qoder" {
		hint += ". qoder documents no ${VAR} expansion, so it cannot share .mcp.json with a tool " +
			"that reads references; drop one of them or supply the secret through --env or .env"
	}
	return oops.
		With("conflicts", strings.Join(conflicts, "; ")).
		Hint(hint).
		Errorf("presets write different content to the same path: %s", strings.Join(conflicts, "; "))
}

// sameOutputContent reports whether two outputs for one path hold the same bytes
// and are written with the same protections.
func sameOutputContent(a, b config.OutputFile) bool {
	return a.Content == b.Content && bytes.Equal(a.RawContent, b.RawContent) &&
		a.Sensitive == b.Sensitive && a.LocalOnly == b.LocalOnly
}
