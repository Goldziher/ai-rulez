package presets

import (
	"path/filepath"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/rulefiles"
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
		rulefiles.ReportNotes(cfg.Diag, path, notes)
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
// reservedIDs are rule ids whose files ai-rulez generates itself (the local root
// file "ai-rulez.local<ext>"): a rule with one of them is renamed.
func PlanLocalRules(t rulefiles.Target, routing rulefiles.Routing, rules []config.ContentFile, baseDir string,
	cfg *config.Config, reservedIDs ...string,
) (files []config.OutputFile, inline []config.ContentFile, err error) {
	reg := rulefiles.NewRegistryFor(cfg)
	for _, id := range reservedIDs {
		reg.Reserve(t, id)
	}
	items, inline, _, err := rulefiles.Plan(rules, nil, &t, routing, rulefiles.ScopeInfo{}, reg)
	if err != nil {
		return nil, nil, oops.With("preset", t.Preset).Wrapf(err, "plan local %s rule files", t.Preset)
	}
	files, err = localRuleOutputs(t, items, baseDir, cfg)
	return files, inline, err
}

// alwaysFileLocalRules implements config.LocalRuleProvider for presets that write
// one file per rule regardless of the rules mode (cursor, devin, cline,
// cline, devin). A zero value plans nothing.
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
	reg := rulefiles.NewRegistryFor(cfg)
	reg.Reserve(copilotRulesTarget, localRootID)
	items, inline, _, err := planCopilotItems(rules, nil, cfg, rulefiles.ScopeInfo{}, reg, true)
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
	return PlanLocalRules(antigravityRulesTarget, routing, rules, baseDir, cfg, localRootID)
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
	return renderLocalRootRuleFile(copilotRulesTarget, root, local, rules, baseDir, cfg)
}

// RenderLocalRoot implements config.LocalRootRenderer. Antigravity reads neither
// GEMINI.local.md nor any other local file, but loads every .agents/rules/*.md
// that declares a trigger, so the inline local rules and the local context become
// one always-on rule file there.
func (g *AntigravityPresetGenerator) RenderLocalRoot(local *config.ContentTree, rules []config.ContentFile, baseDir string,
	cfg *config.Config,
) (config.OutputFile, error) {
	root := rulefiles.RootTarget(presetNameAntigravity, antigravityRulesTarget.RootFile)
	return renderLocalRootRuleFile(antigravityRulesTarget, root, local, rules, baseDir, cfg)
}

// renderLocalRootRuleFile renders the local inline rules and context as the
// always-applied rule file "<dir>/ai-rulez.local<ext>" of t.
func renderLocalRootRuleFile(t rulefiles.Target, root rulefiles.Target, local *config.ContentTree,
	rules []config.ContentFile, baseDir string, cfg *config.Config,
) (config.OutputFile, error) {
	body := localSections(rulefiles.FilterInline(rules, root), rulefiles.FilterInline(allInlineContext(local), root), cfg)
	item := rulefiles.Item{
		File: config.ContentFile{Name: "Local overrides", Path: "local", Content: body},
		Kind: rulefiles.KindRule,
		ID:   localRootID,
		Activation: config.Activation{
			Mode: config.ActivationAlways, Source: config.ActivationSourceDerived,
		},
	}
	text, notes, err := rulefiles.Render(t, item, cfg)
	if err != nil {
		return config.OutputFile{}, oops.With("preset", t.Preset).Wrapf(err, "render local %s rule file", t.Preset)
	}
	path := filepath.Join(baseDir, filepath.FromSlash(rulefiles.LocalPath(t, localRootID)))
	rulefiles.ReportNotes(cfg.Diag, path, notes)
	return config.OutputFile{Path: path, Content: text, LocalOnly: true}, nil
}
