package config

import (
	"path/filepath"
	"slices"
	"strings"
)

// Provider output directory names InferOutputKind matches on. rulesDir,
// skillsDir, agentsDir and commandsDir are the source-tree names from loader.go;
// these two have no source-tree equivalent.
const (
	// promptsDir is the directory some providers use for commands.
	promptsDir = "prompts"
	// clineRulesDir is Cline's rules directory, which is not nested under a
	// provider directory.
	clineRulesDir = ".clinerules"
)

// markdownExt is the canonical markdown extension.
const markdownExt = ".md"

// markdownExtensions are the extensions a provider may use for a markdown
// output. .mdc is Cursor's rule format and .mdx appears in documentation trees.
var markdownExtensions = []string{markdownExt, ".markdown", ".mdx", ".mdc"}

// documentExtensions are the extensions of a settings or manifest document.
var documentExtensions = []string{".json", ".jsonc", ".yaml", ".yml", ".toml", ".ini"}

// OutputKind classifies a generated output so a cost report can reason about
// when an agent loads it. The renderers that know the kind stamp it directly
// (see internal/generator/providers/render.go); anything they do not claim is
// classified from its path by InferOutputKind, which is a documented
// approximation rather than ground truth.
type OutputKind string

// Output kinds.
const (
	// OutputKindUnknown is an output no renderer claimed and whose path did not
	// match any known shape. Reports must not fold these into an always-loaded
	// total — when ai-rulez cannot say how a file is loaded it has to say so.
	OutputKindUnknown OutputKind = "unknown"
	// OutputKindRoot is a top-level instructions file (CLAUDE.md, AGENTS.md, …)
	// read at the start of every session.
	OutputKindRoot OutputKind = "root"
	// OutputKindLocalRoot is a machine-local root variant (CLAUDE.local.md).
	OutputKindLocalRoot OutputKind = "local_root"
	// OutputKindSkill is a per-skill file (.claude/skills/{id}/SKILL.md).
	OutputKindSkill OutputKind = "skill"
	// OutputKindAgent is a per-agent file (.claude/agents/{id}.md).
	OutputKindAgent OutputKind = "agent"
	// OutputKindCommand is a per-command file, which some providers render into
	// the skills directory as a user-invocable skill.
	OutputKindCommand OutputKind = "command"
	// OutputKindSidecar is a settings/manifest document (settings.json, .mcp.json).
	OutputKindSidecar OutputKind = "sidecar"
	// OutputKindResource is a bundled skill resource (references/, scripts/, assets/).
	OutputKindResource OutputKind = "resource"
	// OutputKindRuleFile is one rule or context file emitted into a provider's
	// rules directory (.cursor/rules, .windsurf/rules, .continue/rules,
	// .clinerules). These providers apply such files as project instructions,
	// which makes them the rules-inline equivalent for a directory-shaped
	// provider.
	OutputKindRuleFile OutputKind = "rule_file"
	// OutputKindDirectory is an IsDir marker, which carries no content.
	OutputKindDirectory OutputKind = "directory"
)

// PartKind labels one addressable slice of a rendered output — a section of a
// root instructions file, or one field of a per-item file. Splitting output this
// way is the whole point of the cost report: a skill's description and its body
// are not loaded at the same time, and reporting a single per-file number would
// misattribute the cost.
type PartKind string

// Part kinds for the root instructions file, one per root.sections entry in the
// provider DSL.
const (
	PartKindRootHeader       PartKind = "header"
	PartKindRootTitle        PartKind = "title"
	PartKindRootDescription  PartKind = "description"
	PartKindRootRule         PartKind = "rule"
	PartKindRootContext      PartKind = "context"
	PartKindRootAgentsRoster PartKind = "agents_delegation"
)

// Part kinds for a per-item (skill / agent / command) file.
const (
	// PartKindItemName is the item's name string, which is what a harness puts
	// in front of the model when it lists available skills.
	PartKindItemName PartKind = "name"
	// PartKindItemDescription is the frontmatter description value.
	PartKindItemDescription PartKind = "description"
	// PartKindItemFrontmatter is the rest of the frontmatter block — everything
	// except name and description, which are reported separately.
	PartKindItemFrontmatter PartKind = "frontmatter"
	// PartKindItemBody is the authored body of the item.
	PartKindItemBody PartKind = "body"
	// PartKindItemResourceIndex is the generated index of bundled resources.
	PartKindItemResourceIndex PartKind = "resource_index"
	// PartKindItemTargetedRules is the rules block appended to items that rules
	// explicitly target.
	PartKindItemTargetedRules PartKind = "targeted_rules"
	// PartKindItemTargetedContext is the context block appended to items that
	// context files explicitly target.
	PartKindItemTargetedContext PartKind = "targeted_context"
	// PartKindSidecarDocument is a whole sidecar document.
	PartKindSidecarDocument PartKind = "document"
)

// OutputPart is one measured slice of a rendered output.
type OutputPart struct {
	Kind PartKind
	// Label names the slice: a rule name, a context file name, a field name.
	Label string
	// SourcePath is the authored file the slice came from, when one exists. Used
	// to attribute a slice back to the domain that owns it.
	SourcePath string
	// Content is the rendered text, exactly as it was written into the output.
	Content string
}

// OutputAnalysis records the classification and the per-part content of one
// generated output.
type OutputAnalysis struct {
	// Path is the absolute output path, matching OutputFile.Path.
	Path string
	// Preset is the provider that rendered the output.
	Preset string
	// Scope is the scope subdirectory the output belongs to, or "" for the
	// repository root. Scoped outputs are loaded only when the agent is working
	// inside that subtree, so a report must not add them to a root total.
	Scope string
	Kind  OutputKind
	// ItemID is the skill/agent/command identifier, or the sidecar kind.
	ItemID string
	// SourcePath is the authored file this output was rendered from.
	SourcePath string
	Parts      []OutputPart
}

// AddPart appends a measured slice. Safe to call on a nil receiver so renderers
// can record unconditionally without branching on whether analysis is enabled.
func (a *OutputAnalysis) AddPart(kind PartKind, label, sourcePath, content string) {
	if a == nil || content == "" {
		return
	}
	a.Parts = append(a.Parts, OutputPart{
		Kind:       kind,
		Label:      label,
		SourcePath: sourcePath,
		Content:    content,
	})
}

// Enabled reports whether this analysis is live.
func (a *OutputAnalysis) Enabled() bool { return a != nil }

// AnalysisCollector accumulates per-output classification and per-section
// content while presets render.
//
// It is nil on the normal generate path, and every method tolerates a nil
// receiver, so generation pays nothing for a facility only the cost report uses.
// Generation is single-goroutine per config, so the collector is not
// synchronized; a fresh collector belongs to a single Generator run.
type AnalysisCollector struct {
	byPath map[string]*OutputAnalysis
	order  []string
	scope  string
}

// NewAnalysisCollector returns an empty, enabled collector.
func NewAnalysisCollector() *AnalysisCollector {
	return &AnalysisCollector{byPath: make(map[string]*OutputAnalysis)}
}

// EnterScope stamps every subsequently begun analysis with the given scope
// label. Pass "" to return to the repository root.
func (c *AnalysisCollector) EnterScope(scope string) {
	if c == nil {
		return
	}
	c.scope = scope
}

// Begin registers an output and returns the handle renderers record parts
// against. Returns nil when the collector is nil, which AddPart tolerates.
// Re-registering a path returns the existing handle so two presets writing the
// same file do not produce two entries — matching flattenPresetOutputs, which
// keeps one OutputFile per path.
func (c *AnalysisCollector) Begin(path, preset string, kind OutputKind, itemID, sourcePath string) *OutputAnalysis {
	if c == nil {
		return nil
	}
	if existing, ok := c.byPath[path]; ok {
		return existing
	}
	analysis := &OutputAnalysis{
		Path:       path,
		Preset:     preset,
		Scope:      c.scope,
		Kind:       kind,
		ItemID:     itemID,
		SourcePath: sourcePath,
	}
	c.byPath[path] = analysis
	c.order = append(c.order, path)
	return analysis
}

// Attribute fills in the outputs a renderer did not claim. Preset name comes
// from the caller, which always knows it; kind is inferred from the path, which
// is a documented approximation — only the DSL-backed providers report an exact
// kind and a per-section breakdown.
func (c *AnalysisCollector) Attribute(preset, baseDir string, outputs []OutputFile) {
	if c == nil {
		return
	}
	for i := range outputs {
		output := &outputs[i]
		if output.IsDir {
			continue
		}
		if _, ok := c.byPath[output.Path]; ok {
			continue
		}
		kind := InferOutputKind(output.Path, baseDir)
		if output.LocalOnly {
			kind = OutputKindLocalRoot
		}
		// Raw outputs (skill scripts, bundled assets) carry their payload in
		// RawContent, not Content. Reading the wrong field would leave them with
		// no recorded part and make their whole size look like unexplained
		// per-file overhead.
		content := output.Content
		if output.RawContent != nil {
			content = string(output.RawContent)
		}
		analysis := c.Begin(output.Path, preset, kind, "", "")
		analysis.AddPart(partKindFor(kind), filepath.Base(output.Path), "", content)
	}
}

// Get returns the analysis for a path, or nil.
func (c *AnalysisCollector) Get(path string) *OutputAnalysis {
	if c == nil {
		return nil
	}
	return c.byPath[path]
}

// Analyses returns every recorded analysis in registration order.
func (c *AnalysisCollector) Analyses() []*OutputAnalysis {
	if c == nil {
		return nil
	}
	result := make([]*OutputAnalysis, 0, len(c.order))
	for _, path := range c.order {
		result = append(result, c.byPath[path])
	}
	return result
}

func partKindFor(kind OutputKind) PartKind {
	switch kind {
	case OutputKindRoot, OutputKindLocalRoot, OutputKindRuleFile:
		// A root or rule file from a non-DSL provider is measured whole: those
		// providers build their markdown with bespoke Go and expose no section
		// seam to record against.
		return PartKindRootHeader
	case OutputKindSidecar:
		return PartKindSidecarDocument
	default:
		return PartKindItemBody
	}
}

// InferOutputKind classifies an output from its path. Used only for outputs the
// rendering layer did not classify itself — the eight hand-written preset
// generators in internal/generator/presets, plus scoped and machine-local
// variants. Returns OutputKindUnknown rather than guessing when no rule matches,
// so a report can say "ai-rulez does not know how this loads".
func InferOutputKind(path, baseDir string) OutputKind {
	rel := path
	if baseDir != "" {
		if candidate, err := filepath.Rel(baseDir, path); err == nil {
			rel = candidate
		}
	}
	rel = filepath.ToSlash(rel)
	segments := strings.Split(rel, "/")

	if len(segments) == 1 {
		if isMarkdownPath(rel) {
			return OutputKindRoot
		}
		if isDocumentPath(rel) {
			return OutputKindSidecar
		}
		return OutputKindUnknown
	}

	if InRulesDir(rel) && isMarkdownPath(rel) {
		return OutputKindRuleFile
	}

	for index, segment := range segments[:len(segments)-1] {
		switch segment {
		case skillsDir:
			if isResourceSegment(segments[index+1:]) {
				return OutputKindResource
			}
			return OutputKindSkill
		case agentsDir:
			return OutputKindAgent
		case commandsDir, promptsDir:
			return OutputKindCommand
		case rulesDir, clineRulesDir:
			return OutputKindRuleFile
		}
	}

	if isDocumentPath(rel) {
		return OutputKindSidecar
	}
	return OutputKindUnknown
}

// isResourceSegment reports whether the remaining path segments below a skill
// directory address a bundled resource rather than the skill document itself.
func isResourceSegment(segments []string) bool {
	for _, segment := range segments {
		switch segment {
		case "references", "scripts", "assets":
			return true
		}
	}
	return false
}

func isMarkdownPath(path string) bool {
	return slices.Contains(markdownExtensions, strings.ToLower(filepath.Ext(path)))
}

func isDocumentPath(path string) bool {
	return slices.Contains(documentExtensions, strings.ToLower(filepath.Ext(path)))
}
