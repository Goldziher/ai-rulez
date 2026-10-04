package generator

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/jsonmerge"
	"github.com/Goldziher/ai-rulez/internal/generator/plugin"
	"github.com/Goldziher/ai-rulez/internal/generator/presets"   // Register remaining legacy preset generators
	"github.com/Goldziher/ai-rulez/internal/generator/providers" // Register DSL-backed preset generators (overrides legacy registrations where they overlap)
	"github.com/Goldziher/ai-rulez/internal/generator/rulefiles"
	"github.com/Goldziher/ai-rulez/internal/gitignore"
	"github.com/Goldziher/ai-rulez/internal/gitutil"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/Goldziher/ai-rulez/internal/templates"
	"github.com/Goldziher/ai-rulez/schema"
	"github.com/samber/oops"
)

const defaultProfileName = "default"
const generatedManifestName = ".generated-manifest.json"

// generatedLocalManifestName is the gitignored manifest of machine-local
// outputs. They stay out of the committed manifest so a teammate's run never
// treats them as stale.
const generatedLocalManifestName = ".generated-manifest.local.json"

// localSourceDirName is the subdirectory of the config dir (.ai-rulez/local)
// holding machine-local override content. It is always gitignored.
const localSourceDirName = "local"

// Generator handles configuration generation
type Generator struct {
	config *config.Config
	// previousFiles is the prior generated manifest as a set, read once per
	// writeOutputs pass for the rules-folder overwrite guard.
	previousFiles map[string]bool
	// skippedPaths holds the relative paths the overwrite guard left alone in
	// the latest writeOutputs pass. They are hand-written, so they stay out of
	// the generated manifest and the managed .gitignore block.
	skippedPaths map[string]bool

	// Machine-local overlay handling (see local_drift.go).
	ctx             context.Context // caller's context for the baseline load (nil means Background)
	allowLocalDrift bool            // write merged output even when it drifts from the shared baseline
	lenientMCP      bool            // tolerate unresolved MCP placeholders (baseline renders)
	plan            *localPlan      // baseline comparison for this run; nil without local inputs
	localSkipped    bool            // local files exist on disk but were not loaded (--no-local)

	manifests map[string]generatedManifest // manifests read this run, by path
	warned    map[string]bool              // merged-document warnings already issued by this Generator
}

type generatedManifest struct {
	Version string   `json:"version"`
	Files   []string `json:"files"`
	// Merged records what ai-rulez wrote into each merged JSON document (see
	// merged_claims.go): in the committed manifest for a document it wrote whole,
	// in the machine-local manifest for one shared with the user.
	Merged map[string][]jsonmerge.Claim `json:"merged,omitempty"`
}

// NewGenerator creates a new generator
func NewGenerator(cfg *config.Config) *Generator {
	return &Generator{
		config: cfg,
	}
}

// generateMu serializes generate runs. The downgrade collector in rulefiles is
// process-global, so two concurrent runs (for example from the MCP server)
// would reset and flush each other's entries.
var generateMu sync.Mutex

// Generate generates all outputs for the specified profile.
func (g *Generator) Generate(profile string) error {
	_, err := g.GenerateFiles(profile)
	return err
}

// GenerateFiles generates all outputs for the specified profile and returns the
// number of files it wrote, directories excluded, so a caller reporting a total
// to the user can report a counted one.
func (g *Generator) GenerateFiles(profile string) (int, error) {
	generateMu.Lock()
	defer generateMu.Unlock()
	g.beginRun()
	rulefiles.ResetDowngrades()
	defer rulefiles.FlushDowngrades()

	flatOutputs, activeProfile, err := g.collectOutputs(profile)
	if err != nil {
		return 0, err
	}

	// The machine-local inputs (overlay, local/ tree) are ignored before any check
	// can refuse the run, so a refused first run never leaves them unignored.
	if err := g.ignoreLocalInputs(); err != nil {
		return 0, err
	}

	if err := g.guardLocal(profile, flatOutputs); err != nil {
		return 0, err
	}

	logger.Info("Generating with configuration", "profile", activeProfile)

	if err := g.ensureSecretOutputsIgnored(flatOutputs); err != nil {
		return 0, err
	}
	g.markSensitiveOutputs(flatOutputs)

	ignoredEarly, err := g.ignoreBeforeWriting(flatOutputs)
	if err != nil {
		return 0, err
	}

	staleFiles := g.staleManifestFiles(flatOutputs)
	g.removeStaleManifestFiles(staleFiles)

	// Write all output files
	if err := g.writeOutputs(flatOutputs); err != nil {
		return 0, err
	}

	// Merged documents lose what an earlier run merged in and this one does not
	// (a preset or server that was removed). Planned after the write so that
	// what this run claimed counts.
	unmerged := g.planUnmerge(flatOutputs, false)
	g.applyUnmerge(unmerged)

	// Writing happens first: a directory the stale pass emptied may be one this
	// run re-creates, and pruning before the write would only have it made again.
	g.pruneDirsEmptiedBy(append(staleFiles, deletedPaths(unmerged)...))

	if err := g.writeGeneratedManifest(flatOutputs); err != nil {
		if g.hasLocalOutputs(flatOutputs) {
			// Without the local manifest a later run cannot clean these files up.
			return 0, oops.Wrapf(err, "write the generated manifests")
		}
		logger.Warn("Failed to write generated manifest", "error", err)
	}

	g.finishGitignore(flatOutputs, ignoredEarly)

	written := 0
	for _, output := range flatOutputs {
		if !output.IsDir {
			written++
		}
	}

	logger.Info("Generation complete", "files", written)

	return written, nil
}

// ignoreBeforeWriting makes sure machine-local files and MCP configs holding
// secrets are git-ignored before any is written, and refuses the run when git
// still would not ignore them. It reports whether it wrote the ignore entries.
func (g *Generator) ignoreBeforeWriting(outputs []config.OutputFile) (bool, error) {
	if !g.hasLocalOutputs(outputs) && !g.hasGuardedSecretOutputs(outputs) {
		return false, nil
	}
	if err := g.updateGitignore(outputs); err != nil {
		return false, oops.Wrapf(err, "gitignore machine-local outputs before writing them")
	}
	return true, g.verifyGuardedOutputsIgnored(outputs)
}

// finishGitignore updates .gitignore if enabled, or whenever machine-local
// content exists: local ".local" outputs and the .ai-rulez/local/ source subtree
// are gitignored unconditionally, even when config gitignore is disabled. The
// early pass already wrote the same block unless writing skipped a hand-written
// file, which drops that file's entry.
func (g *Generator) finishGitignore(outputs []config.OutputFile, ignoredEarly bool) {
	if ignoredEarly && len(g.skippedPaths) == 0 {
		return
	}
	if g.config.ShouldUpdateGitignore() || g.hasLocalGitignoreTargets() {
		if err := g.updateGitignore(outputs); err != nil {
			logger.Warn("Failed to update .gitignore", "error", err)
		}
	}
}

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
	outputs, err := g.collectPluginOutputs(profile)
	if err != nil {
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
	logger.Info("Plugin generation complete", "files", written)
	return written, nil
}

// VerifyPlugin verifies the generated plugin bundles against their provenance
// sidecars without regenerating or modifying files.
func (g *Generator) VerifyPlugin(profile string) error {
	expected, err := g.collectPluginOutputs(profile)
	if err != nil {
		return oops.Wrapf(err, "render expected plugin outputs")
	}
	for _, output := range expected {
		if output.IsDir {
			continue
		}
		actual, readErr := os.ReadFile(output.Path)
		if readErr != nil {
			return oops.With("path", output.Path).Wrapf(readErr, "read generated plugin output")
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
	if marketplace := g.config.Marketplace; marketplace != nil && len(marketplace.Members) > 0 {
		for _, member := range marketplace.Members {
			if err := plugin.VerifyProvenance(filepath.Join(g.config.BaseDir, member)); err != nil {
				return oops.With("member", member).Wrapf(err, "verify member plugin bundle")
			}
		}
	}
	return plugin.VerifyProvenance(g.config.BaseDir)
}

// DryRunPlugin returns the plugin generation plan without writing files.
func (g *Generator) DryRunPlugin(profile string) ([]string, error) {
	outputs, err := g.collectPluginOutputs(profile)
	if err != nil {
		return nil, err
	}
	lines := make([]string, 0, len(outputs)+1)
	lines = append(lines, "plugin bundle:")
	for _, output := range outputs {
		lines = append(lines, "write-file: "+g.convertToRelativePath(g.absOutputPath(output.Path)))
	}
	return lines, nil
}

// collectPluginOutputs resolves the content tree and MCP servers, builds the
// plugin manifest, and renders all requested runtime bundles + marketplace. When
// the config is a monorepo root ([marketplace].members set), it instead renders
// each member's bundle plus the aggregate marketplace index.
func (g *Generator) collectPluginOutputs(profile string) ([]config.OutputFile, error) {
	if mkt := g.config.Marketplace; mkt != nil && len(mkt.Members) > 0 {
		return g.collectMonorepoOutputs(mkt)
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
		contentTree, err = config.ScanContentTree(filepath.Join(g.config.BaseDir, g.config.Plugin.ContentRoot))
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

// collectMonorepoOutputs renders every member plugin under its source directory
// and emits the aggregate root marketplace index. Each member is an independent
// ai-rulez project loaded from <baseDir>/<member>.
func (g *Generator) collectMonorepoOutputs(mkt *config.MarketplaceAuthoring) ([]config.OutputFile, error) {
	var outputs []config.OutputFile
	entries := make([]plugin.MemberEntry, 0, len(mkt.Members))

	for _, member := range mkt.Members {
		memberDir := filepath.Join(g.config.BaseDir, member)
		memberCfg, err := config.LoadConfig(context.Background(), memberDir, config.WithoutLocal())
		if err != nil {
			return nil, oops.With("member", member).Wrapf(err, "load monorepo member config")
		}
		if memberCfg.Plugin == nil {
			return nil, oops.
				With("member", member).
				Hint("Each monorepo member must define its own [plugin] block").
				Errorf("monorepo member %q has no [plugin] block", member)
		}

		memberGen := NewGenerator(memberCfg)
		manifest, err := memberGen.buildPluginManifest("")
		if err != nil {
			return nil, oops.With("member", member).Wrapf(err, "build member manifest")
		}

		memberOutputs, err := plugin.GenerateMember(manifest, memberCfg.BaseDir)
		if err != nil {
			return nil, oops.With("member", member).Wrapf(err, "generate member bundle")
		}
		outputs = append(outputs, memberOutputs...)

		entries = append(entries, plugin.MemberEntry{
			Name:        manifest.Name,
			Description: manifest.Description,
			Source:      "./" + filepath.ToSlash(member),
			Category:    manifest.Category,
		})
	}

	market := plugin.ResolveMarketInfo(mkt)
	marketplaceOutput, err := plugin.RenderMonorepoMarketplace(market, entries, g.config.BaseDir)
	if err != nil {
		return nil, oops.Wrapf(err, "render monorepo marketplace")
	}
	codexMarketplaceOutput, err := plugin.RenderCodexMonorepoMarketplace(market, entries, g.config.BaseDir)
	if err != nil {
		return nil, oops.Wrapf(err, "render Codex monorepo marketplace")
	}
	rootOutputs, err := plugin.AddProvenance(
		[]config.OutputFile{marketplaceOutput, codexMarketplaceOutput},
		g.config.BaseDir,
	)
	if err != nil {
		return nil, oops.Wrapf(err, "add marketplace provenance")
	}
	return append(outputs, rootOutputs...), nil
}

// DryRunBlocked reports whether the plan from the last DryRun contains local
// drift that Generate would refuse to write (nil when it is allowed or absent).
// DryRun itself still returns the full plan, including the blocked lines.
func (g *Generator) DryRunBlocked() error {
	generateMu.Lock()
	defer generateMu.Unlock()
	if g.plan == nil {
		return nil
	}
	return g.plan.check(g.allowLocalDrift)
}

// DryRun returns an inspectable generation plan without writing or deleting files.
func (g *Generator) DryRun(profile string) ([]string, error) {
	generateMu.Lock()
	defer generateMu.Unlock()
	g.beginRun()
	rulefiles.ResetDowngrades()
	defer rulefiles.FlushDowngrades()

	flatOutputs, activeProfile, err := g.collectOutputs(profile)
	if err != nil {
		return nil, err
	}

	plan, err := g.planLocal(profile, flatOutputs)
	if err != nil {
		return nil, err
	}

	lines := []string{fmt.Sprintf("profile: %s", activeProfile)}
	if plan != nil {
		lines = append(lines, plan.dryRunLines()...)
	}
	lines = append(lines, g.planLines(flatOutputs)...)
	for _, stale := range g.staleManifestFiles(flatOutputs) {
		lines = append(lines, "delete-stale: "+g.convertToRelativePath(stale))
	}
	for _, edit := range g.planUnmerge(flatOutputs, false) {
		if edit.delete {
			lines = append(lines, "delete-stale: "+edit.rel)
		} else {
			lines = append(lines, "unmerge: "+edit.rel)
		}
	}
	return lines, nil
}

// planLines lists the directories and files a run would create. A file the
// overwrite guard would leave alone is not listed, because it is not written.
func (g *Generator) planLines(outputs []config.OutputFile) []string {
	g.previousFiles = nil
	defer func() { g.previousFiles = nil }()
	var lines []string
	for _, output := range outputs {
		abs := g.absOutputPath(output.Path)
		relPath := g.convertToRelativePath(abs)
		switch {
		case output.IsDir:
			lines = append(lines, "create-dir: "+relPath)
		case output.RawContent == nil && g.isUnmanagedRuleFile(abs, g.finalContent(output)):
		default:
			lines = append(lines, "write-file: "+relPath)
		}
	}
	return lines
}

// disambiguatedRuleName inserts ".ai-rulez" before the extension of a rule
// file path: "x.md" becomes "x.ai-rulez.md", "x.instructions.md" becomes
// "x.ai-rulez.instructions.md".
func disambiguatedRuleName(p string) string {
	ext := filepath.Ext(p)
	if strings.HasSuffix(p, ".instructions.md") {
		ext = ".instructions.md"
	}
	return strings.TrimSuffix(p, ext) + ".ai-rulez" + ext
}

// disambiguateRuleCollisions renames a generated rule file that would collide
// with a hand-written file of the same name to "<id>.ai-rulez<ext>", so the
// rule still reaches the tool instead of being skipped. It runs while the
// outputs are collected rather than in writeOutput because the new name must be
// what the manifest, the managed .gitignore block, the dry run and clean all
// see, and all of them start from the collected outputs. The name depends only
// on the hand-written file existing, so it is stable across runs, and once the
// file is gone the rule goes back to its plain name and the manifest entry of
// the renamed file makes the next run delete it. Machine-local rule files keep
// the skip-and-warn behavior: their ".local." names are reserved.
func (g *Generator) disambiguateRuleCollisions(outputs []config.OutputFile) {
	g.previousFiles = nil
	defer func() { g.previousFiles = nil }()
	for i, output := range outputs {
		if output.IsDir || output.LocalOnly || output.RawContent != nil {
			continue
		}
		abs := g.absOutputPath(output.Path)
		if !config.InRulesDir(filepath.ToSlash(g.convertToRelativePath(abs))) ||
			!g.isUnmanagedRuleFile(abs, g.finalContent(output)) {
			continue
		}
		renamed := output
		renamed.Path = disambiguatedRuleName(output.Path)
		if g.isUnmanagedRuleFile(g.absOutputPath(renamed.Path), g.finalContent(renamed)) {
			continue // the new name is taken by a hand-written file too: the guard skips it
		}
		logger.Warn("A hand-written rule file has the same name as a generated rule; "+
			"the generated rule was written under another name, rename one of them to silence this",
			"hand_written", output.Path, "generated", renamed.Path)
		outputs[i] = renamed
	}
}

func (g *Generator) collectOutputs(profile string) ([]config.OutputFile, string, error) {
	if err := g.resolveMCPEnv(); err != nil {
		return nil, "", err
	}

	activeProfile := g.resolveProfile(profile)

	contentTree, err := g.getContentForProfile(activeProfile)
	if err != nil {
		return nil, "", err
	}

	logger.Debug("Content scanned",
		"rules", len(contentTree.Rules),
		"context", len(contentTree.Context),
		"skills", len(contentTree.Skills),
		"agents", len(contentTree.Agents),
		"domains", len(contentTree.Domains))

	presets.WarnDuplicateContent(contentTree)

	// Collect MCP servers based on the resolved content tree and active profile
	mcpServers := g.collectMCPServersForContent(contentTree, activeProfile)

	// Resolve the run's header timestamp once, before any renderer reads it, so
	// every file this run writes carries the same value. Each preset used to call
	// time.Now() for itself, which made CLAUDE.md and AGENTS.md — byte-identical
	// otherwise — disagree whenever the two renders straddled a second boundary.
	// A caller that set GeneratedAt explicitly keeps its value.
	if g.config.GeneratedAt.IsZero() {
		g.config.GeneratedAt = config.ResolveGenerationTime()
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
		return nil, "", oops.Wrapf(err, "generate presets")
	}

	applySharedOutputs(allOutputs, &tempCfg, contentTree)

	// Auto-generate MCP output if servers exist. The MCP preset is now
	// DSL-driven (internal/generator/providers/builtin/mcp.toml); fetch it
	// from the registry rather than instantiating a hand-written generator.
	if len(mcpServers) > 0 || g.config.HasSelfServer() {
		mcpGen, err := config.GetPresetGenerator("mcp")
		if err != nil {
			logger.Warn("Failed to resolve MCP preset generator", "error", err)
		} else {
			mcpOutputs, err := mcpGen.Generate(contentTree, g.config.BaseDir, &tempCfg)
			if err != nil {
				logger.Warn("Failed to generate MCP output", "error", err)
			} else if len(mcpOutputs) > 0 {
				allOutputs["mcp"] = mcpOutputs
				logger.Debug("Auto-generated MCP output", "count", len(mcpOutputs))
			}
		}
	}

	// Append machine-local root variants (CLAUDE.local.md, AGENTS.local.md, ...)
	// keyed by preset so they flow through the same flatten/manifest/stale path:
	// duplicate local paths (codex + opencode both emit AGENTS.local.md) collapse,
	// and removed local content deletes the file via stale-manifest cleanup.
	if err := g.appendLocalOutputs(allOutputs, &tempCfg, activeProfile); err != nil {
		return nil, "", err
	}

	// Flatten outputs for writing, detecting conflicts and deduplicating
	flatOutputs := flattenPresetOutputs(allOutputs)

	scopedOutputs, err := g.generateScopedOutputs(activeProfile, contentTree, run)
	if err != nil {
		return nil, "", err
	}
	flatOutputs = append(flatOutputs, scopedOutputs...)
	g.disambiguateRuleCollisions(flatOutputs)
	g.reclaimStaleMembers(flatOutputs)

	return flatOutputs, activeProfile, nil
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
		generator, err := config.GetPresetGenerator(preset.BuiltIn)
		if err != nil {
			logger.Debug("Skipping local outputs for unknown preset", "preset", name, "error", err)
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
				warnDroppedLocal(name, rules, allContext)
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
		rulefiles.Warn(presets.AgentsOverrideFile+" exists and was not written by ai-rulez, so it is left alone and "+
			"the presets that read it ("+names+") do not load your machine-local content",
			"hint", "move or delete the file to let ai-rulez write it", "path", presets.AgentsOverrideFile)
		return
	}
	agentsMD, ok := rootAgentsMD(allOutputs, g.config.BaseDir)
	if !ok {
		rulefiles.Warn("machine-local content exists for "+names+", but this run produces no AGENTS.md, so "+
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
	if !pathIsFile(path) {
		return false
	}
	rel := filepath.ToSlash(g.convertToRelativePath(path))
	if slices.Contains(g.previousManifestFiles(), rel) {
		return false
	}
	return !looksGenerated(path)
}

// rootAgentsMD returns the content of the project's AGENTS.md as written this
// run: the winner of flattenPresetOutputs, which is the shared one with agents_md
// and otherwise the last preset in name order to render one.
func rootAgentsMD(allOutputs map[string][]config.OutputFile, baseDir string) (string, bool) {
	path := filepath.Join(baseDir, string(config.SharedAgentsMD))
	for _, o := range flattenPresetOutputs(allOutputs) {
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
	for _, r := range rules {
		items = append(items, "rule "+r.Name)
	}
	for _, c := range contexts {
		items = append(items, "context "+c.Name)
	}
	return items
}

// warnDroppedLocal warns about local rules and context a preset has no output
// for: it writes no local root file and does not route them to rule files.
func warnDroppedLocal(preset string, rules, contexts []config.ContentFile) {
	items := droppedLocalItems(rules, contexts)
	if len(items) == 0 {
		return
	}
	warnLocal("Machine-local content has no output for this preset and was not written",
		"preset", preset, "items", strings.Join(items, ", "))
}

// warnLocal reports local content a preset has no place for; a variable so tests
// can observe the warnings.
var warnLocal = logger.Warn

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

// flattenPresetOutputs merges outputs from all presets, deduplicating directories
// and detecting file conflicts (last write wins with a warning).
func flattenPresetOutputs(allOutputs map[string][]config.OutputFile) []config.OutputFile {
	var flatOutputs []config.OutputFile
	seenPaths := make(map[string]string) // path -> first preset that claimed it
	seenDirs := make(map[string]bool)
	presetNames := make([]string, 0, len(allOutputs))
	for presetName := range allOutputs {
		presetNames = append(presetNames, presetName)
	}
	sort.Strings(presetNames)
	for _, presetName := range presetNames {
		outputs := allOutputs[presetName]
		logger.Debug("Generated outputs for preset", "preset", presetName, "count", len(outputs))
		for _, output := range outputs {
			if output.IsDir {
				if !seenDirs[output.Path] {
					seenDirs[output.Path] = true
					flatOutputs = append(flatOutputs, output)
				}
				continue
			}
			if prev, ok := seenPaths[output.Path]; ok {
				// Multiple presets emitting the same path (e.g. cursor + copilot + auto-mcp
				// all writing .mcp.json) is the expected case, not a configuration error.
				logger.Debug("Multiple presets write to the same file, last write wins",
					"path", output.Path, "presets", prev+" and "+presetName)
				// Replace the existing entry with the latest version
				for i, existing := range flatOutputs {
					if existing.Path == output.Path {
						flatOutputs[i] = output
						break
					}
				}
				seenPaths[output.Path] = presetName
			} else {
				seenPaths[output.Path] = presetName
				flatOutputs = append(flatOutputs, output)
			}
		}
	}
	return flatOutputs
}

func (g *Generator) writeOutputs(outputs []config.OutputFile) error {
	g.previousFiles = nil
	g.skippedPaths = make(map[string]bool)
	defer func() { g.previousFiles = nil }()
	for _, output := range outputs {
		if err := g.writeOutput(output); err != nil {
			return oops.
				With("path", output.Path).
				Wrapf(err, "write output file")
		}
	}
	return nil
}

func (g *Generator) absOutputPath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(g.config.BaseDir, path)
}

// writeOutput writes a single output file or creates a directory.
//
// Skip decision: we hash the freshly-rendered body and compare two values
// embedded in the existing file's header — the Source-Hash (changes when any
// source input changes) and the Content-Hash (changes when this output's body
// changes). The comparison is against the in-header hashes, never against the
// on-disk body, so skipping is robust to formatters or other tools that touch
// the body after we wrote it. We only skip when BOTH match: a Source-Hash
// mismatch alone forces a rewrite to refresh stale provenance metadata even
// when the body would round-trip identically.
//
// On write, output is normalized to end with exactly one trailing newline so
// formatters that enforce that convention (end-of-file-fixer, etc.) don't
// spuriously modify the file after generation.
func (g *Generator) writeOutput(output config.OutputFile) error {
	absPath := g.absOutputPath(output.Path)

	if output.IsDir {
		if err := os.MkdirAll(absPath, 0o755); err != nil {
			return oops.
				With("dir", absPath).
				Hint(fmt.Sprintf("Check directory permissions for: %s", absPath)).
				Wrapf(err, "create directory")
		}
		logger.Debug("Created directory", "path", output.Path)
		return nil
	}

	// Raw mode: write bytes verbatim. Used for skill resources (references,
	// scripts, assets) where the standard header banner would corrupt the
	// payload (e.g. Python scripts) or break binary files.
	if output.RawContent != nil {
		return writeRawOutput(absPath, output)
	}

	finalContent := g.finalContent(output)

	if g.isUnmanagedRuleFile(absPath, finalContent) {
		if g.skippedPaths == nil {
			g.skippedPaths = make(map[string]bool)
		}
		g.skippedPaths[filepath.ToSlash(g.convertToRelativePath(absPath))] = true
		if output.LocalOnly {
			logger.Warn("Skipped existing hand-written file that collides with a machine-local rule file; "+
				"*.local.* names in rules folders are reserved for ai-rulez local rules, rename the file",
				"path", output.Path)
			return nil
		}
		logger.Warn("Skipped existing hand-written rule file that collides with a generated rule; rename one of them"+
			g.unmanagedHint(),
			"path", output.Path, "rule", strings.TrimSuffix(filepath.Base(output.Path), filepath.Ext(output.Path)))
		return nil
	}

	if g.canSkipWrite(absPath, output, finalContent) {
		if output.Sensitive {
			if err := os.Chmod(absPath, sensitiveFileMode); err != nil {
				return oops.With("path", absPath).Wrapf(err, "restrict permissions of a file carrying secrets")
			}
		}
		logger.Debug("Skipped unchanged file", "path", output.Path)
		return nil
	}

	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return oops.
			With("dir", dir).
			With("path", absPath).
			Hint(fmt.Sprintf("Check directory permissions for: %s", dir)).
			Wrapf(err, "create parent directory")
	}

	if output.Sensitive {
		// Owner-only temp file renamed into place: the secret is never on disk
		// with a wider mode, and an existing world-readable file is replaced.
		if err := config.WriteFileAtomic(absPath, []byte(finalContent), sensitiveFileMode); err != nil {
			return oops.
				With("path", absPath).
				Hint(fmt.Sprintf("Check write permissions for: %s", absPath)).
				Wrapf(err, "write file")
		}
		logger.Debug("Wrote file", "path", output.Path, "size", len(finalContent), "mode", sensitiveFileMode)
		return nil
	}

	if err := os.WriteFile(absPath, []byte(finalContent), 0o644); err != nil {
		return oops.
			With("path", absPath).
			Hint(fmt.Sprintf("Check write permissions for: %s", absPath)).
			Wrapf(err, "write file")
	}

	logger.Debug("Wrote file", "path", output.Path, "size", len(finalContent))
	return nil
}

// isUnmanagedRuleFile reports whether absPath is an existing file inside a
// shared rules folder that ai-rulez did not write. A file counts as ours when
// any of these hold: it is in the previous generated manifest, it stores a
// Content-Hash, it carries a generated-file banner, or its bytes already equal
// wantContent (so a fresh clone with no manifest and hashes = "none" does not
// mistake our own output for a hand-written file).
func (g *Generator) isUnmanagedRuleFile(absPath, wantContent string) bool {
	rel := filepath.ToSlash(g.convertToRelativePath(absPath))
	if !config.InRulesDir(rel) {
		return false
	}
	info, err := os.Stat(absPath)
	if err != nil || info.IsDir() {
		return false
	}
	if g.previousFiles == nil {
		g.previousFiles = make(map[string]bool)
		for _, f := range g.previousManifestFiles() {
			g.previousFiles[filepath.ToSlash(f)] = true
		}
	}
	if g.previousFiles[rel] {
		return false
	}
	if contentHash, _ := extractStoredHashes(absPath); contentHash != "" {
		return false
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return false
	}
	if string(data) == wantContent {
		return false
	}
	return !hasGeneratedBanner(absPath, data)
}

// unmanagedHint explains why a generated file may have been mistaken for a
// hand-written one when the header carries no recognizable marker.
func (g *Generator) unmanagedHint() string {
	if g.config.GetHeaderHashes() == config.HeaderHashesNone || g.config.Header.GetCustomHeader() != "" {
		return " (generated files carry no recognizable header with [header] hashes = \"none\" or custom header text;" +
			" if this file was generated by ai-rulez, delete it and regenerate)"
	}
	return ""
}

// generatedBannerMarkers are the strings generated headers and rule banners carry.
var generatedBannerMarkers = [...]string{"GENERATED FILE", "Generated by ai-rulez"}

// extHTML is the only markup extension the banner check treats like markdown.
const extHTML = ".html"

// bannerScanLimit bounds how much of a file the banner check reads.
const bannerScanLimit = 16 * 1024

// frontmatterEnd locates a leading YAML frontmatter block. open reports that
// the content starts with a "---" line (LF or CRLF); end is the offset just past
// the closing "---" line, or 0 when the block is not closed. One helper serves
// the banner check, hash injection and header stripping, so files with CRLF line
// endings (a checkout with autocrlf, an editor that converts them) are read the
// same way as the LF files ai-rulez writes.
func frontmatterEnd(s string) (end int, open bool) {
	var first int
	switch {
	case strings.HasPrefix(s, "---\n"):
		first = len("---\n")
	case strings.HasPrefix(s, "---\r\n"):
		first = len("---\r\n")
	default:
		return 0, false
	}
	for pos := first; pos < len(s); {
		nl := strings.IndexByte(s[pos:], '\n')
		line, next := s[pos:], len(s)
		if nl >= 0 {
			line, next = s[pos:pos+nl], pos+nl+1
		}
		if strings.TrimSuffix(line, "\r") == frontmatterFence {
			return next, true
		}
		pos = next
	}
	return 0, true
}

// skipEOL drops one or two line breaks (LF or CRLF) from the start of s: the
// blank line that follows a banner.
func skipEOL(s string) string {
	for i := 0; i < 2; i++ {
		switch {
		case strings.HasPrefix(s, "\r\n"):
			s = s[2:]
		case strings.HasPrefix(s, "\n"):
			s = s[1:]
		default:
			return s
		}
	}
	return s
}

// hasGeneratedBanner reports whether data starts with a generated-file banner:
// the first comment after the optional frontmatter. For markdown that is an HTML
// comment, for any other extension also a leading block of line comments. A
// marker deeper in the file (documentation that quotes the banner, a hand-written
// rule that mentions it) does not count.
func hasGeneratedBanner(path string, data []byte) bool {
	if len(data) > bannerScanLimit {
		data = data[:bannerScanLimit]
	}
	text := string(data)
	if end, _ := frontmatterEnd(text); end > 0 {
		text = text[end:]
	}
	text = strings.TrimLeft(text, " \t\r\n")
	if strings.HasPrefix(text, "<!--") {
		if end := strings.Index(text, "-->"); end >= 0 {
			text = text[:end]
		}
		return containsBannerMarker(text)
	}
	if isMarkdownRuleExt(path) || strings.EqualFold(filepath.Ext(path), extHTML) {
		return false
	}
	var header strings.Builder
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "//") && !strings.HasPrefix(trimmed, ";") {
			break
		}
		header.WriteString(trimmed + "\n")
	}
	return containsBannerMarker(header.String())
}

// isMarkdownRuleExt reports whether path has a markdown-family extension, the
// files whose headers are HTML comments.
func isMarkdownRuleExt(path string) bool {
	return slices.Contains(markdownRuleExts, strings.ToLower(filepath.Ext(path)))
}

var markdownRuleExts = strings.Fields(".md .mdc .markdown .mdx")

func containsBannerMarker(header string) bool {
	for _, marker := range generatedBannerMarkers {
		if strings.Contains(header, marker) {
			return true
		}
	}
	return false
}

// finalContent is the exact text writeOutput puts on disk for a rendered
// output: the body plus the freshness lines the header hash mode asks for,
// normalized to a single trailing newline.
func (g *Generator) finalContent(output config.OutputFile) string {
	contentHash := templates.HashContent(stripHeader(output.Content, output.Path))
	sourceHash := g.sourceHashFor(output)
	switch g.config.GetHeaderHashes() {
	case config.HeaderHashesNone:
		contentHash, sourceHash = "", ""
	case config.HeaderHashesContent:
		sourceHash = ""
	}
	return normalizeTrailingNewline(injectHashes(output.Content, output.Path, contentHash, sourceHash))
}

// canSkipWrite reports whether the file on disk already matches what would be
// written.
//
// In "full" mode the comparison is the in-header Content-Hash plus Source-Hash,
// never the on-disk body, so a formatter touching the body does not matter.
// "content" and "none" have no Source-Hash to carry header changes (style, text,
// config directory), and "none" has no hash at all, so they compare the whole
// rendered file. With [header] timestamp enabled the Generated: text is ignored
// in that comparison, otherwise every run would rewrite every file.
func (g *Generator) canSkipWrite(absPath string, output config.OutputFile, finalContent string) bool {
	if g.config.GetHeaderHashes() == config.HeaderHashesFull {
		contentHash := templates.HashContent(stripHeader(output.Content, output.Path))
		existingContentHash, existingSourceHash, legacy := scanStoredHashes(absPath)
		if legacy && config.InRulesDir(output.Path) {
			// Hashes in the frontmatter are the pre-banner layout: rewrite once.
			return false
		}
		return existingContentHash != "" && existingContentHash == contentHash &&
			existingSourceHash == g.sourceHashFor(output)
	}
	existing, err := os.ReadFile(absPath)
	if err != nil {
		return false
	}
	if g.config.ShowHeaderTimestamp() {
		return equalIgnoringHeaderStamp(string(existing), finalContent, output.Path)
	}
	return string(existing) == finalContent
}

// equalIgnoringHeaderStamp compares two renderings of outputPath, ignoring the
// Generated: stamp in the header only. Bodies must match exactly, so a body line
// that happens to start with "Generated: " is still compared.
func equalIgnoringHeaderStamp(existing, final, outputPath string) bool {
	existingBody, finalBody := stripHeader(existing, outputPath), stripHeader(final, outputPath)
	if existingBody != finalBody {
		return false
	}
	existingHeader := strings.TrimSuffix(existing, existingBody)
	finalHeader := strings.TrimSuffix(final, finalBody)
	return generatedStampPattern.ReplaceAllString(existingHeader, "") ==
		generatedStampPattern.ReplaceAllString(finalHeader, "")
}

// generatedStampPattern matches the header timestamp in both its inline
// (" | Generated: ...") and standalone ("Generated: ...") forms.
var generatedStampPattern = regexp.MustCompile(`(?m)(?: \| )?Generated: [^\n]*$`)

// writeRawOutput writes an OutputFile with non-nil RawContent verbatim,
// preserving Mode (defaulting to 0o644). Skips the write when both the
// existing bytes and mode already match on disk so unchanged bundled
// assets don't dirty the working tree on every regeneration.
func writeRawOutput(absPath string, output config.OutputFile) error {
	mode := output.Mode.Perm()
	if mode == 0 {
		mode = 0o644
	}
	if output.Sensitive {
		mode &= sensitiveFileMode
		if mode == 0 {
			mode = sensitiveFileMode
		}
	}

	if rawWriteCanSkip(absPath, output.RawContent, mode) {
		logger.Debug("Skipped unchanged raw file", "path", output.Path)
		return nil
	}

	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return oops.
			With("dir", dir).
			With("path", absPath).
			Hint(fmt.Sprintf("Check directory permissions for: %s", dir)).
			Wrapf(err, "create parent directory")
	}
	if err := os.WriteFile(absPath, output.RawContent, mode); err != nil {
		return oops.
			With("path", absPath).
			Hint(fmt.Sprintf("Check write permissions for: %s", absPath)).
			Wrapf(err, "write file")
	}
	// os.WriteFile only applies the mode on file creation. To handle mode
	// changes on subsequent regenerations, chmod explicitly.
	if err := os.Chmod(absPath, mode); err != nil {
		return oops.
			With("path", absPath).
			With("mode", mode).
			Wrapf(err, "set file mode")
	}
	logger.Debug("Wrote raw file", "path", output.Path, "size", len(output.RawContent), "mode", mode)
	return nil
}

func rawWriteCanSkip(absPath string, payload []byte, mode os.FileMode) bool {
	existing, err := os.ReadFile(absPath)
	if err != nil || !bytes.Equal(existing, payload) {
		return false
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return false
	}
	return info.Mode().Perm() == mode
}

// normalizeTrailingNewline ensures the file ends with exactly one '\n'.
// Empty content stays empty.
func normalizeTrailingNewline(content string) string {
	if content == "" {
		return content
	}
	trimmed := strings.TrimRight(content, "\n")
	return trimmed + "\n"
}

// extractContentHash returns just the Content-Hash header value (or empty if absent).
// Kept for tests and consumers that don't care about Source-Hash.
func extractContentHash(filePath string) string {
	contentHash, _ := extractStoredHashes(filePath)
	return contentHash
}

// maxHeaderLines bounds how far extractStoredHashes reads past a frontmatter
// block (or into a file without one).
const maxHeaderLines = 60

// extractStoredHashes scans the header of an existing file and returns the
// Content-Hash and Source-Hash values found there (empty strings if missing).
// A file starting with a YAML frontmatter block is scanned through the closing
// "---" however long the block is, then up to maxHeaderLines more lines for the
// banner; other files are scanned for maxHeaderLines lines.
func extractStoredHashes(filePath string) (contentHash, sourceHash string) {
	contentHash, sourceHash, _ = scanStoredHashes(filePath)
	return contentHash, sourceHash
}

// scanStoredHashes is extractStoredHashes that also reports whether a hash was
// found inside the frontmatter block (the layout older versions used for
// rules-folder files). Lines of any length are handled and CRLF is accepted.
func scanStoredHashes(filePath string) (contentHash, sourceHash string, inFrontmatterBlock bool) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", "", false
	}
	defer file.Close()

	st := hashScan{budget: maxHeaderLines}
	reader := bufio.NewReader(file)
	for first := true; ; first = false {
		raw, readErr := reader.ReadString('\n')
		if raw == "" && readErr != nil {
			break // EOF, or a read error: end with what was found
		}
		if st.line(strings.TrimRight(raw, "\r\n"), first) || readErr != nil {
			break
		}
	}
	return st.contentHash, st.sourceHash, st.inBlock
}

// hashScan is the state of scanStoredHashes.
type hashScan struct {
	contentHash, sourceHash string
	inFrontmatter           bool // currently inside the frontmatter block
	inBlock                 bool // a hash was found inside the frontmatter block
	budget                  int  // lines left outside the frontmatter block
}

// line consumes one line and reports whether the scan is finished.
func (h *hashScan) line(raw string, first bool) bool {
	switch {
	case first && raw == "---":
		h.inFrontmatter = true
		return false
	case h.inFrontmatter && raw == "---":
		h.inFrontmatter = false
		return false
	case !h.inFrontmatter:
		h.budget--
		if h.budget < 0 {
			return true
		}
	}
	c, s := hashFromLine(raw)
	if c != "" {
		h.contentHash = c
	}
	if s != "" {
		h.sourceHash = s
	}
	if (c != "" || s != "") && h.inFrontmatter {
		h.inBlock = true
	}
	return h.contentHash != "" && h.sourceHash != ""
}

// hashFromLine extracts a Content-Hash or Source-Hash value from one header
// line, regardless of comment style (HTML, hash, slash, semicolon).
func hashFromLine(raw string) (contentHash, sourceHash string) {
	stripped := strings.TrimSpace(raw)
	for _, prefix := range []string{"<!--", "-->", "//", "#", ";", "/*", "*/"} {
		stripped = strings.TrimPrefix(stripped, prefix)
	}
	stripped = strings.TrimSpace(stripped)
	switch {
	case strings.HasPrefix(stripped, "Content-Hash: "):
		return strings.TrimPrefix(stripped, "Content-Hash: "), ""
	case strings.HasPrefix(stripped, "Source-Hash: "):
		return "", strings.TrimPrefix(stripped, "Source-Hash: ")
	}
	return "", ""
}

// stripHeader removes the header comment from generated content, returning
// only the body. The header format depends on the output file extension.
//
// Detection runs in the same priority order as injectHashes so that the body
// passed to the body hash is symmetric with the body the injection sees.
// Files may have multiple header layers (e.g. windsurf rule files have
// trigger frontmatter THEN a generated-file banner) — we strip them all,
// otherwise the banner's per-run timestamp would leak into the body hash.
func stripHeader(content, outputPath string) string {
	ext := strings.ToLower(filepath.Ext(outputPath))

	// 1. YAML frontmatter (skill/agent files, plus rule files that prepend
	// trigger frontmatter). Strip and continue — there may be a banner after.
	if end, _ := frontmatterEnd(content); end > 0 {
		// Drop a single leading blank line so banner detection works against
		// either "---\n\n<!--" or "---\n<!--" forms.
		content = strings.TrimPrefix(strings.TrimPrefix(content[end:], "\r"), "\n")
	}

	switch ext {
	case ".mdc", ".md", ".markdown", ".mdx", ".html":
		// Only a banner at the very start (after the frontmatter) is a header: a
		// "-->" further down belongs to the body (fenced HTML, comments) and
		// stripping up to it would truncate the hashed body. "# Heading" is a
		// heading in markdown, not a line comment, so no line-comment stripping.
		if !strings.HasPrefix(content, "<!--") {
			return content
		}
		if idx := strings.Index(content, "-->"); idx >= 0 {
			return skipEOL(content[idx+len("-->"):])
		}
		return content
	default:
		// Line-prefix comments (#, //, ;) — also covers .mdc which uses
		// "# title\n\nbody" with the title acting as a heading-shaped header.
		lines := strings.Split(content, "\n")
		i := 0
		for i < len(lines) {
			trimmed := strings.TrimSpace(lines[i])
			if trimmed == "" {
				i++
				break
			}
			isComment := strings.HasPrefix(trimmed, "#") ||
				strings.HasPrefix(trimmed, "//") ||
				strings.HasPrefix(trimmed, ";")
			if !isComment {
				break
			}
			i++
		}
		if i >= len(lines) {
			return ""
		}
		return strings.Join(lines[i:], "\n")
	}
}

// injectContentHash inserts a Content-Hash line into the header of the
// generated content. Backward-compatible thin wrapper around injectHashes.
func injectContentHash(content, outputPath, hash string) string {
	return injectHashes(content, outputPath, hash, "")
}

// injectHashes inserts Content-Hash and (optionally) Source-Hash lines into
// the header of the generated content. It locates the header closing marker
// and inserts the hash lines before it. If sourceHash is empty, only
// Content-Hash is injected (e.g., from older callers).
func injectHashes(content, outputPath, contentHash, sourceHash string) string {
	if contentHash == "" {
		return content
	}
	ext := strings.ToLower(filepath.Ext(outputPath))

	hashBlock := func(linePrefix string) string { return hashLines(linePrefix, contentHash, sourceHash) }

	// 0. Native rules folders: the tools' frontmatter parsers are not documented
	// to tolerate YAML comments (a failed parse can turn a scoped rule global or
	// drop it), so the hashes go into the HTML banner after the frontmatter.
	if out, ok := injectIntoRulesDirBanner(content, outputPath, hashBlock("")); ok {
		return out
	}

	// 1. YAML frontmatter (skill/agent files): inject as YAML comment lines
	// inside the frontmatter, before the closing "---". YAML parsers ignore
	// comments, so consumers see the same parsed fields.
	if strings.HasPrefix(content, "---\n") {
		rest := content[len("---\n"):]
		if idx := strings.Index(rest, "\n---\n"); idx >= 0 {
			closeIdx := len("---\n") + idx
			return content[:closeIdx] + "\n" + hashBlock("# ") + content[closeIdx:]
		}
	}

	// 2. HTML comment banner — only for true markdown/HTML extensions.
	switch ext {
	case ".md", ".markdown", ".mdx", ".html":
		marker := "\n-->\n"
		if idx := strings.Index(content, marker); idx >= 0 {
			return content[:idx] + "\n" + hashBlock("") + marker + content[idx+len(marker):]
		}
		return content
	}

	// 3. Line-prefix comments (yaml, ini, .mdc title-as-header, etc).
	prefix := "# "
	switch ext {
	case ".json", ".jsonc", ".go", ".js", ".ts", ".tsx", ".jsx", ".java", ".c", ".cc", ".cpp", ".cs":
		prefix = "// "
	case ".ini":
		prefix = "; "
	}

	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" && i > 0 {
			prevTrimmed := strings.TrimSpace(lines[i-1])
			isComment := strings.HasPrefix(prevTrimmed, "#") ||
				strings.HasPrefix(prevTrimmed, "//") ||
				strings.HasPrefix(prevTrimmed, ";")
			if isComment {
				hashLines := strings.Split(hashBlock(prefix), "\n")
				result := make([]string, 0, len(lines)+len(hashLines))
				result = append(result, lines[:i]...)
				result = append(result, hashLines...)
				result = append(result, lines[i:]...)
				return strings.Join(result, "\n")
			}
		}
	}
	return content
}

// hashLines renders the Content-Hash line and, when set, the Source-Hash line,
// each starting with linePrefix.
func hashLines(linePrefix, contentHash, sourceHash string) string {
	var b strings.Builder
	b.WriteString(linePrefix)
	b.WriteString("Content-Hash: ")
	b.WriteString(contentHash)
	if sourceHash != "" {
		b.WriteByte('\n')
		b.WriteString(linePrefix)
		b.WriteString("Source-Hash: ")
		b.WriteString(sourceHash)
	}
	return b.String()
}

// injectIntoRulesDirBanner is injectIntoBanner for files in a native rules
// folder; it reports false for any other output. A markdown rules-folder file
// without a banner (a provider spec that is not split writes such files) gets a
// minimal banner carrying the hashes, right after its frontmatter: the hashes
// must not go into the frontmatter, and without them the file would be
// rewritten on every run and could not be told apart from a hand-written one.
// Other extensions report false and take the generic fallback.
func injectIntoRulesDirBanner(content, outputPath, block string) (string, bool) {
	if !config.InRulesDir(outputPath) {
		return content, false
	}
	if out, ok := injectIntoBanner(content, block); ok {
		return out, true
	}
	if !isMarkdownRuleExt(outputPath) {
		return content, false
	}
	banner, ok := injectIntoBanner(templates.RuleBanner(""), block)
	if !ok {
		return content, false
	}
	end, open := frontmatterEnd(content)
	switch {
	case end > 0:
		return content[:end] + banner + strings.TrimPrefix(content[end:], "\n"), true
	case open:
		return content, false
	}
	return banner + content, true
}

// injectIntoBanner adds block before the closing "-->" of the HTML comment
// banner that follows the optional frontmatter. It reports false when the
// content has no such banner.
func injectIntoBanner(content, block string) (string, bool) {
	offset, open := frontmatterEnd(content)
	if open && offset == 0 {
		return content, false
	}
	rest := content[offset:]
	if !strings.HasPrefix(strings.TrimLeft(rest, "\r\n"), "<!--") {
		return content, false
	}
	idx := strings.Index(rest, "\n-->")
	if idx < 0 {
		return content, false
	}
	at := offset + idx
	eol := "\n"
	if at > 0 && content[at-1] == '\r' {
		at--
		eol = "\r\n"
	}
	return content[:at] + eol + block + content[at:], true
}

// computeSourceHash returns a stable blake3 hash over the inputs that determine
// generated output for the active profile. It includes:
//   - GeneratorSchemaVersion (so renderer changes invalidate)
//   - Resolved config metadata that affects rendering
//   - Resolved MCP servers
//   - Every ContentFile in root + domains, with name/path/content/metadata
//
// Determinism requires sorted iteration everywhere — Go maps must be visited
// in lexical key order, ContentFile slices must arrive in a stable order (the
// scanner relies on os.ReadDir's lexical-by-filename ordering; include-merged
// trees are ordered by the include resolver), and metadata serialization uses
// encoding/json which sorts map keys. Paths are normalized by hashPath so the
// checkout root never enters the hash.
func computeSourceHash(cfg *config.Config, content *config.ContentTree) string {
	var b strings.Builder
	b.WriteString("schema=" + templates.GeneratorSchemaVersion + "\n")
	b.WriteString("name=" + cfg.Name + "\n")
	b.WriteString("description=" + cfg.Description + "\n")
	b.WriteString("version=" + cfg.Version + "\n")
	if cfg.Run != nil && cfg.Run.Scope != nil {
		b.WriteString("scope=" + cfg.Run.Scope.Path + "\n")
	}
	b.WriteString("header_style=" + cfg.GetHeaderStyle() + "\n")
	_, _ = fmt.Fprintf(&b, "header_timestamp=%t\n", cfg.ShowHeaderTimestamp())

	// Effective rules mode per enabled preset, sorted, so switching modes
	// rewrites the affected files.
	modes := make([]string, 0, len(cfg.Presets))
	for _, preset := range cfg.Presets {
		name := preset.Name
		if name == "" {
			name = preset.BuiltIn
		}
		modes = append(modes, name+"="+cfg.RulesModeFor(name))
	}
	sort.Strings(modes)
	for _, mode := range modes {
		b.WriteString("rules_mode:" + mode + "\n")
	}

	// MCP servers — sorted by name
	mcpNames := make([]string, 0, len(cfg.MCPServers))
	for name := range cfg.MCPServers {
		mcpNames = append(mcpNames, name)
	}
	sort.Strings(mcpNames)
	for _, name := range mcpNames {
		serverJSON, err := json.Marshal(mcpServerForSourceHash(cfg.MCPServers[name], cfg.BaseDir))
		if err != nil {
			serverJSON = []byte("<marshal-error>")
		}
		b.WriteString("mcp:" + name + "=" + string(serverJSON) + "\n")
	}

	if cfg.HasSelfServer() {
		entryJSON, err := json.Marshal(cfg.SelfMCPServerEntry(schema.Version))
		if err != nil {
			entryJSON = []byte("<marshal-error>")
		}
		b.WriteString("mcp-self=" + string(entryJSON) + "\n")
	}

	// Plugins — sorted by name to match output rendering order
	plugins := append([]config.PluginConfig(nil), cfg.Plugins...)
	sort.SliceStable(plugins, func(i, j int) bool { return plugins[i].Name < plugins[j].Name })
	for _, p := range plugins {
		pluginJSON, err := json.Marshal(p)
		if err != nil {
			pluginJSON = []byte("<marshal-error>")
		}
		b.WriteString("plugin=" + string(pluginJSON) + "\n")
	}

	writeSourceContent(&b, cfg, content)
	return templates.HashContent(b.String())
}

// writeSourceContent writes every content category of the tree into a source
// hash input.
func writeSourceContent(b *strings.Builder, cfg *config.Config, content *config.ContentTree) {
	// Root content categories — slices already sorted by Name in the scanner
	writeContentFiles(b, "root.rules", content.Rules, cfg)
	writeContentFiles(b, "root.context", content.Context, cfg)
	writeContentFiles(b, "root.skills", content.Skills, cfg)
	writeContentFiles(b, "root.agents", content.Agents, cfg)
	writeContentFiles(b, "root.commands", content.Commands, cfg)

	// Domain content — domains visited in sorted name order
	domainNames := make([]string, 0, len(content.Domains))
	for name := range content.Domains {
		domainNames = append(domainNames, name)
	}
	sort.Strings(domainNames)
	for _, name := range domainNames {
		domain := content.Domains[name]
		_, _ = fmt.Fprintf(b, "domain:%s,builtin=%t,from_include=%t\n", name, domain.Builtin, domain.FromInclude)
		writeContentFiles(b, "domain."+name+".rules", domain.Rules, cfg)
		writeContentFiles(b, "domain."+name+".context", domain.Context, cfg)
		writeContentFiles(b, "domain."+name+".skills", domain.Skills, cfg)
		writeContentFiles(b, "domain."+name+".agents", domain.Agents, cfg)
		writeContentFiles(b, "domain."+name+".commands", domain.Commands, cfg)
	}
}

// computeSharedSourceHash is the Source-Hash of the shared outputs (AGENTS.md,
// .agents/skills). It covers the content and the settings that shape these files
// and deliberately not the preset list, per-preset rules modes, MCP servers or
// plugins: the files are written once for every preset that reads them, so adding
// or removing a preset must not change their provenance line unless it changes
// the file. Two inputs of that kind are included: what AGENTS.md inlines for
// presets without a rules folder, and, when some rule or context item has
// frontmatter targets, its owners (the configured presets relying on it, whose
// names and root files those targets select items by). Without targets the
// owners cannot change the file, so they stay out and the line stays stable.
func computeSharedSourceHash(cfg *config.Config, content *config.ContentTree, inlining config.AgentsMDInlining,
	owners []string,
) string {
	var b strings.Builder
	b.WriteString("schema=" + templates.GeneratorSchemaVersion + "\n")
	b.WriteString("shared=agents_md\n")
	_, _ = fmt.Fprintf(&b, "inline_scoped=%t,inline_auto_manual=%t\n", inlining.Scoped, inlining.AutoManual)
	if hasTargetedRulesOrContext(content) {
		b.WriteString("agents_md_owners=" + strings.Join(slices.Sorted(slices.Values(owners)), ",") + "\n")
	}
	b.WriteString("name=" + cfg.Name + "\n")
	b.WriteString("description=" + cfg.Description + "\n")
	b.WriteString("version=" + cfg.Version + "\n")
	if cfg.Run != nil && cfg.Run.Scope != nil {
		b.WriteString("scope=" + cfg.Run.Scope.Path + "\n")
	}
	b.WriteString("header_style=" + cfg.GetHeaderStyle() + "\n")
	_, _ = fmt.Fprintf(&b, "header_timestamp=%t\n", cfg.ShowHeaderTimestamp())
	_, _ = fmt.Fprintf(&b, "compact=%t\n", cfg.IsCompact())
	writeSourceContent(&b, cfg, content)
	return templates.HashContent(b.String())
}

// hasTargetedRulesOrContext reports whether any rule or context item restricts
// itself with frontmatter targets.
func hasTargetedRulesOrContext(content *config.ContentTree) bool {
	targeted := func(files []config.ContentFile) bool {
		return slices.ContainsFunc(files, func(f config.ContentFile) bool {
			return f.Metadata != nil && len(f.Metadata.Targets) > 0
		})
	}
	if targeted(content.Rules) || targeted(content.Context) {
		return true
	}
	for _, domain := range content.Domains {
		if targeted(domain.Rules) || targeted(domain.Context) {
			return true
		}
	}
	return false
}

// builtinPathScheme prefixes the synthetic paths used for embedded builtin
// content, which are already location-independent.
const builtinPathScheme = "builtin://"

// hashPath returns a location-independent identifier for a content file so the
// source hash stays stable across checkout roots, machines, and operating
// systems. ContentFile.Path is absolute for everything the scanner reads from
// disk, so hashing it raw made the checkout root part of the hash: the same
// tree generated from two directories produced different Source-Hash values,
// forcing a rewrite and a fresh timestamp on every relocated checkout (#166).
//
// Out-of-tree content — git includes cached under the user's home directory,
// installed skills, the temp symlink used for bare include layouts — collapses
// to the last two segments. That is exactly what rendering consumes: presets
// derive the skill id from filepath.Base(filepath.Dir(path)) via
// extractSkillID, so the directory name still busts the hash while the
// machine-specific prefix never enters it.
func hashPath(path string, cfg *config.Config) string {
	if path == "" || strings.HasPrefix(path, builtinPathScheme) {
		return path
	}

	for _, root := range []string{cfg.ConfigDir, cfg.BaseDir} {
		if root == "" {
			continue
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		return filepath.ToSlash(rel)
	}

	return filepath.ToSlash(filepath.Join(filepath.Base(filepath.Dir(path)), filepath.Base(path)))
}

// writeContentFiles serializes a ContentFile slice into the hash builder.
// Slices reach this function in the scanner's os.ReadDir order, which is
// lexical by filename; include-merged trees are ordered by the include
// resolver instead.
func writeContentFiles(b *strings.Builder, label string, files []config.ContentFile, cfg *config.Config) {
	for _, f := range files {
		b.WriteString(label + ":" + f.Name + "|path=" + hashPath(f.Path, cfg) + "|content=" + f.Content)
		if f.Metadata != nil {
			metaJSON, err := json.Marshal(f.Metadata)
			if err != nil {
				metaJSON = []byte("<marshal-error>")
			}
			b.WriteString("|meta=" + string(metaJSON))
		}
		// Include skill resources in the source hash so changes to
		// references/, scripts/, or assets/ bust the SKILL.md skip-cache
		// (the resource index is derived from these and would otherwise
		// stay stale).
		//
		// Length-prefix the content so resource bytes can never spoof
		// another resource record's delimiter — without this, a reference
		// whose body happened to contain `|res=ref:other:...` could be
		// indistinguishable from two separate resources.
		for _, r := range f.Resources {
			fmt.Fprintf(b, "|res=%s:%s:len=%d:", r.Kind, r.RelPath, len(r.Content))
			b.Write(r.Content)
		}
		b.WriteByte('\n')
	}
}

func (g *Generator) manifestDir() string {
	if g.config.ConfigDir == "" {
		return filepath.Join(g.config.BaseDir, ".ai-rulez")
	}
	return g.config.ConfigDir
}

func (g *Generator) manifestPath() string {
	return filepath.Join(g.manifestDir(), generatedManifestName)
}

func (g *Generator) localManifestPath() string {
	return filepath.Join(g.manifestDir(), generatedLocalManifestName)
}

// beginRun forgets the manifests the previous run of this Generator read. The
// warnings it issued are kept: clean plans (and so warns) once to show the plan
// and again to apply it, and the user should see each only once.
func (g *Generator) beginRun() {
	g.manifests = nil
}

// readManifest reads a manifest at most once per run, so a corrupt one is
// reported once and every consumer sees the same content.
func (g *Generator) readManifest(path string) generatedManifest {
	if manifest, ok := g.manifests[path]; ok {
		return manifest
	}
	manifest := readManifestFile(path)
	if g.manifests == nil {
		g.manifests = map[string]generatedManifest{}
	}
	g.manifests[path] = manifest
	return manifest
}

func readManifestFile(path string) generatedManifest {
	data, err := os.ReadFile(path)
	if err != nil {
		return generatedManifest{}
	}
	var manifest generatedManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		logger.Warn("Ignoring invalid generated manifest", "path", path, "error", err)
		return generatedManifest{}
	}
	return manifest
}

// previousManifestFiles returns every path the previous run generated: the
// committed manifest plus the machine-local one. Committed entries that name a
// ".local." file are ignored, because only the gitignored local manifest may
// authorize deleting machine-local outputs (older versions recorded them in the
// committed manifest, where a teammate's run would delete them).
func (g *Generator) previousManifestFiles() []string {
	var files []string
	for _, f := range g.readManifest(g.manifestPath()).Files {
		if !strings.Contains(filepath.Base(filepath.FromSlash(f)), ".local.") {
			files = append(files, f)
		}
	}
	if g.localSkipped {
		// A run that deliberately ignores local inputs must not clean up the
		// outputs of the inputs it ignored.
		return files
	}
	return append(files, g.readManifest(g.localManifestPath()).Files...)
}

// writeGeneratedManifest records the generated files so the next run can delete
// the ones that dropped out. Partially owned outputs are deliberately left out:
// the manifest exists only to drive deletion, and deleting a file ai-rulez
// merely contributed a key to would take the hand-authored remainder with it.
// Machine-local outputs go to the separate, gitignored local manifest, which is
// removed when no local output remains.
func (g *Generator) writeGeneratedManifest(outputs []config.OutputFile) error {
	var shared, local []string
	committedMerged, localMerged := g.splitMergedClaims(outputs)
	for _, output := range outputs {
		if output.IsDir || output.PartiallyOwned {
			continue
		}
		rel := filepath.ToSlash(g.convertToRelativePath(g.absOutputPath(output.Path)))
		switch {
		case g.skippedPaths[rel]:
		case output.LocalOnly:
			local = append(local, rel)
		default:
			shared = append(shared, rel)
		}
	}
	if g.plan != nil {
		// The committed manifest describes the shared baseline, not this machine.
		shared = g.plan.sharedManifestFiles(g.skippedPaths)
	}
	defer func() { g.manifests = nil }()
	if err := writeManifestFile(g.manifestPath(), shared, committedMerged); err != nil {
		return err
	}
	if g.localSkipped {
		// Local files were deliberately not loaded: their manifest is not ours to
		// drop, and its record of merged documents stays what it was.
		return nil
	}
	if len(local) == 0 && len(localMerged) == 0 {
		if err := os.Remove(g.localManifestPath()); err != nil && !os.IsNotExist(err) {
			return oops.With("path", g.localManifestPath()).Wrapf(err, "remove local manifest")
		}
		return nil
	}
	return writeManifestFile(g.localManifestPath(), local, localMerged)
}

func writeManifestFile(path string, files []string, merged map[string][]jsonmerge.Claim) error {
	sort.Strings(files)
	files = slices.Compact(files)
	if files == nil {
		files = []string{}
	}
	data, err := json.MarshalIndent(generatedManifest{Version: "1", Files: files, Merged: merged}, "", "  ")
	if err != nil {
		return oops.Wrapf(err, "marshal generated manifest")
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return oops.With("dir", filepath.Dir(path)).Wrapf(err, "create manifest directory")
	}
	return os.WriteFile(path, data, 0o644)
}

func (g *Generator) staleManifestFiles(outputs []config.OutputFile) []string {
	previous := g.previousManifestFiles()
	if len(previous) == 0 {
		return nil
	}

	next := make(map[string]bool)
	// Files only the shared baseline produces are never this machine's to delete.
	if g.plan != nil {
		for _, rel := range g.plan.suppressed {
			next[rel] = true
		}
	}
	for _, output := range outputs {
		if output.IsDir {
			continue
		}
		next[filepath.ToSlash(g.convertToRelativePath(g.absOutputPath(output.Path)))] = true
	}

	// Never delete a merged settings document from a manifest entry. The manifest
	// holds bare paths with no record of what the document contained, and one
	// written by an older ai-rulez lists these files even when the consumer
	// hand-authored them — so the flag on OutputFile cannot be consulted here.
	// Leaving a wholly generated .mcp.json behind is the cost; the alternative
	// deletes a tracked file full of the user's own settings (#185).
	//
	// Both sources are needed: provider sidecar specs cover .claude/settings.json,
	// .mcp.json and .amp/settings.json, while .gemini/settings.json and
	// .agents/settings.json are merged by preset generators that have no spec.
	// Those two are also the ones emitted only when MCP servers are declared, so
	// they are exactly the paths a render omits while the manifest still lists them.
	merged := append(providers.MergedSidecarPaths(), presets.MergedDocumentPaths()...)

	// The gitignored local manifest lists only whole files ai-rulez wrote from
	// machine-local inputs (never a partially owned document), so it may remove
	// a merged document too: a secret-bearing .mcp.json must not outlive the
	// overlay that produced it.
	local := g.localManifestSet()

	var stale []string
	for _, relPath := range previous {
		if next[relPath] || (isMergedDocumentPath(merged, relPath) && !local[relPath]) {
			continue
		}
		absPath := filepath.Join(g.config.BaseDir, filepath.FromSlash(relPath))
		if !isUnderBaseDir(g.config.BaseDir, absPath) {
			logger.Warn("Skipping generated manifest path outside project", "path", relPath)
			continue
		}
		if _, err := os.Stat(absPath); err != nil {
			continue
		}
		// A rules folder is shared with hand-written rules: delete only a file that
		// still looks generated, even when a manifest lists it.
		if config.InRulesDir(relPath) && !looksGenerated(absPath) {
			logger.Debug("Keeping manifest-listed rule file without a generated marker", "path", relPath)
			continue
		}
		stale = append(stale, absPath)
	}
	sort.Strings(stale)
	return stale
}

// localManifestSet is the set of paths the machine-local manifest authorizes
// deleting; empty when the run deliberately ignores local inputs.
func (g *Generator) localManifestSet() map[string]bool {
	set := map[string]bool{}
	if g.localSkipped {
		return set
	}
	for _, f := range g.readManifest(g.localManifestPath()).Files {
		set[f] = true
	}
	return set
}

// looksGenerated reports whether the file at absPath carries stored hashes or a
// generated banner.
func looksGenerated(absPath string) bool {
	if contentHash, _ := extractStoredHashes(absPath); contentHash != "" {
		return true
	}
	data, err := os.ReadFile(absPath)
	return err == nil && hasGeneratedBanner(absPath, data)
}

// isMergedDocumentPath reports whether relPath names one of the merged settings
// documents. The registries hold paths relative to a config's own base dir
// (".mcp.json"), while a manifest entry is relative to the root config, so a
// scope's document appears as "packages/api/.mcp.json". Matching on the tail --
// the same rule presets.isRegisteredMergedDocument applies -- is what keeps a
// scoped document out of the stale set; an exact match protected only the root
// one and deleted every scope's, which is the #185 data loss this guard exists
// to prevent.
func isMergedDocumentPath(merged []string, relPath string) bool {
	slashed := filepath.ToSlash(relPath)
	for _, candidate := range merged {
		if slashed == candidate || strings.HasSuffix(slashed, "/"+candidate) {
			return true
		}
	}
	return false
}

func isUnderBaseDir(baseDir, path string) bool {
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return false
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absBase, absPath)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")
}

func (g *Generator) removeStaleManifestFiles(files []string) {
	for _, file := range files {
		g.removeStaleFile(file)
	}
}

// removeStaleFile removes a single stale file.
func (g *Generator) removeStaleFile(filePath string) {
	if err := os.Remove(filePath); err != nil {
		logger.Warn("Failed to remove stale file", "path", filePath, "error", err)
	} else {
		logger.Debug("Removed stale file", "path", filePath)
	}
}

// hasLocalOutputs reports whether any output is machine-local.
func (g *Generator) hasLocalOutputs(outputs []config.OutputFile) bool {
	for _, output := range outputs {
		if output.LocalOnly {
			return true
		}
	}
	return false
}

// guardedOutputs lists the project-relative files that must never be committable:
// machine-local outputs and MCP configs holding resolved secrets.
func (g *Generator) guardedOutputs(outputs []config.OutputFile) []string {
	var rels []string
	for _, output := range outputs {
		if output.IsDir {
			continue
		}
		rel := filepath.ToSlash(g.convertToRelativePath(g.absOutputPath(output.Path)))
		if output.LocalOnly || (output.Sensitive && isMCPConfigOutput(rel)) {
			rels = append(rels, rel)
		}
	}
	sort.Strings(rels)
	return rels
}

func (g *Generator) hasGuardedSecretOutputs(outputs []config.OutputFile) bool {
	for _, output := range outputs {
		if output.IsDir || !output.Sensitive {
			continue
		}
		if isMCPConfigOutput(filepath.ToSlash(g.convertToRelativePath(g.absOutputPath(output.Path)))) {
			return true
		}
	}
	return false
}

// verifyGuardedOutputsIgnored asks git (or, outside a repository, the project's
// .gitignore) about the real paths of the guarded outputs, after the ignore
// entries are written. The entries were chosen from probes and patterns; a user
// rule that un-ignores a path, a rule that only looked like it covered a pattern,
// or an ignore file git does not read would otherwise leave local or secret
// content committable without any message. It names paths only.
func (g *Generator) verifyGuardedOutputsIgnored(outputs []config.OutputFile) error {
	rels := append(g.guardedOutputs(outputs), g.localInputRels()...)
	if len(rels) == 0 {
		return nil
	}
	ignored := g.ignoredSet(rels, nil)
	var open []string
	for _, rel := range rels {
		if !ignored[rel] {
			open = append(open, rel)
		}
	}
	if len(open) == 0 {
		return nil
	}
	return oops.
		With("paths", open).
		Hint("Machine-local and secret-bearing files must be git-ignored. A .gitignore rule probably un-ignores them "+
			"(for example \"!.claude/skills/**\"): narrow that rule, or run with --no-local to generate the shared view").
		Errorf("generated machine-local or secret outputs are not git-ignored: %s", strings.Join(open, ", "))
}

// localInputRels lists the project-relative machine-local inputs that exist: the
// overlay files and the local/ content tree. They are guarded like outputs, so a
// rule that un-ignores them fails the run instead of exposing their content.
func (g *Generator) localInputRels() []string {
	dir := g.config.ConfigDir
	if dir == "" {
		return nil
	}
	var rels []string
	if matches, err := filepath.Glob(filepath.Join(dir, "config.local.*")); err == nil {
		for _, m := range matches {
			if rel := filepath.ToSlash(g.convertToRelativePath(m)); rel != "" {
				rels = append(rels, rel)
			}
		}
	}
	if info, err := os.Stat(filepath.Join(dir, localSourceDirName)); err == nil && info.IsDir() {
		if rel := filepath.ToSlash(g.convertToRelativePath(filepath.Join(dir, localSourceDirName))); rel != "" {
			rels = append(rels, rel)
		}
	}
	sort.Strings(rels)
	return rels
}

// ignoreLocalInputs makes sure the overlay file, its temp files, the local/
// content tree and the local manifest are git-ignored. It runs before any check
// that can refuse the run.
func (g *Generator) ignoreLocalInputs() error {
	patterns := g.localInputPatterns()
	if len(patterns) == 0 {
		return nil
	}
	// The overlay's lock and temp files come and go: ignore them before they exist.
	patterns = append(patterns, g.configDirName()+"/.config.local.*")
	if err := gitignore.EnsureEntries(g.config.BaseDir, patterns); err != nil {
		return oops.Wrapf(err, "gitignore the machine-local inputs")
	}
	return nil
}

// localInputPatterns lists the ignore patterns of the machine-local inputs: the
// local/ content tree, the local manifest, the overlay and whichever of its lock
// and temp files exist. It is empty when the project has no local inputs.
func (g *Generator) localInputPatterns() []string {
	if !g.hasLocalGitignoreTargets() {
		return nil
	}
	patterns := []string{
		g.configDirName() + "/" + localSourceDirName + "/",
		g.configDirName() + "/config.local.*",
	}
	if rel := filepath.ToSlash(g.convertToRelativePath(g.localManifestPath())); rel != "" {
		patterns = append(patterns, rel)
	}
	return append(patterns, g.localGitignorePatternsOnDisk()...)
}

// hasLocalGitignoreTargets reports whether machine-local content exists and so
// requires unconditional gitignore entries (the ".local" outputs plus the
// .ai-rulez/local/ source subtree). The local manifest counts: besides local
// outputs it records what ai-rulez merged into hand-authored documents.
func (g *Generator) hasLocalGitignoreTargets() bool {
	return g.config.HasLocalInputs() || len(g.localGitignorePatternsOnDisk()) > 0 || pathIsFile(g.localManifestPath())
}

// localGitignorePatternsOnDisk lists the machine-local ignore patterns for the
// overlay file, its lock/temp files and the local/ content tree that exist on
// disk. It checks the filesystem rather than the loaded config, so a run that
// skipped local inputs (plugin mode, --no-local) still keeps them ignored.
func (g *Generator) localGitignorePatternsOnDisk() []string {
	dir := g.config.ConfigDir
	if dir == "" {
		return nil
	}
	prefix := g.configDirName() + "/"
	var patterns []string
	for _, p := range []string{"config.local.*", ".config.local.*"} {
		if matches, err := filepath.Glob(filepath.Join(dir, p)); err == nil && len(matches) > 0 {
			patterns = append(patterns, prefix+p)
		}
	}
	if info, err := os.Stat(filepath.Join(dir, localSourceDirName)); err == nil && info.IsDir() {
		patterns = append(patterns, prefix+localSourceDirName+"/")
	}
	return patterns
}

// collectGitignorePaths collects unique patterns to add to .gitignore.
//
// Committed output patterns are only added when config gitignore management is
// enabled. Machine-local ".local" outputs and the .ai-rulez/local/ source
// subtree are added UNCONDITIONALLY (even when gitignore is disabled) because
// committing them would leak machine-local configuration.
func (g *Generator) collectGitignorePaths(outputs []config.OutputFile) map[string]bool {
	paths := make(map[string]bool)
	includeCommitted := g.config.ShouldUpdateGitignore()
	for _, output := range outputs {
		relPath := filepath.ToSlash(g.convertToRelativePath(g.absOutputPath(output.Path)))
		if output.LocalOnly {
			if pattern := g.localOutputPattern(relPath); pattern != "" {
				paths[pattern] = true
			}
			continue
		}
		// A hand-written file the overwrite guard skipped is the user's, not ours.
		if g.skippedPaths[relPath] {
			continue
		}
		// A partially owned settings document is hand-authored and tracked apart
		// from the one key ai-rulez writes into it; telling git to ignore it
		// would hide the user's own file (#185).
		if output.PartiallyOwned {
			continue
		}
		if !includeCommitted {
			continue
		}
		if g.shouldSkipPath(relPath) {
			continue
		}
		if pattern := gitignorePatternForOutput(relPath, output.IsDir); pattern != "" {
			paths[pattern] = true
		}
	}

	// The .ai-rulez/local/ source subtree holds machine-local override content
	// and must never be committed. Ignore it unconditionally, bypassing the
	// config-dir skip that normally protects .ai-rulez/.
	for _, p := range g.localInputPatterns() {
		paths[p] = true
	}

	// A run that skipped the local inputs on purpose (--no-local) writes no
	// local outputs, but the ones an earlier run left are still there and must
	// stay ignored.
	if g.localSkipped {
		for _, rel := range g.readManifest(g.localManifestPath()).Files {
			if pattern := g.skippedLocalPattern(rel); pattern != "" {
				paths[pattern] = true
			}
		}
	}

	// The generated manifest sits inside the config dir and is rewritten on
	// every `generate`; include it explicitly so the managed fence covers it.
	// Only relevant when committed outputs are managed.
	if includeCommitted {
		if manifestRel := filepath.ToSlash(g.convertToRelativePath(g.manifestPath())); manifestRel != "" {
			paths[manifestRel] = true
		}
	}

	return paths
}

// skippedLocalPattern is the managed-.gitignore pattern for a machine-local file
// an earlier run recorded, for a run that did not render local inputs. Files
// named after local content are excluded per clone and stay in .git/info/exclude,
// which such a run leaves alone; only outside a repository do they need the block.
func (g *Generator) skippedLocalPattern(rel string) string {
	if stableLocalName(rel) || gitutil.InfoExcludePath(g.config.BaseDir) == "" {
		return localGitignorePattern(rel)
	}
	return ""
}

// localGitignorePattern maps a machine-local output to its ignore pattern. Local
// rule files share a rules folder with committed rules, so they get the stable
// "<rulesdir>/*.local.*" pattern instead of one entry per file; that keeps the
// block identical for teammates and covers rules added later.
func localGitignorePattern(relPath string) string {
	if config.InRulesDir(relPath) {
		return relPath[:strings.LastIndex(relPath, "/")] + "/*.local.*"
	}
	return relPath
}

// gitignorePatternForOutput maps one generated output path to the pattern that
// belongs in the managed .gitignore block, or "" when the path must not be ignored
// at all. Each family of generated output gets its own resolver so none of them
// has to be read through the others.
func gitignorePatternForOutput(relPath string, isDir bool) string {
	relPath = strings.TrimPrefix(filepath.ToSlash(relPath), "./")
	if relPath == ".github" || strings.HasSuffix(relPath, "/.github") {
		return ""
	}
	// Rules folders are shared with hand-written rules, so ignore generated
	// files one by one rather than the folder.
	if rest, ok := config.RulesDirRemainder(relPath); ok {
		if rest == "" {
			return ""
		}
		if !isDir {
			return relPath
		}
	}
	if pattern, matched := assistantDirGitignorePattern(relPath, isDir); matched {
		return pattern
	}
	for _, file := range generatedRootFiles {
		if relPath == file {
			return file
		}
	}
	if pattern, matched := githubGitignorePattern(relPath); matched {
		return pattern
	}
	if isDir {
		return strings.TrimSuffix(relPath, "/") + "/"
	}
	return relPath
}

// assistantDirGitignorePattern resolves relPath against the assistant directories
// ai-rulez shares with the user (.claude/, .gemini/, ...), whether at the repo root
// or nested under a subproject. matched is false when relPath is not inside one of
// them; a matched but empty pattern means the path is the shared directory itself,
// which must stay un-ignored because the user tracks their own files in it (#184).
func assistantDirGitignorePattern(relPath string, isDir bool) (pattern string, matched bool) {
	for _, dir := range generatedAssistantDirs {
		trimmedDir := strings.TrimSuffix(dir, "/")
		if relPath == trimmedDir {
			return "", true
		}
		if strings.HasPrefix(relPath, dir) {
			return ownedAssistantSubPath("", dir, strings.TrimPrefix(relPath, dir), isDir), true
		}
		idx := strings.Index(relPath, "/"+trimmedDir)
		if idx < 0 {
			continue
		}
		remainder := relPath[idx+1+len(trimmedDir):]
		if remainder == "" {
			return "", true
		}
		// Anything else is a longer segment that merely starts with the directory
		// name (".clauderc"), so keep looking.
		if strings.HasPrefix(remainder, "/") {
			return ownedAssistantSubPath(relPath[:idx+1], dir, remainder[1:], isDir), true
		}
	}
	return "", false
}

// githubGitignorePattern resolves relPath against the .github/ content ai-rulez
// generates, at the repo root or nested under a subproject.
func githubGitignorePattern(relPath string) (pattern string, matched bool) {
	for _, candidate := range generatedGithubPatterns {
		if relPath == strings.TrimSuffix(candidate, "/") || strings.HasPrefix(relPath, candidate) {
			return candidate, true
		}
		if idx := strings.Index(relPath, "/"+candidate); idx >= 0 {
			return relPath[:idx+1] + candidate, true
		}
	}
	return "", false
}

// ownedAssistantSubPath narrows a gitignore pattern to the content ai-rulez
// actually writes inside an assistant directory.
//
// An assistant directory is shared territory: ai-rulez writes .claude/skills/
// and .claude/agents/, while the user hand-authors and tracks
// .claude/settings.json beside them. Ignoring the directory root makes git skip
// those tracked files with no diagnostic (issue #184), so the pattern names the
// first owned segment instead — .claude/skills/ rather than .claude/.
//
// remainder is the path relative to the assistant directory. A remainder with a
// separator identifies an owned subdirectory; a single segment is either a
// directory marker (isDir) or a file ai-rulez writes into the root, which is
// ignored by name so narrowing never stops covering generated content.
func ownedAssistantSubPath(nestedPrefix, assistantDir, remainder string, isDir bool) string {
	if remainder == "" {
		return ""
	}
	if idx := strings.Index(remainder, "/"); idx >= 0 {
		return nestedPrefix + assistantDir + remainder[:idx+1]
	}
	if isDir {
		return nestedPrefix + assistantDir + remainder + "/"
	}

	return nestedPrefix + assistantDir + remainder
}

var generatedRootFiles = [...]string{
	"AGENTS.md",
	"CLAUDE.md",
	"GEMINI.md",
	".mcp.json",
}

var generatedAssistantDirs = [...]string{
	".agents/",
	".claude/",
	".codex/",
	".cursor/",
	".gemini/",
	".continue/",
	".cline/",
	".clinerules/",
	".windsurf/",
	".junie/",
	".opencode/",
	".amp/",
}

var generatedGithubPatterns = [...]string{
	".github/copilot-instructions.md",
	".github/agents/",
	".github/commands/",
	".github/skills/",
}

// convertToRelativePath converts an absolute path to relative, or returns the original path
func (g *Generator) convertToRelativePath(path string) string {
	if !filepath.IsAbs(path) {
		return path
	}
	relPath, err := filepath.Rel(g.config.BaseDir, path)
	if err != nil {
		return filepath.Base(path)
	}
	return relPath
}

// shouldSkipPath checks if a path should be skipped for .gitignore
func (g *Generator) shouldSkipPath(relPath string) bool {
	configDir := g.configDirName()
	return relPath == configDir ||
		hasPrefix(relPath, configDir+"/") ||
		hasPrefix(relPath, configDir+"\\")
}

func (g *Generator) configDirName() string {
	if g.config.ConfigDirName != "" {
		return g.config.ConfigDirName
	}
	return ".ai-rulez"
}

// updateGitignore updates .gitignore with generated file paths using a fenced block
func (g *Generator) updateGitignore(outputs []config.OutputFile) error {
	gitignorePath := filepath.Join(g.config.BaseDir, ".gitignore")

	// Patterns git does not already cover: not ignored by a user rule, and not
	// deliberately un-ignored by one.
	paths, overridden := g.neededGitignorePatterns(outputs)
	for _, o := range overridden {
		logger.Warn("A .gitignore rule un-ignores a machine-local or secret output; ai-rulez will not re-ignore it",
			"path", o.Pattern, "rule", o.Rule, "source", o.Source)
	}

	// Read existing .gitignore content
	existingData, err := gitutil.ReadIgnoreFileOrEmpty(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return oops.
			With("path", gitignorePath).
			Wrapf(err, "read .gitignore")
	}
	existingContent := string(existingData)

	// Git does not read a symlinked .gitignore, and writing through the link would
	// change a file that lives elsewhere: keep every entry in .git/info/exclude.
	// The link's patterns protect nothing, so none may be dropped as user-covered.
	if gitignore.IsSymlink(g.config.BaseDir) {
		return gitignore.ReplaceViaExclude(g.config.BaseDir, paths) //nolint:wrapcheck // already contextual
	}

	sortedPaths := dropUserPatterns(paths, existingContent)

	if len(sortedPaths) == 0 {
		logger.Debug("No paths to add to .gitignore")
		// Nothing is left to add: drop a block from an earlier run rather than
		// leave an empty fence behind.
		if !contains(existingContent, gitignore.BeginMarker) && !contains(existingContent, gitignore.OldHeader) {
			return nil
		}
		return dropManagedBlock(gitignorePath, existingContent)
	}

	// Build the fenced block
	var fencedBlock strings.Builder
	fencedBlock.WriteString(gitignore.BeginMarker + "\n")
	for _, p := range sortedPaths {
		fencedBlock.WriteString(p + "\n")
	}
	fencedBlock.WriteString(gitignore.EndMarker + "\n")

	var newContent string

	switch {
	case contains(existingContent, gitignore.BeginMarker):
		newContent = gitignore.ReplaceFencedBlock(existingContent, fencedBlock.String())
	case contains(existingContent, gitignore.OldHeader):
		newContent = gitignore.ReplaceOldHeaderBlock(existingContent, fencedBlock.String())
	case len(existingData) == 0:
		newContent = fencedBlock.String()
	default:
		newContent = existingContent
		if !hasSuffix(newContent, "\n") {
			newContent += "\n"
		}
		newContent += "\n" + fencedBlock.String()
	}

	if err := os.WriteFile(gitignorePath, []byte(newContent), 0o644); err != nil { //nolint:gosec // path from config, not user input
		return oops.
			With("path", gitignorePath).
			Wrapf(err, "write .gitignore")
	}

	logger.Debug("Updated .gitignore", "entries", len(sortedPaths))
	return nil
}

func (g *Generator) ensureSecretOutputsIgnored(outputs []config.OutputFile) error {
	secretKeys := g.secretMCPEnvKeys()
	if len(secretKeys) == 0 {
		return nil
	}

	var pending []string
	if g.config.ShouldUpdateGitignore() {
		pending = g.pendingIgnorePatterns(outputs)
	}

	var candidates []string
	var unsafe []string
	// A partially owned document is the consumer's file: ai-rulez merges one key
	// into it and cannot gitignore it on their behalf, so --gitignore is not the
	// remedy and the hint must not suggest it.
	shared := false
	for _, output := range outputs {
		if output.IsDir {
			continue
		}
		relPath := filepath.ToSlash(g.convertToRelativePath(g.absOutputPath(output.Path)))
		if !isMCPConfigOutput(relPath) {
			continue
		}
		candidates = append(candidates, relPath)
	}
	ignored := g.ignoredSet(candidates, pending)
	for _, relPath := range candidates {
		if !ignored[relPath] {
			unsafe = append(unsafe, relPath)
		}
	}
	for _, output := range outputs {
		if output.PartiallyOwned && !output.IsDir {
			relPath := filepath.ToSlash(g.convertToRelativePath(g.absOutputPath(output.Path)))
			shared = shared || (isMCPConfigOutput(relPath) && !ignored[relPath])
		}
	}
	if len(unsafe) == 0 {
		return nil
	}
	sort.Strings(unsafe)
	return oops.
		With("paths", unsafe).
		With("env_keys", secretKeys).
		Hint(g.secretIgnoreHint(unsafe, shared)).
		Errorf("generated MCP config contains secrets but is not gitignored: %s", strings.Join(unsafe, ", "))
}

// secretIgnoreHint tells how to fix MCP configs that hold secrets and are not
// ignored. handAuthored marks a partially owned document, which ai-rulez cannot
// gitignore on the consumer's behalf, so --gitignore is not the remedy there.
func (g *Generator) secretIgnoreHint(unsafe []string, handAuthored bool) string {
	switch {
	case handAuthored:
		return "These files hold hand-authored settings alongside the generated mcpServers key, so ai-rulez will " +
			"not gitignore them for you. Either ignore them yourself, or move the secret-bearing server into a " +
			"config whose MCP output is not shared."
	case g.sharedWithTeam(unsafe):
		return "These MCP config files are shared with your team (committed) but would carry secrets. Add them to " +
			".gitignore, or move the secret-bearing server out of config.local.* and out of the shared config."
	}
	return "Enable gitignore generation with --gitignore or add these generated MCP config paths to .gitignore"
}

// sharedWithTeam reports whether any of the project-relative paths is a file the
// shared baseline also produces, that is, one teammates generate and commit.
func (g *Generator) sharedWithTeam(rels []string) bool {
	if g.plan == nil {
		return false
	}
	for _, rel := range rels {
		if slices.Contains(g.plan.baselineFiles, rel) || slices.Contains(g.plan.drift, rel) {
			return true
		}
	}
	return false
}

// sensitiveFileMode is the mode of generated files that carry MCP secrets.
const sensitiveFileMode os.FileMode = 0o600

// minSecretMatchLen is the shortest secret value matched against output content.
// A shorter value ("1", "dev") would match unrelated files and tighten their
// permissions for no benefit; secret key names are handled separately and always
// count.
const minSecretMatchLen = 8

// secretMCPValues lists the resolved values of MCP env entries and headers that
// are secret (placeholder-sourced or sensitively named) and long enough to match
// output content reliably.
func (g *Generator) secretMCPValues() []string {
	var values []string
	for _, server := range g.config.MCPServers {
		if server == nil {
			continue
		}
		for _, key := range server.SecretEnvKeys {
			if v := server.Env[key]; len(v) >= minSecretMatchLen {
				values = append(values, v)
			}
		}
		for _, key := range server.SecretHeaderKeys {
			if v := server.Headers[key]; len(v) >= minSecretMatchLen {
				values = append(values, v)
			}
		}
		for _, v := range literalSecrets(server) {
			if len(v) >= minSecretMatchLen {
				values = append(values, v)
			}
		}
	}
	return values
}

// markSensitiveOutputs flags every output file whose content contains a resolved
// MCP secret value, whichever preset produced it, so the writer keeps it
// owner-only.
func (g *Generator) markSensitiveOutputs(outputs []config.OutputFile) {
	values := g.secretMCPValues()
	names := g.secretMCPNames()
	if len(values) == 0 && len(names) == 0 {
		return
	}
	for i := range outputs {
		o := &outputs[i]
		if o.IsDir {
			continue
		}
		if outputContainsAny(o, values) {
			o.Sensitive = true
			continue
		}
		// An MCP config file naming a secret key is sensitive whatever the value's
		// length, so a short secret cannot loosen it.
		rel := filepath.ToSlash(g.convertToRelativePath(g.absOutputPath(o.Path)))
		o.Sensitive = o.Sensitive || (isMCPConfigOutput(rel) && outputContainsAny(o, names))
	}
}

// outputContainsAny reports whether the output's content holds any of the strings.
func outputContainsAny(o *config.OutputFile, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(o.Content, n) || (o.RawContent != nil && bytes.Contains(o.RawContent, []byte(n))) {
			return true
		}
	}
	return false
}

// secretMCPNames lists the names of secret MCP env entries and headers, plus the
// URL and flag credentials of servers, which an MCP config holds whatever their length.
func (g *Generator) secretMCPNames() []string {
	var names []string
	for _, server := range g.config.MCPServers {
		if server == nil {
			continue
		}
		names = append(names, server.SecretEnvKeys...)
		names = append(names, server.SecretHeaderKeys...)
		names = append(names, literalSecrets(server)...)
	}
	return names
}

func (g *Generator) secretMCPEnvKeys() []string {
	keys := make(map[string]bool)
	for name, server := range g.config.MCPServers {
		if hasLiteralSecrets(server) {
			keys["url or args of "+name] = true
		}
		for _, key := range server.SecretEnvKeys {
			keys[key] = true
		}
		for _, key := range server.SecretHeaderKeys {
			keys["header:"+key] = true
		}
	}
	return sortedMapKeys(keys)
}

func isMCPConfigOutput(relPath string) bool {
	return relPath == ".mcp.json" ||
		relPath == "opencode.json" ||
		strings.HasSuffix(relPath, "/opencode.json") ||
		relPath == ".claude/settings.json" ||
		relPath == ".gemini/settings.json" ||
		relPath == ".agents/settings.json" ||
		relPath == ".xum/mcp.jsonc" ||
		relPath == ".pi/mcp.json" ||
		strings.HasSuffix(relPath, "/.mcp.json") ||
		strings.HasSuffix(relPath, "/.claude/settings.json") ||
		strings.HasSuffix(relPath, "/.gemini/settings.json") ||
		strings.HasSuffix(relPath, "/.agents/settings.json") ||
		strings.HasSuffix(relPath, "/.xum/mcp.jsonc") ||
		strings.HasSuffix(relPath, "/.pi/mcp.json")
}

func gitignorePatterns(content string) []string {
	var patterns []string
	for _, line := range splitLines(content) {
		trimmed := trimSpace(line)
		if trimmed == "" || hasPrefix(trimmed, "#") {
			continue
		}
		patterns = append(patterns, trimmed)
	}
	return patterns
}

// isIgnored checks if a filename matches any gitignore pattern
func isIgnored(filename string, patterns []string) bool {
	for _, pattern := range patterns {
		if matchesPattern(filename, pattern) {
			return true
		}
	}
	return false
}

// matchesPattern checks if a filename matches a gitignore pattern
func matchesPattern(filename, pattern string) bool {
	// Exact match
	if pattern == filename {
		return true
	}

	// Directory pattern
	if hasSuffix(pattern, "/") {
		return matchesDirectory(filename, pattern)
	}

	// Glob pattern
	if contains(pattern, "*") || contains(pattern, "?") {
		if matched, _ := filepath.Match(pattern, filename); matched {
			return true
		}
		if matched, _ := filepath.Match(pattern, filepath.Base(filename)); matched {
			return true
		}
		return false
	}

	// Absolute pattern
	if hasPrefix(pattern, "/") {
		return filename == trimPrefix(pattern, "/")
	}

	// Substring match
	return filename == pattern ||
		hasSuffix(filename, "/"+pattern) ||
		contains(filename, "/"+pattern+"/") ||
		contains(filename, pattern)
}

// matchesDirectory checks if filename matches a directory pattern.
//
// A directory pattern covers the directory itself and everything nested under
// it, so the nesting check has to run whether or not filename is itself a
// directory: a pre-existing ".claude/" already ignores ".claude/skills/", and
// re-listing the subdirectory inside the managed fence would duplicate it.
// Trailing slashes and the leading anchor are notation, not path segments, so
// both sides are normalised before comparing.
func matchesDirectory(filename, pattern string) bool {
	dirPrefix := trimPrefix(trimSuffix(pattern, "/"), "/")
	candidate := trimPrefix(trimSuffix(filename, "/"), "/")

	return candidate == dirPrefix || hasPrefix(candidate, dirPrefix+"/")
}

// String helper functions to avoid importing strings package
func splitLines(s string) []string {
	var lines []string
	start := 0

	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}

	if start < len(s) {
		lines = append(lines, s[start:])
	}

	return lines
}

func trimSpace(s string) string {
	start := 0
	end := len(s)

	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}

	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}

	return s[start:end]
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func trimPrefix(s, prefix string) string {
	if hasPrefix(s, prefix) {
		return s[len(prefix):]
	}
	return s
}

func trimSuffix(s, suffix string) string {
	if hasSuffix(s, suffix) {
		return s[:len(s)-len(suffix)]
	}
	return s
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// dropUserPatterns drops entries the user already has outside the managed fence,
// which avoids duplicating lines like ".cursor/" that pre-existed in the file.
func dropUserPatterns(paths []string, existingContent string) []string {
	outside := gitignore.PatternsOutsideFence(existingContent)
	outsidePatterns := make([]string, 0, len(outside))
	for pattern := range outside {
		outsidePatterns = append(outsidePatterns, pattern)
	}
	var kept []string
	for _, p := range paths {
		if !isIgnored(p, outsidePatterns) {
			kept = append(kept, p)
		}
	}
	return kept
}
