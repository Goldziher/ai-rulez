package presets

import (
	"path/filepath"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/rulefiles"
	"github.com/samber/oops"
)

// localRuleOutputs renders already planned local rule items as personal rule
// files. Local rules are not scoped, so item IDs never contain a path
// separator and the files sit directly in the rules folder.
func localRuleOutputs(t rulefiles.Target, items []rulefiles.Item, baseDir string, cfg *config.Config,
) ([]config.OutputFile, error) {
	outputs := make([]config.OutputFile, 0, len(items))
	for i := range items {
		it := &items[i]
		text, notes, err := rulefiles.Render(t, *it, cfg)
		if err != nil {
			return nil, oops.With("preset", t.Preset, "rule", it.File.Name).Wrapf(err, "render local %s rule file", t.Preset)
		}
		path := filepath.Join(baseDir, filepath.FromSlash(rulefiles.LocalPath(t, it.ID)))
		for j := range notes {
			notes[j].Name += " (local)"
		}
		rulefiles.ReportNotes(path, notes)
		analysis := cfg.Analysis.Begin(path, t.Preset, config.OutputKindRuleFile, it.ID, it.File.Path)
		if analysis != nil {
			analysis.MachineLocal = true
		}
		analysis.AddPart(config.PartKindItemBody, "body", it.File.Path, text)
		outputs = append(outputs, config.OutputFile{Path: path, Content: text, LocalOnly: true})
	}
	return outputs, nil
}

// PlanLocalRules routes local rules to t's rules folder with the routing the
// preset applies to shared rules and renders the routed ones as
// "<dir>/<id>.local<ext>". Rules the routing keeps inline are returned in
// inline. The plan has its own registry: a local rule may share a name with a
// shared rule, but two local rules that map to the same file name are disambiguated like shared ones.
func PlanLocalRules(t rulefiles.Target, routing rulefiles.Routing, rules []config.ContentFile, baseDir string,
	cfg *config.Config,
) (files []config.OutputFile, inline []config.ContentFile, err error) {
	items, inline, _, err := rulefiles.Plan(rules, nil, &t, routing, rulefiles.ScopeInfo{}, rulefiles.NewRegistryFor(cfg))
	if err != nil {
		return nil, nil, oops.With("preset", t.Preset).Wrapf(err, "plan local %s rule files", t.Preset)
	}
	files, err = localRuleOutputs(t, items, baseDir, cfg)
	return files, inline, err
}

// alwaysFileLocalRules implements config.LocalRuleProvider for presets that write
// one file per rule regardless of the rules mode (cursor, windsurf, cline,
// continue-dev). A zero value plans nothing.
type alwaysFileLocalRules struct {
	target  *rulefiles.Target
	routing rulefiles.Routing
}

// LocalRuleOutputs implements config.LocalRuleProvider.
func (l alwaysFileLocalRules) LocalRuleOutputs(rules []config.ContentFile, baseDir string, cfg *config.Config,
) ([]config.OutputFile, []config.ContentFile, error) {
	if l.target == nil {
		return nil, rules, nil
	}
	return PlanLocalRules(*l.target, l.routing, rules, baseDir, cfg)
}

// LocalRuleOutputs implements config.LocalRuleProvider. Copilot routes local
// rules exactly like shared ones: rules it cannot apply automatically stay inline.
func (g *CopilotPresetGenerator) LocalRuleOutputs(rules []config.ContentFile, baseDir string, cfg *config.Config,
) ([]config.OutputFile, []config.ContentFile, error) {
	items, inline, _, err := planCopilotItems(rules, nil, cfg, rulefiles.ScopeInfo{}, rulefiles.NewRegistryFor(cfg), true)
	if err != nil {
		return nil, nil, err
	}
	files, err := localRuleOutputs(copilotRulesTarget, items, baseDir, cfg)
	return files, inline, err
}

// LocalRuleOutputs implements config.LocalRuleProvider. Antigravity keeps rules
// inline when the gemini preset shares GEMINI.md, unless the mode is explicit.
func (g *AntigravityPresetGenerator) LocalRuleOutputs(rules []config.ContentFile, baseDir string, cfg *config.Config,
) ([]config.OutputFile, []config.ContentFile, error) {
	routing, _ := antigravityRouting(cfg, func(string, ...any) {})
	return PlanLocalRules(antigravityRulesTarget, routing, rules, baseDir, cfg)
}

// LocalRootFile implements config.LocalRootProvider. Copilot has no single local
// root: its personal override is a path-specific instructions file that applies
// to every file and that the tool loads natively.
func (g *CopilotPresetGenerator) LocalRootFile() string {
	return rulefiles.LocalPath(copilotRulesTarget, localRootID)
}

// localRootID names the generated local overrides file ("ai-rulez.local<ext>").
const localRootID = "ai-rulez"

// RenderLocalRoot implements config.LocalRootRenderer: the inline local rules and
// the local context become one always-applied instructions file.
func (g *CopilotPresetGenerator) RenderLocalRoot(local *config.ContentTree, rules []config.ContentFile, baseDir string,
	cfg *config.Config,
) (config.OutputFile, error) {
	root := rulefiles.RootTarget(presetNameCopilot, copilotRulesTarget.RootFile)
	body := localSections(rulefiles.FilterInline(rules, root), rulefiles.FilterInline(allInlineContext(local), root), cfg)
	item := rulefiles.Item{
		File: config.ContentFile{Name: "Local overrides", Path: "local", Content: body},
		Kind: rulefiles.KindRule,
		ID:   localRootID,
		Activation: config.Activation{
			Mode: config.ActivationAlways, Source: config.ActivationSourceDerived,
		},
	}
	text, notes, err := rulefiles.Render(copilotRulesTarget, item, cfg)
	if err != nil {
		return config.OutputFile{}, oops.With("preset", presetNameCopilot).Wrapf(err, "render local copilot instructions")
	}
	path := filepath.Join(baseDir, filepath.FromSlash(g.LocalRootFile()))
	rulefiles.ReportNotes(path, notes)
	return config.OutputFile{Path: path, Content: text, LocalOnly: true}, nil
}

// LocalRootFile implements config.LocalRootProvider: GEMINI.md → GEMINI.local.md.
func (g *AntigravityPresetGenerator) LocalRootFile() string {
	return config.LocalVariantPath("GEMINI.md")
}
