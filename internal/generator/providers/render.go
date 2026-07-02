package providers

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/presets"
	"github.com/Goldziher/ai-rulez/internal/markdown"
	"github.com/Goldziher/ai-rulez/internal/templates"
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

	for _, dir := range g.Spec.Directories {
		outputs = append(outputs, config.OutputFile{
			Path:  filepath.Join(baseDir, dir),
			IsDir: true,
		})
	}

	if g.Spec.Root != nil {
		rootOutput, err := g.renderRootFile(content, baseDir, cfg)
		if err != nil {
			return nil, fmt.Errorf("render root file: %w", err)
		}
		outputs = append(outputs, rootOutput)
	}

	// Per-type rendering in a fixed iteration order so output is deterministic
	// across map iterations.
	for _, typ := range []string{OutputTypeSkills, OutputTypeAgents, OutputTypeCommands} {
		spec, ok := g.Spec.Outputs[typ]
		if !ok || spec == nil {
			continue
		}
		items := collectItemsByType(content, typ)
		for _, item := range items {
			if !g.filterAllows(spec, item) {
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
		body, err := g.renderSidecar(sidecar.Kind, cfg)
		if err != nil {
			return nil, fmt.Errorf("render sidecar %s: %w", sidecar.Kind, err)
		}
		outputs = append(outputs, config.OutputFile{
			Path:    filepath.Join(baseDir, sidecar.Path),
			Content: body,
		})
	}

	return outputs, nil
}

// collectItemsByType returns the merged root + domain item slice for the given
// content type, alphabetised. Wraps the existing presets helpers so the
// renderer stays preset-agnostic.
func collectItemsByType(content *config.ContentTree, typ string) []config.ContentFile {
	switch typ {
	case OutputTypeSkills:
		return presets.CombineContentFiles(content.Skills, presets.GetAllDomainSkills(content))
	case OutputTypeAgents:
		return presets.CombineContentFiles(content.Agents, presets.GetAllDomainAgents(content))
	case OutputTypeCommands:
		return presets.CombineContentFiles(content.Commands, presets.GetAllDomainCommands(content))
	}
	return nil
}

// filterAllows applies the closed-set filter predicate. Currently only one
// filter exists: include_if_targeting_provider drops items whose
// Metadata.Targets is non-empty and excludes this provider.
func (g *Generator) filterAllows(spec *OutputSpec, item config.ContentFile) bool {
	if spec.Filter == "" {
		return true
	}
	if spec.Filter == FilterIncludeIfTargetingProvider {
		if item.Metadata == nil || len(item.Metadata.Targets) == 0 {
			return true
		}
		for _, target := range item.Metadata.Targets {
			if target == g.Spec.Name {
				return true
			}
		}
		return false
	}
	return false
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

	body, err := g.renderItemBody(spec, item, content, cfg, outputPath)
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
func (g *Generator) renderItemBody(spec *OutputSpec, item config.ContentFile, content *config.ContentTree, cfg *config.Config, outputPath string) (string, error) {
	var b strings.Builder
	if spec.Body == nil {
		return "", nil
	}

	for _, section := range spec.Body.Sections {
		switch section {
		case SectionBodyFrontmatter:
			if err := g.writeFrontmatter(&b, spec.Frontmatter, item, cfg); err != nil {
				return "", err
			}
		case SectionBodyContent:
			b.WriteString(item.Content)
		case SectionBodyResourceIndex:
			b.WriteString(presets.RenderSkillResourcesIndex(&item))
		case SectionBodyTargetedRules:
			writeTargetedSection(&b, "Rules", presets.FilterContentByExplicitTargetsExported(content.Rules, outputPath, cfg.BaseDir), true)
		case SectionBodyTargetedContext:
			writeTargetedSection(&b, "Context", presets.FilterContentByExplicitTargetsExported(content.Context, outputPath, cfg.BaseDir), false)
		}
	}
	return b.String(), nil
}

// writeTargetedSection writes a "## <Heading>" section listing the included
// content files. includePriority controls whether the "**Priority:**" line is
// emitted under each entry (matches the existing claude-skill formatting).
func writeTargetedSection(b *strings.Builder, heading string, items []config.ContentFile, includePriority bool) {
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
		if includePriority && item.Metadata != nil && item.Metadata.Priority != "" {
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
func (g *Generator) writeFrontmatter(b *strings.Builder, spec *FrontmatterSpec, item config.ContentFile, cfg *config.Config) error {
	frontmatter := g.buildFrontmatterMap(spec, item, cfg)

	yamlData, err := yaml.Marshal(frontmatter)
	if err != nil {
		return fmt.Errorf("marshal frontmatter: %w", err)
	}
	b.WriteString("---\n")
	b.Write(yamlData)
	b.WriteString("---\n\n")
	return nil
}

// buildFrontmatterMap assembles the frontmatter map. Composition order is
// documented on the writeFrontmatter docstring above.
func (g *Generator) buildFrontmatterMap(spec *FrontmatterSpec, item config.ContentFile, cfg *config.Config) map[string]any {
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
	return frontmatter
}

func (g *Generator) applyResolvedScalars(frontmatter map[string]any, spec *FrontmatterSpec, item config.ContentFile, cfg *config.Config) {
	if spec.EmitEffort {
		if mapped := g.resolveEffort(item, cfg); mapped != "" {
			frontmatter["effort"] = mapped
		}
	}
	if spec.EmitModel && g.Spec.Model != nil {
		if model := presets.ResolveAgentModel(g.Spec.Name, item, cfg); model != "" {
			frontmatter[g.Spec.Model.Field] = model
		}
	}
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

// renderRootFile composes the root instructions file (CLAUDE.md, AGENTS.md, ...)
// from the spec.Root.Sections list. Closed-set dispatch on each section.
func (g *Generator) renderRootFile(content *config.ContentTree, baseDir string, cfg *config.Config) (config.OutputFile, error) {
	var b strings.Builder

	rootRelPath := g.Spec.Root.File
	for _, section := range g.Spec.Root.Sections {
		switch section {
		case SectionRootHeader:
			ruleCount, agentCount := countContent(content)
			data := &templates.TemplateData{
				ProjectName: cfg.Name,
				Timestamp:   time.Now(),
				ConfigFile:  presets.ConfigFileName(cfg),
				OutputFile:  rootRelPath,
				Config:      cfg,
				RuleCount:   ruleCount,
				AgentCount:  agentCount,
			}
			b.WriteString(templates.GenerateHeader(data))
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
			writeInlineRules(&b, content, cfg.IsCompact())
		case SectionRootContextInline:
			writeInlineContext(&b, content)
		case SectionRootAgentsDelegation:
			allAgents := presets.CombineContentFiles(content.Agents, presets.GetAllDomainAgents(content))
			presets.RenderAgentsSectionExported(&b, content, allAgents)
		}
	}

	return config.OutputFile{
		Path:    filepath.Join(baseDir, rootRelPath),
		Content: b.String(),
	}, nil
}

func countContent(content *config.ContentTree) (rules, agents int) {
	// Count rules after deduplication so the header reflects what is actually
	// rendered — a builtin and an include defining the same rule name collapse
	// to one entry (see writeInlineRules).
	rules = len(presets.AllInlineRules(content))
	agents = len(content.Agents)
	for _, domain := range content.Domains {
		agents += len(domain.Agents)
	}
	return
}

// writeInlineRules mirrors the "## Rules" block produced by the legacy
// renderClaudeMarkdown — heading + entries with **Priority:** when set and
// markdown-processed content.
func writeInlineRules(b *strings.Builder, content *config.ContentTree, compact bool) {
	allRules := presets.AllInlineRules(content)
	if len(allRules) == 0 {
		return
	}
	b.WriteString("## Rules\n\n")
	for _, rule := range allRules {
		b.WriteString("### ")
		b.WriteString(rule.Name)
		b.WriteString("\n\n")
		if !compact && rule.Metadata != nil && rule.Metadata.Priority != "" {
			b.WriteString("**Priority:** ")
			b.WriteString(rule.Metadata.Priority)
			b.WriteString("\n\n")
		}
		b.WriteString(markdown.ProcessEmbeddedContent(rule.Content))
		b.WriteString("\n\n")
	}
}

// writeInlineContext mirrors the "## Context" block produced by the legacy
// renderClaudeMarkdown, including the per-entry "summary" extras handling.
func writeInlineContext(b *strings.Builder, content *config.ContentTree) {
	allContext := presets.AllInlineContext(content)
	if len(allContext) == 0 {
		return
	}
	b.WriteString("## Context\n\n")
	for _, ctx := range allContext {
		b.WriteString("### ")
		b.WriteString(ctx.Name)
		b.WriteString("\n\n")
		if ctx.Metadata != nil && ctx.Metadata.Extra["summary"] != "" {
			b.WriteString(ctx.Metadata.Extra["summary"])
			b.WriteString("\n\n")
		}
		b.WriteString(markdown.ProcessEmbeddedContent(ctx.Content))
		b.WriteString("\n\n")
	}
}
