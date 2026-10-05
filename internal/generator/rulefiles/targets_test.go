package rulefiles

import (
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTargetsAllow(t *testing.T) {
	claude := Target{Preset: "claude", Dir: ".claude/rules", RootFile: "CLAUDE.md"}
	cursor := Target{Preset: "cursor", Dir: ".cursor/rules"}
	amp := Target{Preset: "amp", RootFile: "AGENTS.md"}
	antigravity := Target{Preset: "antigravity", Dir: ".agents/rules", RootFile: "GEMINI.md"}
	junie := Target{Preset: "junie", Dir: ".junie/rules", RootFile: "AGENTS.md"}
	copilot := Target{Preset: "copilot", Dir: ".github/instructions", RootFile: ".github/copilot-instructions.md"}
	tests := []struct {
		name    string
		targets []string
		target  Target
		relPath string
		want    bool
	}{
		{"no targets", nil, cursor, ".cursor/rules/x.mdc", true},
		{"empty targets inline", []string{}, claude, "", true},
		{"preset name", []string{"cursor"}, cursor, ".cursor/rules/x.mdc", true},
		{"preset name case-insensitive", []string{"Cursor"}, cursor, ".cursor/rules/x.mdc", true},
		{"other preset name", []string{"cursor"}, claude, ".claude/rules/x.md", false},
		{"root file selects rule files", []string{"CLAUDE.md"}, claude, ".claude/rules/x.md", true},
		{"root file selects inline", []string{"CLAUDE.md"}, claude, "", true},
		{"root file base name selects inline", []string{"copilot-instructions.md"}, copilot, "", true},
		{"root file base name selects rule files", []string{"copilot-instructions.md"}, copilot,
			".github/instructions/x.instructions.md", true},
		{"junie root base name", []string{"AGENTS.md"}, junie, ".junie/rules/x.md", true},
		{"root file base name of other preset", []string{"AGENTS.md"}, claude, "", false},
		{"root file of other preset", []string{"CLAUDE.md"}, cursor, ".cursor/rules/x.mdc", false},
		{"exact path", []string{".cursor/rules/x.mdc"}, cursor, ".cursor/rules/x.mdc", true},
		{"exact path with ./", []string{"./.cursor/rules/x.mdc"}, cursor, ".cursor/rules/x.mdc", true},
		{"exact path other file", []string{".cursor/rules/y.mdc"}, cursor, ".cursor/rules/x.mdc", false},
		{"base name", []string{"x.mdc"}, cursor, ".cursor/rules/x.mdc", true},
		{"directory prefix", []string{".cursor/rules/"}, cursor, ".cursor/rules/x.mdc", true},
		{"directory prefix nested", []string{".cursor/rules/"}, cursor, ".cursor/rules/sub/x.mdc", true},
		{"directory prefix without slash is a path", []string{".cursor/rules"}, cursor, ".cursor/rules/x.mdc", false},
		{"directory prefix elsewhere", []string{".cursor/rules/"}, claude, ".claude/rules/x.md", false},
		{"directory prefix never matches inline", []string{".cursor/rules/"}, cursor, "", false},
		{"directory glob star", []string{".cursor/rules/*"}, cursor, ".cursor/rules/sub/x.mdc", true},
		{"directory glob double star", []string{".cursor/**"}, cursor, ".cursor/rules/x.mdc", true},
		{"directory glob elsewhere", []string{".devin/*"}, cursor, ".cursor/rules/x.mdc", false},
		{"shared AGENTS.md selected by a sibling preset", []string{"codex"}, amp, "", true},
		{"shared AGENTS.md selected by its file", []string{"agents.md"}, amp, "", true},
		{"shared AGENTS.md not selected by other preset", []string{"claude"}, amp, "", false},
		{"shared GEMINI.md selected by gemini", []string{"gemini"}, antigravity, "", true},
		{"antigravity files are not selected by gemini", []string{"gemini"}, antigravity, ".agents/rules/x.md", false},
		{"windows path target", []string{`.cursor\rules\x.mdc`}, cursor, ".cursor/rules/x.mdc", true},
		{"windows directory target", []string{`.cursor\rules\`}, cursor, ".cursor/rules/x.mdc", true},
		{"dot slash directory", []string{"./.cursor/rules/"}, cursor, ".cursor/rules/x.mdc", true},
		{"path is case-insensitive", []string{".Cursor/Rules/X.mdc"}, cursor, ".cursor/rules/x.mdc", true},
		{"bare star", []string{"*"}, cursor, ".cursor/rules/x.mdc", true},
		{"junie dir prefix", []string{".junie/"}, junie, ".junie/rules/x.md", true},
		{"junie dir prefix does not select the inline root", []string{".junie/"}, junie, "", false},
		{"copilot dir prefix", []string{".github/"}, copilot, ".github/instructions/x.instructions.md", true},
		{"copilot dir prefix selects root", []string{".github/"}, copilot, "", true},
		{"skill-only target excludes root", []string{".claude/skills/*/SKILL.md"}, claude, "", false},
		{"skill-only target excludes rule files", []string{".claude/skills/*/SKILL.md"}, claude, ".claude/rules/x.md", false},
		{"glob on path", []string{".cursor/rules/*.mdc"}, cursor, ".cursor/rules/x.mdc", true},
		{"glob on base name", []string{"*.mdc"}, cursor, ".cursor/rules/x.mdc", true},
		{"glob miss", []string{"*.md"}, cursor, ".cursor/rules/x.mdc", false},
		{"glob matches root file inline", []string{"*.md"}, claude, "", true},
		{"invalid glob", []string{"[x"}, cursor, ".cursor/rules/x.mdc", false},
		{"any target matches", []string{"devin", "cursor"}, cursor, ".cursor/rules/x.mdc", true},
		{"blank target ignored", []string{" "}, cursor, ".cursor/rules/x.mdc", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := TargetsAllow(tt.targets, tt.target, tt.relPath)

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPlan_TargetsFilterFilesAndInline(t *testing.T) {
	tg := &Target{Preset: "claude", Dir: ".claude/rules", Ext: ".md", RootFile: "CLAUDE.md"}
	mk := func(name string, targets ...string) config.ContentFile {
		return config.ContentFile{Name: name, Path: "r/" + name + ".md", Metadata: &config.Metadata{Targets: targets}}
	}
	rules := []config.ContentFile{
		mk("free"), mk("mine", "CLAUDE.md"), mk("other", "cursor"), mk("dir", ".claude/rules/"),
	}
	ctxs := []config.ContentFile{mk("cfree"), mk("cother", "cursor")}
	tests := []struct {
		name      string
		routing   Routing
		files     []string
		inlineR   []string
		inlineCtx []string
	}{
		{"all", RoutingAll, []string{"free.md", "mine.md", "dir.md"}, nil, []string{"cfree"}},
		{"everything", RoutingEverything, []string{"free.md", "mine.md", "dir.md", "context-cfree.md"}, nil, nil},
		{"scoped only", RoutingScopedOnly, []string{"dir.md"}, []string{"free", "mine"}, []string{"cfree"}},
		{"none", RoutingNone, nil, []string{"free", "mine"}, []string{"cfree"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			files, ir, ic, err := Plan(rules, ctxs, tg, tt.routing, ScopeInfo{}, nil)

			// Assert
			require.NoError(t, err)
			var names []string
			for _, f := range files {
				names = append(names, FileName(*tg, f))
			}
			assert.Equal(t, tt.files, names)
			assert.Equal(t, tt.inlineR, contentNames(ir))
			assert.Equal(t, tt.inlineCtx, contentNames(ic))
		})
	}
}

func TestPlan_NilTargetAndRoutingNone(t *testing.T) {
	rules := []config.ContentFile{
		{Name: "free", Path: "r/free.md"},
		{Name: "other", Path: "r/other.md", Metadata: &config.Metadata{Targets: []string{"cursor"}}},
		{Name: "dir", Path: "r/dir.md", Metadata: &config.Metadata{Targets: []string{".claude/rules/"}}},
	}
	tg := &Target{Preset: "claude", Dir: ".claude/rules", Ext: ".md", RootFile: "CLAUDE.md"}
	tests := []struct {
		name    string
		target  *Target
		routing Routing
		inline  []string
	}{
		{"nil target keeps everything", nil, RoutingAll, []string{"free", "other", "dir"}},
		{"no routing filters inline by target", tg, RoutingNone, []string{"free"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files, ir, _, err := Plan(rules, nil, tt.target, tt.routing, ScopeInfo{}, nil)

			require.NoError(t, err)
			assert.Empty(t, files)
			assert.Equal(t, tt.inline, contentNames(ir))
		})
	}
}

func TestPlan_SharedRootKeepsSiblingTargets(t *testing.T) {
	tg := &Target{Preset: "antigravity", Dir: ".agents/rules", Ext: ".md", RootFile: "GEMINI.md"}
	rules := []config.ContentFile{
		{Name: "scoped", Path: "r/s.md", Metadata: &config.Metadata{Targets: []string{"gemini"}, Globs: []string{"src/**"}}},
	}

	files, ir, _, err := Plan(rules, nil, tg, RoutingAll, ScopeInfo{}, nil)

	require.NoError(t, err)
	assert.Empty(t, files)
	assert.Equal(t, []string{"scoped"}, contentNames(ir), "a sibling preset's target keeps it in the shared root file")
}

func TestTargetsAllow_RootAliases(t *testing.T) {
	shared := Target{
		Preset: "codex", RootFile: "AGENTS.md", Owners: []string{"codex", "claude"},
		RootAliases: []string{"CLAUDE.md", "GEMINI.md", ".hermes.md", ".github/copilot-instructions.md"},
	}
	tests := []struct {
		target string
		want   bool
	}{
		{"AGENTS.md", true},
		{"CLAUDE.md", true},
		{"claude.md", true},
		{"GEMINI.md", true},
		{".hermes.md", true},
		{".github/copilot-instructions.md", true},
		{"copilot-instructions.md", true},
		{"OTHER.md", false},
		{".cursor/rules/x.mdc", false},
	}
	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			assert.Equal(t, tt.want, TargetsAllow([]string{tt.target}, shared, ""))
		})
	}
	t.Run("aliases do not select rule files", func(t *testing.T) {
		assert.False(t, TargetsAllow([]string{"GEMINI.md"}, shared, ".cursor/rules/x.mdc"))
	})
	t.Run("no aliases", func(t *testing.T) {
		assert.False(t, TargetsAllow([]string{"GEMINI.md"}, Target{Preset: "codex", RootFile: "AGENTS.md"}, ""))
	})
}
