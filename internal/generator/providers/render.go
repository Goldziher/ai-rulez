package providers

import (
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/presets"
	"github.com/Goldziher/ai-rulez/internal/generator/rulefiles"
	"github.com/Goldziher/ai-rulez/internal/generator/targetmatch"
	"github.com/Goldziher/ai-rulez/internal/templates"
	"github.com/samber/oops"
	"gopkg.in/yaml.v3"
)

// Generator is a config.PresetGenerator backed by a declarative ProviderSpec.
// All preset-specific behavior lives in the spec; the renderer is generic.
type Generator struct {
	Spec *ProviderSpec
}

// New wraps a validated spec in a Generator.
func New(spec *ProviderSpec) *Generator {
	return &Generator{Spec: spec}
}

// GetName implements config.PresetGenerator.
func (g *Generator) GetName() string {
	return g.Spec.Name
}

// LocalRootFile implements config.LocalRootProvider. It returns the spec's
// machine-local root file: the ".local" variant of the root file (CLAUDE.md →
// CLAUDE.local.md) unless root.local_file names another path, or "" when the spec
// has no single-file markdown root or the tool has no local file (local_file =
// "none").
func (g *Generator) LocalRootFile() string {
	if g.Spec.Root == nil || g.Spec.Root.File == "" || g.Spec.Root.LocalFile == LocalFileNone {
		return ""
	}
	if g.Spec.Root.LocalFile != "" {
		return filepath.ToSlash(g.Spec.Root.LocalFile)
	}
	return config.LocalVariantPath(g.Spec.Root.File)
}

// LocalRootStandsIn implements config.LocalRootStandIn: the root file the local
// file extends, for target matching.
func (g *Generator) LocalRootStandsIn() string {
	if g.Spec.Root == nil {
		return ""
	}
	return g.Spec.Root.File
}

// LocalRuleOutputs implements config.LocalRuleProvider for split-aware specs:
// local rules are routed like shared ones and the routed rules become
// "<dir>/<id>.local<ext>".
func (g *Generator) LocalRuleOutputs(rules []config.ContentFile, baseDir string, cfg *config.Config,
) ([]config.OutputFile, []config.ContentFile, error) {
	spec := g.Spec.Outputs[OutputTypeRules]
	if spec == nil || !spec.Split {
		return nil, rules, nil
	}
	t := g.rulesTarget(spec)
	return presets.PlanLocalRules(t, g.splitRouting(spec, cfg, true), rules, baseDir, cfg, g.reservedLocalRootIDs(t)...)
}

// reservedLocalRootIDs is the rule id whose local file the generated local root
// file occupies, when that file sits in the rules folder of t ("ai-rulez" for
// .junie/rules/ai-rulez.local.md).
func (g *Generator) reservedLocalRootIDs(t rulefiles.Target) []string {
	local := g.LocalRootFile()
	if local == "" || path.Dir(local) != strings.TrimRight(t.Dir, "/") {
		return nil
	}
	if id, ok := strings.CutSuffix(path.Base(local), ".local"+t.Ext); ok {
		return []string{id}
	}
	return nil
}

// GetOutputPaths implements config.PresetGenerator. Returns root file (if any)
// plus the always-emitted directories declared in the spec.
func (g *Generator) GetOutputPaths(baseDir string) []string {
	var paths []string
	if g.Spec.Root != nil && g.Spec.Root.File != "" {
		paths = append(paths, filepath.Join(baseDir, g.Spec.Root.File))
	}
	for _, dir := range g.Spec.Directories {
		paths = append(paths, filepath.Join(baseDir, dir))
	}
	return paths
}

// Generate implements config.PresetGenerator. Composes the output slice in a
// stable order: declared directories → root file → per-type item files
// (skills, agents, commands) → sidecars.
func (g *Generator) Generate(content *config.ContentTree, baseDir string, cfg *config.Config) ([]config.OutputFile, error) {
	var outputs []config.OutputFile

	outputs = append(outputs, g.directoryOutputs(baseDir, cfg)...)

	reg := rulefiles.RegistryFor(cfg, g.Spec.Name)
	plan, err := g.planRules(content, cfg, reg)
	if err != nil {
		return nil, oops.With("preset", g.Spec.Name).Wrapf(err, "plan rules")
	}

	g.warnScopeLimits(cfg, plan)

	if g.Spec.Root != nil && !g.rootReplacedByAgentsMD(cfg) {
		rootOutput, err := g.renderRootFile(content, baseDir, cfg, plan)
		if err != nil {
			return nil, fmt.Errorf("render root file: %w", err)
		}
		outputs = append(outputs, rootOutput)
	}

	ruleOutputs, err := g.renderRuleFiles(plan, content, baseDir, cfg)
	if err != nil {
		return nil, err
	}
	outputs = append(outputs, g.withheldRulesDirMarker(baseDir, cfg, ruleOutputs)...)
	outputs = append(outputs, ruleOutputs...)

	// Per-type rendering in a fixed iteration order so output is deterministic
	// across map iterations. Rules were rendered above from the routing plan.
	for _, typ := range []string{OutputTypeSkills, OutputTypeAgents, OutputTypeCommands} {
		spec, ok := g.Spec.Outputs[typ]
		if !ok || spec == nil {
			continue
		}
		items := collectItemsByType(content, typ)
		for _, item := range items {
			if !g.itemAllowed(typ, spec, item, content, cfg) {
				continue
			}
			itemOutputs, err := g.renderItem(typ, spec, item, content, baseDir, cfg)
			if err != nil {
				return nil, fmt.Errorf("render %s %q: %w", typ, item.Name, err)
			}
			outputs = append(outputs, itemOutputs...)
		}
	}

	for _, sidecar := range g.Spec.Sidecars {
		if !g.evalPredicate(sidecar.EmitWhen, cfg) {
			continue
		}
		outputPath := filepath.Join(baseDir, sidecar.Path)
		rendered, err := g.renderSidecar(sidecar.Kind, cfg, outputPath)
		if err != nil {
			return nil, fmt.Errorf("render sidecar %s: %w", sidecar.Kind, err)
		}
		cfg.Analysis.Begin(outputPath, g.Spec.Name, config.OutputKindSidecar, sidecar.Kind, "").
			AddPart(config.PartKindSidecarDocument, sidecar.Kind, "", rendered.Body)
		outputs = append(outputs, config.OutputFile{
			Path:           outputPath,
			Content:        rendered.Body,
			PartiallyOwned: rendered.PartiallyOwned,
			MergeClaims:    rendered.Claims,
		})
	}

	return outputs, nil
}

// collectItemsByType returns the merged root + domain item slice for the given
// content type, deduplicated by name in source-precedence order and then
// alphabetised. Wraps the existing presets helpers so the renderer stays
// preset-agnostic. Every per-item output writes to a name-derived path, so the
// deduplication is load-bearing: without it two same-named items both render and
// the one written last replaces the other.
func collectItemsByType(content *config.ContentTree, typ string) []config.ContentFile {
	switch typ {
	case OutputTypeRules:
		return presets.AllInlineRules(content)
	case OutputTypeSkills:
		return presets.AllSkills(content)
	case OutputTypeAgents:
		return presets.AllAgents(content)
	case OutputTypeCommands:
		return presets.AllCommands(content)
	}
	return nil
}

// rulesPlan is the routing of rules and context between the root file and the
// rules output. The root file renders exactly inlineRules/inlineContext; the
// rules output renders exactly files (split specs) or legacy (non-split).
type rulesPlan struct {
	target        *rulefiles.Target
	files         []rulefiles.Item
	legacy        []config.ContentFile
	inlineRules   []config.ContentFile
	inlineContext []config.ContentFile
}

// ruleCount is the number of rules the root file inlines.
func (p *rulesPlan) ruleCount() int { return len(p.inlineRules) }

// rulesOutputAccepts reports whether the rules output of a legacy (non-split)
// spec takes a rule. The root file skips exactly the accepted rules.
func (g *Generator) rulesOutputAccepts(spec *OutputSpec, rule config.ContentFile) bool {
	return g.filterAllows(spec, rule)
}

// planRules splits the deduplicated rules and context between the root file
// and the rules output. Split specs delegate to rulefiles.Plan with the routing
// that the configured rules mode selects; legacy specs keep the filter-based
// behavior and leave context inline.
func (g *Generator) planRules(content *config.ContentTree, cfg *config.Config, reg *rulefiles.Registry) (*rulesPlan, error) {
	rules := presets.AllInlineRules(content)
	ctx := presets.AllInlineContext(content)
	spec := g.Spec.Outputs[OutputTypeRules]
	root := g.rootTarget()
	plan := &rulesPlan{inlineRules: rulefiles.FilterInline(rules, root), inlineContext: rulefiles.FilterInline(ctx, root)}
	if spec == nil {
		return plan, nil
	}

	if !spec.Split {
		plan.inlineRules = nil
		for _, rule := range rules {
			switch {
			case g.rulesOutputAccepts(spec, rule):
				plan.legacy = append(plan.legacy, rule)
			case rulefiles.InlineAllowed(rule, root):
				plan.inlineRules = append(plan.inlineRules, rule)
			}
		}
		return plan, nil
	}

	target := g.rulesTarget(spec)
	plan.target = &target
	files, inlineRules, inlineContext, err := rulefiles.Plan(rules, ctx, plan.target, g.splitRouting(spec, cfg, false), rulefiles.ScopeOf(cfg), reg)
	if err != nil {
		return nil, oops.With("preset", g.Spec.Name).Wrapf(err, "plan %s rule files", g.Spec.Name)
	}
	plan.files, plan.inlineRules, plan.inlineContext = files, inlineRules, inlineContext
	return plan, nil
}

// splitRouting picks the Routing for a split spec. In inline mode a spec
// without inline_filter keeps everything in the root file.
//
// Machine-local rules are not part of AGENTS.md, so their always-on rules stay in
// the rules folder unless a local root file carries them (the claude shim has
// none, so CLAUDE.local.md does).
func (g *Generator) splitRouting(spec *OutputSpec, cfg *config.Config, local bool) rulefiles.Routing {
	routing := rulefiles.RoutingFor(cfg.RulesModeFor(g.Spec.Name), true)
	if routing == rulefiles.RoutingScopedOnly && spec.InlineFilter != InlineFilterPathScoped {
		return rulefiles.RoutingNone
	}
	if g.foldsAlwaysOnIntoAgentsMD(cfg) && (!local || g.importsAgentsMD(cfg)) {
		// Always-on rules and context live in the shared AGENTS.md.
		return rulefiles.WithoutAlwaysOn(routing)
	}
	return routing
}

// foldsAlwaysOnIntoAgentsMD reports whether agents_md moves this provider's
// always-on rules and context into the shared AGENTS.md, leaving its rules
// folder the rest (claude, junie).
func (g *Generator) foldsAlwaysOnIntoAgentsMD(cfg *config.Config) bool {
	if g.Spec.Root == nil || !cfg.ReadsSharedAgentsMD(g.Spec.Name) {
		return false
	}
	consumer, _ := config.SharedOutputConsumerFor(g.Spec.Name)
	return consumer.Folder != config.RulesFolderNone
}

// rootReplacedByAgentsMD reports whether agents_md makes the shared AGENTS.md
// stand in for this provider's root file, which is then not written at all
// (hermes, junie). Claude keeps its root file as an import shim.
func (g *Generator) rootReplacedByAgentsMD(cfg *config.Config) bool {
	if g.Spec.Root == nil || !cfg.ReadsSharedAgentsMD(g.Spec.Name) {
		return false
	}
	consumer, _ := config.SharedOutputConsumerFor(g.Spec.Name)
	return consumer.OwnRootFile != ""
}

// importsAgentsMD reports whether the agents_md flag turns this provider's root
// file into a shim importing the shared AGENTS.md (claude).
func (g *Generator) importsAgentsMD(cfg *config.Config) bool {
	if !cfg.AgentsMD || g.Spec.Root == nil {
		return false
	}
	consumer, ok := config.SharedOutputConsumerFor(g.Spec.Name)
	return ok && consumer.ImportsAgentsMD
}

// rootTarget describes the root file for inline target filtering.
func (g *Generator) rootTarget() rulefiles.Target {
	file := ""
	if g.Spec.Root != nil {
		file = g.Spec.Root.File
	}
	return rulefiles.RootTarget(g.Spec.Name, file)
}

// contextSummary reports whether inline context entries carry their "summary"
// extra. A root file written by several presets (AGENTS.md: codex, opencode,
// xum, amp) must render identically whichever writes it last; the hand-written
// presets never emitted summaries, so the provider does not either.
func (g *Generator) contextSummary() bool {
	return g.Spec.Root == nil || !rulefiles.SharedRootFile(g.Spec.Root.File)
}

// warnScopeLimits reports, in a scope run, rules the tool will not load from the
// scope directory.
func (g *Generator) warnScopeLimits(cfg *config.Config, plan *rulesPlan) {
	if g.Spec.Root != nil && g.Spec.Name == presetNameJunie && !g.rootReplacedByAgentsMD(cfg) {
		rulefiles.WarnUnreadScopeFile(cfg, g.Spec.Name, g.Spec.Root.File, plan.inlineRules, plan.inlineContext)
	}
	if spec := g.Spec.Outputs[OutputTypeRules]; spec != nil && !spec.Split {
		rulefiles.WarnScopeLegacyRules(cfg, g.Spec.Name, spec.Dir)
	}
}

// presetNameJunie names the preset whose root file only the repository root
// provides.
const presetNameJunie = "junie"

// directoryOutputs returns the always-emitted directory markers of the spec,
// without the rules folder when a monorepo scope is generated.
func (g *Generator) directoryOutputs(baseDir string, cfg *config.Config) []config.OutputFile {
	var outputs []config.OutputFile
	for _, dir := range g.Spec.Directories {
		if g.isScopedRulesDir(dir, cfg) || (g.withheldRulesDir(cfg) && g.isRulesDir(dir)) {
			continue
		}
		outputs = append(outputs, config.OutputFile{Path: filepath.Join(baseDir, dir), IsDir: true})
	}
	return outputs
}

// isRulesDir reports whether dir is the rules folder of a split rules output.
func (g *Generator) isRulesDir(dir string) bool {
	spec := g.Spec.Outputs[OutputTypeRules]
	return spec != nil && spec.Split && filepath.ToSlash(dir) == filepath.ToSlash(spec.Dir)
}

// withheldRulesDirMarker returns the rules folder marker withheld by
// directoryOutputs, once a rule file lands in the folder.
func (g *Generator) withheldRulesDirMarker(baseDir string, cfg *config.Config, ruleOutputs []config.OutputFile) []config.OutputFile {
	if len(ruleOutputs) == 0 || !g.withheldRulesDir(cfg) {
		return nil
	}
	return []config.OutputFile{{Path: filepath.Join(baseDir, g.Spec.Outputs[OutputTypeRules].Dir), IsDir: true}}
}

// withheldRulesDir reports whether the rules folder marker is emitted only when a
// rule file lands in the folder: agents_md moves the always-on content into the
// shared AGENTS.md, so a folder without scoped items would stay empty (and be
// pruned and recreated on each toggle). A monorepo scope has no folder of its own.
func (g *Generator) withheldRulesDir(cfg *config.Config) bool {
	spec := g.Spec.Outputs[OutputTypeRules]
	return spec != nil && spec.Split && !rulefiles.InScope(cfg) && cfg.ReadsSharedAgentsMD(g.Spec.Name)
}

// isScopedRulesDir reports whether dir is the rules folder of a split rules
// output while a monorepo scope is generated: the scope's rule files live in
// the root folder, so the scope gets no folder of its own.
func (g *Generator) isScopedRulesDir(dir string, cfg *config.Config) bool {
	spec := g.Spec.Outputs[OutputTypeRules]
	return rulefiles.InScope(cfg) && spec != nil && spec.Split && filepath.ToSlash(dir) == filepath.ToSlash(spec.Dir)
}

// rulesTarget builds the rulefiles Target of a split rules output.
func (g *Generator) rulesTarget(spec *OutputSpec) rulefiles.Target {
	dialect := rulefiles.Dialect(spec.Dialect)
	return rulefiles.Target{
		Preset:    g.Spec.Name,
		Dir:       spec.Dir,
		RootFile:  g.rootTarget().RootFile,
		Ext:       strings.TrimPrefix(spec.Filename, "{id}"),
		Dialect:   dialect,
		Recursive: dialect == rulefiles.DialectClaude,
		Banner:    true,
	}
}

// renderRuleFiles emits the rules output: rule/context files of a split plan
// or the legacy per-item rule files.
func (g *Generator) renderRuleFiles(plan *rulesPlan, content *config.ContentTree, baseDir string, cfg *config.Config) ([]config.OutputFile, error) {
	spec := g.Spec.Outputs[OutputTypeRules]
	if spec == nil {
		return nil, nil
	}
	var outputs []config.OutputFile
	for _, rule := range plan.legacy {
		itemOutputs, err := g.renderItem(OutputTypeRules, spec, rule, content, baseDir, cfg)
		if err != nil {
			return nil, fmt.Errorf("render %s %q: %w", OutputTypeRules, rule.Name, err)
		}
		outputs = append(outputs, itemOutputs...)
	}
	for i := range plan.files {
		it := &plan.files[i]
		text, notes, err := rulefiles.Render(*plan.target, *it, cfg)
		if err != nil {
			return nil, oops.With("preset", g.Spec.Name, "rule", it.File.Name).Wrapf(err, "render rule file")
		}
		outputPath := rulefiles.RulesDirPath(cfg, baseDir, *plan.target, rulefiles.FileName(*plan.target, *it))
		rulefiles.ReportNotes(outputPath, notes)
		cfg.Analysis.Begin(outputPath, g.Spec.Name, config.OutputKindRuleFile, it.ID, it.File.Path).
			AddPart(config.PartKindItemBody, "body", it.File.Path, text)
		outputs = append(outputs, config.OutputFile{Path: outputPath, Content: text})
	}
	return outputs, nil
}

// hasPathScope reports whether a content file declares file globs, i.e. it
// should be delivered through the tool's path-scoped mechanism.
func hasPathScope(item config.ContentFile) bool {
	return item.Metadata.ResolveActivation().Mode == config.ActivationGlob
}

// filterAllows applies the closed-set filter predicate. Currently only one
// filter exists: include_if_targeting_provider drops items whose
// Metadata.Targets is non-empty and excludes this provider.
func (g *Generator) filterAllows(spec *OutputSpec, item config.ContentFile) bool {
	if spec.Filter == "" {
		return true
	}
	if spec.Filter == FilterIncludeIfTargetingProvider {
		if item.Metadata == nil {
			return true
		}
		return targetmatch.Allow(item.Metadata.Targets, []string{g.Spec.Name})
	}
	if spec.Filter == FilterPathScoped {
		return hasPathScope(item)
	}
	return false
}

// itemAllowed applies the output's filter to one item. placement_core needs the
// content tree and config (an item's domain and the [placement] block), so it is
// handled apart from the item-only filters.
func (g *Generator) itemAllowed(typ string, spec *OutputSpec, item config.ContentFile, content *config.ContentTree, cfg *config.Config) bool {
	if spec.Filter == FilterPlacementCore {
		return g.placementAllows(typ, item, content, cfg)
	}
	return g.filterAllows(spec, item)
}

// renderItem produces the OutputFile(s) for a single per-item file: the file
// itself, its parent directory (if the filename template contains a slash),
// and optionally bundled skill resources when spec.Resources is true.
func (g *Generator) renderItem(typ string, spec *OutputSpec, item config.ContentFile, content *config.ContentTree, baseDir string, cfg *config.Config) ([]config.OutputFile, error) {
	itemID := computeItemID(typ, item)
	relFilename := strings.ReplaceAll(spec.Filename, "{id}", itemID)
	itemDir := filepath.Join(baseDir, spec.Dir)
	outputPath := filepath.Join(itemDir, relFilename)
	parentDir := filepath.Dir(outputPath)

	var outputs []config.OutputFile

	// Emit the per-item subdirectory entry when the filename template nests
	// the file inside one (e.g. skills/{id}/SKILL.md). Matches the existing
	// preset behavior that depends on these markers for stale-file cleanup.
	if parentDir != itemDir {
		outputs = append(outputs, config.OutputFile{
			Path:  parentDir,
			IsDir: true,
		})
	}

	analysis := cfg.Analysis.Begin(outputPath, g.Spec.Name, outputKindForType(typ), itemID, item.Path)
	body, err := g.renderItemBody(typ, spec, item, content, cfg, outputPath, newPartRecorder(analysis))
	if err != nil {
		return nil, err
	}

	outputs = append(outputs, config.OutputFile{
		Path:    outputPath,
		Content: body,
	})

	if spec.Resources {
		outputs = append(outputs, presets.SkillResourceOutputs(&item, parentDir)...)
	}

	return outputs, nil
}

// computeItemID derives the filename stem for an item. Skills use their
// path (which already encodes the canonical skill ID), agents and commands
// derive from a sanitized name.
func computeItemID(typ string, item config.ContentFile) string {
	if typ == OutputTypeSkills {
		// Path format: .../skills/{skill-id}/SKILL.md → {skill-id}
		dir := filepath.Dir(item.Path)
		return filepath.Base(dir)
	}
	return sanitizeAgentID(item.Name)
}

// sanitizeAgentID mirrors the original preset behavior: lowercase, replace
// spaces and underscores with dashes. Distinct from presets.SanitizeName,
// which strips non-alphanumeric characters as well.
func sanitizeAgentID(name string) string {
	id := strings.ToLower(name)
	id = strings.ReplaceAll(id, " ", "-")
	id = strings.ReplaceAll(id, "_", "-")
	return id
}

// renderItemBody composes the body of a per-item file from the spec's
// section list. Closed-set dispatch — adding a new section needs a new
// constant in spec.go and a new case here.
func (g *Generator) renderItemBody(typ string, spec *OutputSpec, item config.ContentFile, content *config.ContentTree, cfg *config.Config, outputPath string, recorder *partRecorder) (string, error) {
	var b strings.Builder
	if spec.Body == nil {
		return "", nil
	}

	for _, section := range spec.Body.Sections {
		start := recorder.mark(&b)
		switch section {
		case SectionBodyFrontmatter:
			frontmatter, err := g.writeFrontmatter(&b, typ, spec.Frontmatter, item, cfg)
			if err != nil {
				return "", err
			}
			recorder.section(config.PartKindItemFrontmatter, "frontmatter", item.Path, start, &b)
			// name and description are reported apart from the rest of the
			// frontmatter because a harness loads them on a different schedule
			// from the block that carries them: the name appears in every skill
			// listing, the description only in some harness modes.
			recorder.literal(config.PartKindItemName, "name", item.Path, frontmatterString(frontmatter, "name"))
			recorder.literal(config.PartKindItemDescription, "description", item.Path, frontmatterString(frontmatter, "description"))
		case SectionBodyContent:
			b.WriteString(item.Content)
			recorder.section(config.PartKindItemBody, "body", item.Path, start, &b)
		case SectionBodyResourceIndex:
			b.WriteString(presets.RenderSkillResourcesIndex(&item))
			recorder.section(config.PartKindItemResourceIndex, "resource_index", item.Path, start, &b)
		case SectionBodyTargetedRules:
			writeTargetedSection(&b, "Rules", presets.FilterContentByExplicitTargetsExported(content.Rules, outputPath, cfg.BaseDir), true, cfg.IsCompact())
			recorder.section(config.PartKindItemTargetedRules, "targeted_rules", item.Path, start, &b)
		case SectionBodyTargetedContext:
			writeTargetedSection(&b, "Context", presets.FilterContentByExplicitTargetsExported(content.Context, outputPath, cfg.BaseDir), false, cfg.IsCompact())
			recorder.section(config.PartKindItemTargetedContext, "targeted_context", item.Path, start, &b)
		}
	}
	rendered := b.String()
	recorder.flush(rendered)
	return rendered, nil
}

// writeTargetedSection writes a "## <Heading>" section listing the included
// content files. includePriority controls whether the "**Priority:**" line is
// emitted under each entry (matches the existing claude-skill formatting). When
// compact is true the priority line is suppressed regardless of includePriority,
// mirroring the inline-rules compact behavior.
func writeTargetedSection(b *strings.Builder, heading string, items []config.ContentFile, includePriority, compact bool) {
	if len(items) == 0 {
		return
	}
	// The leading blank line + "## <Heading>" pattern mirrors the existing
	// claude.go output exactly, including the extra newline before Rules and
	// the single newline before Context.
	if heading == "Rules" {
		b.WriteString("\n\n## Rules\n\n")
	} else {
		b.WriteString("\n## ")
		b.WriteString(heading)
		b.WriteString("\n\n")
	}
	for _, item := range items {
		b.WriteString("### ")
		b.WriteString(item.Name)
		b.WriteString("\n")
		if includePriority && !compact && item.Metadata != nil && item.Metadata.Priority != "" {
			b.WriteString("**Priority:** ")
			b.WriteString(item.Metadata.Priority)
			b.WriteString("\n\n")
		} else if heading == "Context" {
			b.WriteString("\n")
		}
		b.WriteString(item.Content)
		b.WriteString("\n\n")
	}
}

// writeFrontmatter writes the YAML frontmatter block (--- header, --- footer,
// trailing blank line). Composition order:
//  1. `name` (always emitted, from item.Name)
//  2. constants (insertion order via spec)
//  3. resolved effort (if emit_effort)
//  4. resolved model (if emit_model)
//  5. tools list (if tools=true and Metadata.Tools non-empty)
//  6. skills list (if skills=true and Metadata.Skills non-empty)
//  7. ordered fields from Metadata.Extra (whitelist)
//  8. include_extras: every remaining Metadata.Extra key minus extras_blacklist
//
// YAML map keys are sorted alphabetically by yaml.v3 on marshal, so insertion
// order here only matters when constants override a computed value (they
// don't in any current builtin).
// It returns the assembled frontmatter map so callers can report on individual
// fields without re-parsing the rendered YAML.
func (g *Generator) writeFrontmatter(b *strings.Builder, typ string, spec *FrontmatterSpec, item config.ContentFile, cfg *config.Config) (map[string]any, error) {
	frontmatter := g.buildFrontmatterMap(typ, spec, item, cfg)

	yamlData, err := yaml.Marshal(frontmatter)
	if err != nil {
		return nil, fmt.Errorf("marshal frontmatter: %w", err)
	}
	b.WriteString("---\n")
	b.Write(yamlData)
	b.WriteString("---\n\n")
	return frontmatter, nil
}

// buildFrontmatterMap assembles the frontmatter map. Composition order is
// documented on the writeFrontmatter docstring above.
func (g *Generator) buildFrontmatterMap(typ string, spec *FrontmatterSpec, item config.ContentFile, cfg *config.Config) map[string]any {
	frontmatter := map[string]any{"name": item.Name}
	if spec == nil {
		return frontmatter
	}
	for k, v := range spec.Constants {
		frontmatter[k] = v
	}
	g.applyResolvedScalars(frontmatter, spec, item, cfg)
	if item.Metadata != nil {
		applyTypedLists(frontmatter, spec, item.Metadata)
		applyOrderedFields(frontmatter, spec, item.Metadata)
		applyExtras(frontmatter, spec, item.Metadata)
	}
	if typ == OutputTypeRules && spec.Paths && item.Metadata != nil {
		if scope := item.Metadata.PathScope(); len(scope) > 0 {
			frontmatter["paths"] = scope
		}
	}
	// Honor the global per-field omission policy. model/effort are already
	// suppressed by the shared resolvers returning ""; tools and description
	// are written here, so drop them post-hoc.
	if cfg.OmitsAgentField("tools") {
		delete(frontmatter, "tools")
	}
	if cfg.OmitsAgentField("description") {
		delete(frontmatter, "description")
	}
	// A skill whose frontmatter failed to parse loads with nil Metadata, so
	// applyOrderedFields/applyExtras never get a chance to write its
	// description — the generated SKILL.md would ship without one and the
	// skill becomes invisible to the assistant. Honor the documented name
	// fallback for any spec that surfaces a description field (#176).
	if typ == OutputTypeSkills && (slices.Contains(spec.Fields, "description") || spec.IncludeExtras) {
		if desc, ok := frontmatter["description"].(string); !ok || strings.TrimSpace(desc) == "" {
			frontmatter["description"] = config.SkillDescriptionOrFallback(config.SkillDescription(item.Metadata), config.SkillID(item))
		}
	}
	return frontmatter
}

func (g *Generator) applyResolvedScalars(frontmatter map[string]any, spec *FrontmatterSpec, item config.ContentFile, cfg *config.Config) {
	if spec.EmitEffort {
		if mapped := g.resolveEffort(item, cfg); mapped != "" {
			frontmatter[effortFrontmatterKey(spec)] = mapped
		}
	}
	if spec.EmitModel && g.Spec.Model != nil {
		if model := presets.ResolveAgentModel(g.Spec.Name, item, cfg); model != "" {
			frontmatter[g.Spec.Model.Field] = model
		}
	}
}

// effortFrontmatterKey is the key the resolved effort is written under: the
// spec's effort_field, or "effort" when it leaves it unset.
func effortFrontmatterKey(spec *FrontmatterSpec) string {
	if spec.EffortField != "" {
		return spec.EffortField
	}
	return "effort"
}

func applyTypedLists(frontmatter map[string]any, spec *FrontmatterSpec, meta *config.Metadata) {
	if spec.Tools && len(meta.Tools) > 0 {
		frontmatter["tools"] = meta.Tools
	}
	if spec.Skills && len(meta.Skills) > 0 {
		frontmatter["skills"] = meta.Skills
	}
}

func applyOrderedFields(frontmatter map[string]any, spec *FrontmatterSpec, meta *config.Metadata) {
	for _, field := range spec.Fields {
		if val, ok := meta.Extra[field]; ok && val != "" {
			frontmatter[field] = val
		}
	}
}

func applyExtras(frontmatter map[string]any, spec *FrontmatterSpec, meta *config.Metadata) {
	if !spec.IncludeExtras {
		return
	}
	blacklist := buildBlacklistSet(spec.ExtrasBlacklist)
	for k, v := range meta.Extra {
		if blacklist[k] {
			continue
		}
		if _, alreadySet := frontmatter[k]; alreadySet {
			continue
		}
		frontmatter[k] = v
	}
}

// resolveEffort runs the shared effort resolver then translates through the
// provider's effort_map. Returns "" when no effort applies for this item.
func (g *Generator) resolveEffort(item config.ContentFile, cfg *config.Config) string {
	raw := presets.ResolveAgentEffort(g.Spec.Name, item, cfg)
	if raw == "" {
		return ""
	}
	if g.Spec.EffortMap == nil {
		return ""
	}
	if mapped, ok := g.Spec.EffortMap.Values[raw]; ok {
		return mapped
	}
	return ""
}

func buildBlacklistSet(blacklist []string) map[string]bool {
	set := make(map[string]bool, len(blacklist))
	for _, k := range blacklist {
		set[k] = true
	}
	// "name" is unconditionally blacklisted from the extras pass — it's
	// always set explicitly first and re-emitting it from extras would
	// double the key in the yaml map.
	set["name"] = true
	return set
}

// agentsMDImportLine is the Claude Code memory import of the shared AGENTS.md.
// The path is relative to the importing file, so a scope's CLAUDE.md imports the
// AGENTS.md next to it.
const agentsMDImportLine = "@AGENTS.md"

// renderRootFile composes the root instructions file (CLAUDE.md, AGENTS.md, ...)
// from the spec.Root.Sections list. Closed-set dispatch on each section.
func (g *Generator) renderRootFile(content *config.ContentTree, baseDir string, cfg *config.Config, plan *rulesPlan) (config.OutputFile, error) {
	var b strings.Builder

	rootRelPath := g.Spec.Root.File
	outputPath := filepath.Join(baseDir, rootRelPath)
	recorder := newPartRecorder(cfg.Analysis.Begin(outputPath, g.Spec.Name, config.OutputKindRoot, "", ""))

	sections := g.Spec.Root.Sections
	if g.importsAgentsMD(cfg) {
		// A shim: the banner and the import, nothing that AGENTS.md already carries.
		sections = []string{SectionRootHeader, SectionRootAgentsMDImport}
	}
	for _, section := range sections {
		start := recorder.mark(&b)
		switch section {
		case SectionRootHeader:
			ruleCount, agentCount := countContent(content, plan)
			data := &templates.TemplateData{
				ProjectName: cfg.Name,
				Timestamp:   cfg.HeaderTimestamp(),
				ConfigFile:  presets.ConfigFileName(cfg),
				OutputFile:  rootRelPath,
				Config:      cfg,
				RuleCount:   ruleCount,
				AgentCount:  agentCount,
			}
			b.WriteString(templates.GenerateHeader(data))
		case SectionRootAgentsMDImport:
			b.WriteString(agentsMDImportLine)
			b.WriteString("\n")
		case SectionRootTitle:
			b.WriteString("# ")
			b.WriteString(cfg.Name)
			b.WriteString("\n\n")
		case SectionRootDescription:
			if cfg.Description != "" {
				b.WriteString(cfg.Description)
				b.WriteString("\n\n")
			}
		case SectionRootRulesInline:
			// Rules and context are recorded per entry, not per section: a cost
			// report has to be able to name the rule that is expensive. Path-scoped
			// rules move to the tool's rules directory instead of the root file.
			writeInlineRules(&b, plan.inlineRules, cfg.IsCompact(), recorder)
			continue
		case SectionRootContextInline:
			writeInlineContext(&b, plan.inlineContext, cfg.IsCompact(), g.contextSummary(), recorder)
			continue
		case SectionRootAgentsDelegation:
			allAgents := presets.AllAgents(content)
			presets.RenderAgentsSectionExported(&b, content, allAgents)
		}
		recorder.section(partKindForRootSection(section), section, "", start, &b)
	}

	rendered := b.String()
	recorder.flush(rendered)

	return config.OutputFile{
		Path:    outputPath,
		Content: rendered,
	}, nil
}

func countContent(content *config.ContentTree, plan *rulesPlan) (rules, agents int) {
	// Count the rules the root file inlines, after deduplication and routing, so
	// the header reflects what is actually rendered.
	rules = plan.ruleCount()
	// Count agents after dedup + extends resolution so the header matches what
	// AllAgents actually renders (mirrors the rules count above).
	agents = len(presets.AllAgents(content))
	return
}

// writeInlineRules mirrors the "## Rules" block produced by the legacy
// renderClaudeMarkdown — heading + entries with **Priority:** when set and
// markdown-processed content. rules is the already routed and deduplicated
// slice the root file inlines.
func writeInlineRules(b *strings.Builder, rules []config.ContentFile, compact bool, recorder *partRecorder) {
	if len(rules) == 0 {
		return
	}
	rulefiles.WriteInlineRules(b, rules, rulefiles.InlineOpts{Compact: compact, AppliesTo: true}, recorder)
}

// writeInlineContext mirrors the "## Context" block produced by the legacy
// renderClaudeMarkdown, including the per-entry "summary" extras handling. When
// compact is true the per-entry summary line is suppressed, mirroring the
// compact suppression of the inline-rules priority line.
func writeInlineContext(b *strings.Builder, contextFiles []config.ContentFile, compact, summary bool, recorder *partRecorder) {
	if len(contextFiles) == 0 {
		return
	}
	rulefiles.WriteInlineContext(b, contextFiles, rulefiles.InlineOpts{Compact: compact, AppliesTo: true, ContextSummary: summary}, recorder)
}
