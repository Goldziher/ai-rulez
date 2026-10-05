package importer

import (
	"sort"
	"sync"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/presets"

	// Register every preset (Go and declarative) so their layouts can be read.
	_ "github.com/Goldziher/ai-rulez/internal/generator"
)

// nativeSource is one place a tool keeps instructions: a root file or a
// directory, and the content kind its files map to.
type nativeSource struct {
	Path    string
	Kind    Kind
	Presets []string
}

// extraSources are layouts no preset writes, or aliases of one that it does
// (Windsurf rules are read by the devin preset, Roo by zoocode, the legacy
// single-file rule formats by cursor and devin).
var extraSources = []nativeSource{
	{Path: ".cursorrules", Kind: KindContext, Presets: []string{"cursor"}},
	{Path: ".windsurfrules", Kind: KindContext, Presets: []string{"devin"}},
	{Path: ".windsurf/rules", Kind: KindRule, Presets: []string{"devin"}},
	{Path: ".windsurf/workflows", Kind: KindCommand, Presets: []string{"devin"}},
	{Path: ".windsurf/skills", Kind: KindSkill, Presets: []string{"devin"}},
	{Path: ".roo/rules", Kind: KindRule, Presets: []string{"zoocode"}},
	{Path: ".roo/commands", Kind: KindCommand, Presets: []string{"zoocode"}},
	{Path: ".roo/skills", Kind: KindSkill, Presets: []string{"zoocode"}},
	{Path: ".claude/commands", Kind: KindCommand, Presets: []string{"claude"}},
	{Path: ".junie/guidelines.md", Kind: KindContext, Presets: []string{"junie"}},
	{Path: ".xum/skills", Kind: KindSkill, Presets: []string{"xum"}},
	{Path: ".xum/agents", Kind: KindAgent, Presets: []string{"xum"}},
	{Path: ".opencode/command", Kind: KindCommand, Presets: []string{"opencode"}},
	{Path: ".github/prompts", Kind: KindCommand, Presets: []string{"copilot"}},
}

// preferredPresets decides which preset a path implies when several write it.
var preferredPresets = map[string]string{
	".github/copilot-instructions.md": "copilot",
	".github/instructions":            "copilot",
	".github/agents":                  "copilot",
	".github/skills":                  "copilot",
	".github/prompts":                 "copilot",
	"GEMINI.md":                       "gemini",
}

// skippedPresets have a layout that is not instructions-shaped (takt writes
// workflow facets), so reading it as content would be wrong.
var skippedPresets = map[string]bool{"takt": true}

var (
	sourcesOnce sync.Once
	sourcesList []nativeSource
)

// nativeSources returns every known native source, derived from the project
// layout of every registered preset plus the extra table, sorted by path.
func nativeSources() []nativeSource {
	sourcesOnce.Do(func() {
		type key struct {
			path string
			kind Kind
		}
		claims := map[key]map[string]bool{}
		claim := func(path string, kind Kind, preset string) {
			if path == "" {
				return
			}
			k := key{path, kind}
			if claims[k] == nil {
				claims[k] = map[string]bool{}
			}
			claims[k][preset] = true
		}
		for _, name := range config.IndividualPresetNames() {
			if skippedPresets[name] {
				continue
			}
			gen, err := config.GetPresetGenerator(name)
			if err != nil {
				continue
			}
			provider, ok := gen.(presets.ProjectLayoutProvider)
			if !ok {
				continue
			}
			l := provider.ProjectLayout()
			claim(l.RootFile, KindContext, name)
			claim(l.RulesDir, KindRule, name)
			claim(l.SkillsDir, KindSkill, name)
			if l.AgentsDir != l.SkillsDir {
				claim(l.AgentsDir, KindAgent, name)
			}
			if l.CommandsDir != l.SkillsDir && l.CommandsDir != l.AgentsDir {
				claim(l.CommandsDir, KindCommand, name)
			}
		}
		for _, e := range extraSources {
			for _, p := range e.Presets {
				claim(e.Path, e.Kind, p)
			}
		}
		for k, set := range claims {
			var ps []string
			for p := range set {
				ps = append(ps, p)
			}
			sort.Strings(ps)
			sourcesList = append(sourcesList, nativeSource{Path: k.path, Kind: k.kind, Presets: ps})
		}
		sort.Slice(sourcesList, func(i, j int) bool {
			if sourcesList[i].Path != sourcesList[j].Path {
				return sourcesList[i].Path < sourcesList[j].Path
			}
			return sourcesList[i].Kind < sourcesList[j].Kind
		})
	})
	return sourcesList
}

// impliedPreset returns the preset a found source implies, or "" when the path
// is shared by several presets (AGENTS.md, .agents/skills) and says nothing.
func (s nativeSource) impliedPreset() string {
	if p, ok := preferredPresets[s.Path]; ok {
		return p
	}
	if len(s.Presets) == 1 {
		return s.Presets[0]
	}
	return ""
}
