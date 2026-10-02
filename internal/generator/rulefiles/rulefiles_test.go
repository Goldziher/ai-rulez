package rulefiles

import (
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func item(name string, mode config.ActivationMode, desc string, globs ...string) Item {
	return Item{
		File:       config.ContentFile{Name: name, Path: "/abs/proj/.ai-rulez/rules/" + name + ".md", Content: "Body.", Metadata: &config.Metadata{Priority: "high"}},
		Kind:       KindRule,
		ID:         ID(name),
		Activation: config.Activation{Mode: mode, Globs: globs, Description: desc},
	}
}

func TestFrontmatter_Dialects(t *testing.T) {
	const (
		always = config.ActivationAlways
		glob   = config.ActivationGlob
		auto   = config.ActivationAuto
		manual = config.ActivationManual
	)
	globs := []string{"*.{ts,tsx}", "src/**"}
	type m = map[string]any
	fallback := func(d, mode string) []string {
		return []string{`rule "r": activation ` + mode + ` not supported by ` + d + `; loaded always`}
	}
	noApplyTo := func(mode string) []string {
		return []string{`rule "r": ` + mode + ` activation is not applied automatically on GitHub.com (no applyTo)`}
	}
	tests := []struct {
		name    string
		dialect Dialect
		mode    config.ActivationMode
		want    m
		notes   []string
	}{
		{"claude always", DialectClaude, always, nil, nil},
		{"claude glob", DialectClaude, glob, m{"paths": globs}, nil},
		{"claude auto", DialectClaude, auto, nil, fallback("claude", "auto")},
		{"claude manual", DialectClaude, manual, nil, fallback("claude", "manual")},

		{"cursor always", DialectCursor, always, m{"alwaysApply": true, "description": "d"}, nil},
		{"cursor glob", DialectCursor, glob, m{"globs": "*.ts,*.tsx,src/**", "alwaysApply": false, "description": "d"}, nil},
		{"cursor auto", DialectCursor, auto, m{"description": "d", "alwaysApply": false}, nil},
		{"cursor manual", DialectCursor, manual, m{"alwaysApply": false}, nil},

		{"trigger always", DialectTrigger, always, m{"trigger": "always_on"}, nil},
		{"trigger glob", DialectTrigger, glob, m{"trigger": "glob", "globs": "*.ts,*.tsx,src/**"}, nil},
		{"trigger auto", DialectTrigger, auto, m{"trigger": "model_decision", "description": "d"}, nil},
		{"trigger manual", DialectTrigger, manual, m{"trigger": "manual"}, nil},

		{"copilot always", DialectCopilot, always, m{"applyTo": "**"}, nil},
		{"copilot glob", DialectCopilot, glob, m{"applyTo": "*.ts,*.tsx,src/**"}, nil},
		{"copilot auto", DialectCopilot, auto, m{"description": "d"}, noApplyTo("auto")},
		{"copilot manual", DialectCopilot, manual, nil, noApplyTo("manual")},

		{"cline always", DialectCline, always, nil, nil},
		{"cline glob", DialectCline, glob, m{"paths": globs}, nil},
		{"cline auto", DialectCline, auto, nil, fallback("cline", "auto")},
		{"cline manual", DialectCline, manual, nil, fallback("cline", "manual")},

		{"continue always", DialectContinue, always, m{"name": "r", "alwaysApply": true}, nil},
		{"continue glob", DialectContinue, glob, m{"name": "r", "globs": globs, "alwaysApply": false}, nil},
		{"continue auto", DialectContinue, auto, m{"name": "r", "description": "d"}, nil},
		{"continue manual", DialectContinue, manual, m{"name": "r", "alwaysApply": false}, nil},

		{"junie always", DialectJunie, always, nil, nil},
		{"junie glob", DialectJunie, glob, nil, nil},
		{"junie auto", DialectJunie, auto, nil, fallback("junie", "auto")},
		{"junie manual", DialectJunie, manual, nil, fallback("junie", "manual")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			it := item("r", tt.mode, "d", globs...)

			fm, notes := Frontmatter(tt.dialect, it)

			assert.Equal(t, tt.want, map[string]any(fm))
			assert.Equal(t, tt.notes, noteTexts(notes))
		})
	}
}

func noteTexts(notes []Note) []string {
	var out []string
	for _, n := range notes {
		out = append(out, n.Text)
	}
	return out
}

func TestExpandBraces(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"*.go", []string{"*.go"}},
		{"*.{ts,tsx}", []string{"*.ts", "*.tsx"}},
		{"{a,b}/{c,d}", []string{"a/c", "a/d", "b/c", "b/d"}},
		{"x.{a,{b,c}}", []string{"x.a", "x.b", "x.c"}},
		{"src/[a,b].ts", []string{"src/[a,b].ts"}},
		{`a\,b`, []string{`a\,b`}},
		{`{a\,b,c}`, []string{`a\,b`, "c"}},
		{"{unclosed,x", []string{"{unclosed,x"}},
		{"[{]a,b}", []string{"[{]a,b}"}},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, tt.want, ExpandBraces(tt.in))
		})
	}
}

func testCfg() *config.Config {
	return &config.Config{ConfigDir: "/abs/proj/.ai-rulez", ConfigDirName: ".ai-rulez"}
}

func claudeTarget() Target {
	return Target{Preset: "claude", Dir: ".claude/rules", Ext: ".md", Dialect: DialectClaude, Banner: true}
}

func TestRender_FrontmatterFirstThenBanner(t *testing.T) {
	it := item("Go Style", config.ActivationGlob, "", "**/*.go")

	out, notes, err := Render(claudeTarget(), it, testCfg())

	require.NoError(t, err)
	assert.Empty(t, notes)
	want := "---\npaths:\n    - '**/*.go'\n---\n<!--\nGenerated by ai-rulez from .ai-rulez/rules/Go Style.md. Edit the source, not this file.\n-->\n\n" +
		"# Go Style\n\n**Priority:** high\n\nBody.\n"
	assert.Equal(t, want, out)
}

func TestRender_AlwaysOnClaudeHasBannerOnly(t *testing.T) {
	out, _, err := Render(claudeTarget(), item("r", config.ActivationAlways, ""), testCfg())

	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(out, "<!--\nGenerated by ai-rulez from .ai-rulez/rules/r.md."), out)
	assert.Contains(t, out, "\n-->\n\n# r\n")
}

func TestRender_NoBannerForMDC(t *testing.T) {
	tg := Target{Preset: "cursor", Dir: ".cursor/rules", Ext: ".mdc", Dialect: DialectCursor}

	out, _, err := Render(tg, item("r", config.ActivationAlways, ""), &config.Config{})

	require.NoError(t, err)
	assert.Equal(t, "---\nalwaysApply: true\n---\n# r\n\n**Priority:** high\n\nBody.\n", out)
}

func TestRender_ErrorsWithoutFrontmatterAndBanner(t *testing.T) {
	tg := Target{Preset: "claude", Ext: ".md", Dialect: DialectClaude}

	_, _, err := Render(tg, item("r", config.ActivationAlways, ""), &config.Config{})

	require.Error(t, err)
}

func TestRender_CursorManualHasFrontmatter(t *testing.T) {
	tg := Target{Preset: "cursor", Ext: ".mdc", Dialect: DialectCursor}

	out, _, err := Render(tg, item("r", config.ActivationManual, "d"), &config.Config{})

	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(out, "---\nalwaysApply: false\n---\n"), out)
}

func TestSourceLabel(t *testing.T) {
	tests := []struct {
		name string
		path string
		cfg  *config.Config
		want string
	}{
		{"ai-rulez dir", "/p/.ai-rulez/rules/a.md", &config.Config{ConfigDir: "/p/.ai-rulez", ConfigDirName: ".ai-rulez"}, ".ai-rulez/rules/a.md"},
		{"dot config", "/p/.config/ai-rulez/rules/a.md", &config.Config{ConfigDir: "/p/.config/ai-rulez", ConfigDirName: ".config/ai-rulez"}, ".config/ai-rulez/rules/a.md"},
		{"outside", "/elsewhere/a.md", &config.Config{ConfigDir: "/p/.ai-rulez", ConfigDirName: ".ai-rulez"}, "a.md"},
		{"sibling prefix", "/p/.ai-rulez-x/a.md", &config.Config{ConfigDir: "/p/.ai-rulez", ConfigDirName: ".ai-rulez"}, "a.md"},
		{"nil cfg", "/p/.ai-rulez/rules/a.md", nil, "a.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, sourceLabel(tt.path, tt.cfg))
		})
	}
}

func TestRender_BannerEscapesPath(t *testing.T) {
	it := item("r", config.ActivationAlways, "")
	it.File.Path = "/x/a-->b\nc.md"

	out, _, err := Render(claudeTarget(), it, &config.Config{})

	require.NoError(t, err)
	assert.Contains(t, out, "from a- ->b c.md.")
	assert.Equal(t, 1, strings.Count(out, "-->"))
}

func TestRender_ValidYAMLFrontmatter(t *testing.T) {
	desc := "Use: when\nneeded # really"
	tests := []struct {
		name string
		tg   Target
		it   Item
		want map[string]any
	}{
		{"claude star", Target{Ext: ".md", Dialect: DialectClaude, Banner: true},
			item("r", config.ActivationGlob, "", "*.go", "{a,b}/**", "!vendor/**"),
			map[string]any{"paths": []any{"*.go", "{a,b}/**", "!vendor/**"}}},
		{"cursor brace", Target{Ext: ".mdc", Dialect: DialectCursor},
			item("r", config.ActivationGlob, "", "*.{ts,tsx}"),
			map[string]any{"globs": "*.ts,*.tsx", "alwaysApply": false}},
		{"cursor description", Target{Ext: ".mdc", Dialect: DialectCursor},
			item("r", config.ActivationAuto, desc),
			map[string]any{"description": desc, "alwaysApply": false}},
		{"copilot brace", Target{Ext: ".instructions.md", Dialect: DialectCopilot, Banner: true},
			item("r", config.ActivationGlob, "", "{src,lib}/**"),
			map[string]any{"applyTo": "src/**,lib/**"}},
		{"trigger negated", Target{Ext: ".md", Dialect: DialectTrigger, Banner: true},
			item("r", config.ActivationGlob, "", "*.md", "!x/*.md"),
			map[string]any{"trigger": "glob", "globs": "*.md,!x/*.md"}},
		{"continue", Target{Ext: ".md", Dialect: DialectContinue, Banner: true},
			item("r", config.ActivationGlob, "", "{a}/*.go"),
			map[string]any{"name": "r", "globs": []any{"{a}/*.go"}, "alwaysApply": false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, _, err := Render(tt.tg, tt.it, &config.Config{})

			require.NoError(t, err)
			require.True(t, strings.HasPrefix(out, "---\n"))
			end := strings.Index(out[4:], "\n---\n")
			require.GreaterOrEqual(t, end, 0)
			var parsed map[string]any
			require.NoError(t, yaml.Unmarshal([]byte(out[4:4+end+1]), &parsed))
			assert.Equal(t, tt.want, parsed)
		})
	}
}

func TestRender_MaxCharsCountsRunes(t *testing.T) {
	tg := claudeTarget()
	it := item("r", config.ActivationAlways, "")
	it.File.Content = strings.Repeat("é", 60)
	out, _, err := Render(tg, it, &config.Config{})
	require.NoError(t, err)
	tg.MaxChars = len([]rune(out))

	_, notes, err := Render(tg, it, &config.Config{})

	require.NoError(t, err)
	assert.Empty(t, notes)
}

func TestRender_Compact(t *testing.T) {
	compact := true
	out, _, err := Render(claudeTarget(), item("r", config.ActivationAlways, ""), &config.Config{Compact: &compact})

	require.NoError(t, err)
	assert.NotContains(t, out, "Priority")
}

func TestRender_JunieAppliesTo(t *testing.T) {
	tg := Target{Preset: "junie", Ext: ".md", Dialect: DialectJunie, Banner: true}

	out, _, err := Render(tg, item("r", config.ActivationGlob, "", "a/**", "b/**"), &config.Config{})

	require.NoError(t, err)
	assert.Contains(t, out, "_Applies to: a/**, b/**_\n")
	assert.True(t, strings.HasPrefix(out, "<!--"))
}

func TestRender_MaxCharsNote(t *testing.T) {
	tests := []struct {
		name string
		max  int
		note bool
	}{
		{"under limit", 10000, false},
		{"over limit", 20, true},
		{"unlimited", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tg := claudeTarget()
			tg.MaxChars = tt.max

			out, notes, err := Render(tg, item("r", config.ActivationAlways, ""), &config.Config{})

			require.NoError(t, err)
			assert.Contains(t, out, "Body.")
			if tt.note {
				require.Len(t, notes, 1)
				assert.Contains(t, notes[0].Text, "exceeds")
			} else {
				assert.Empty(t, notes)
			}
		})
	}
}

func cf(name, path string, globs ...string) config.ContentFile {
	return config.ContentFile{Name: name, Path: path, Metadata: &config.Metadata{Globs: globs}}
}

func TestPlan_Routing(t *testing.T) {
	tg := &Target{Preset: "claude", Dir: ".claude/rules", Ext: ".md"}
	rules := []config.ContentFile{cf("global", "r/global.md"), cf("go", "r/go.md", "**/*.go")}
	ctxs := []config.ContentFile{cf("plain", "c/plain.md"), cf("scoped", "c/scoped.md", "docs/**")}
	tests := []struct {
		name      string
		target    *Target
		routing   Routing
		files     []string
		inlineR   []string
		inlineCtx []string
	}{
		{"all", tg, RoutingAll, []string{"global.md", "go.md", "context-scoped.md"}, nil, []string{"plain"}},
		{"scoped only", tg, RoutingScopedOnly, []string{"go.md", "context-scoped.md"}, []string{"global"}, []string{"plain"}},
		{"none", tg, RoutingNone, nil, []string{"global", "go"}, []string{"plain", "scoped"}},
		{"nil target", nil, RoutingAll, nil, []string{"global", "go"}, []string{"plain", "scoped"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files, ir, ic, err := Plan(rules, ctxs, tt.target, tt.routing, ScopeInfo{}, nil)

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

func contentNames(in []config.ContentFile) []string {
	var out []string
	for _, c := range in {
		out = append(out, c.Name)
	}
	return out
}

func TestPlan_IDCollision(t *testing.T) {
	tg := &Target{Ext: ".md"}
	rules := []config.ContentFile{cf("Go Style", "a/one.md"), cf("Go_Style", "b/two.md")}

	_, _, _, err := Plan(rules, nil, tg, RoutingAll, ScopeInfo{}, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "a/one.md")
	assert.Contains(t, err.Error(), "b/two.md")
}

func TestPlan_ContextDoesNotCollideWithRule(t *testing.T) {
	tg := &Target{Ext: ".md"}

	files, _, _, err := Plan([]config.ContentFile{cf("x", "r.md")}, []config.ContentFile{cf("x", "c.md", "a/**")},
		tg, RoutingAll, ScopeInfo{}, nil)

	require.NoError(t, err)
	assert.Len(t, files, 2)
}

func TestPlan_Scope(t *testing.T) {
	rules := []config.ContentFile{cf("Go Rule", "r/go.md", "*.go", "/lib/*.go"), cf("global", "r/g.md")}
	scope := ScopeInfo{Slug: "api", Prefix: "services/api"}
	tests := []struct {
		name      string
		recursive bool
		ids       []string
	}{
		{"flat", false, []string{"api--Go-Rule", "api--global"}},
		{"recursive", true, []string{"api/Go-Rule", "api/global"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tg := &Target{Ext: ".md", Recursive: tt.recursive}

			files, _, _, err := Plan(rules, nil, tg, RoutingAll, scope, nil)

			require.NoError(t, err)
			require.Len(t, files, 2)
			assert.Equal(t, tt.ids, []string{files[0].ID, files[1].ID})
			assert.Equal(t, []string{"services/api/*.go", "services/api/lib/*.go"}, files[0].Activation.Globs)
			assert.Equal(t, config.ActivationGlob, files[1].Activation.Mode)
			assert.Equal(t, []string{"services/api/**"}, files[1].Activation.Globs)
		})
	}
}

func TestRoutingFor(t *testing.T) {
	assert.Equal(t, RoutingAll, RoutingFor("split", true))
	assert.Equal(t, RoutingScopedOnly, RoutingFor("inline", true))
	assert.Equal(t, RoutingNone, RoutingFor("split", false))
	assert.Equal(t, RoutingNone, RoutingFor("inline", false))
}

func TestFileName_LocalPath(t *testing.T) {
	md := Target{Dir: ".claude/rules", Ext: ".md"}
	cop := Target{Dir: ".github/instructions/", Ext: ".instructions.md"}
	tests := []struct {
		name string
		fn   func() string
		want string
	}{
		{"rule", func() string { return FileName(md, Item{ID: "go"}) }, "go.md"},
		{"context", func() string { return FileName(md, Item{ID: "go", Kind: KindContext}) }, "context-go.md"},
		{"copilot rule", func() string { return FileName(cop, Item{ID: "go"}) }, "go.instructions.md"},
		{"local md", func() string { return LocalPath(md, "go") }, ".claude/rules/go.local.md"},
		{"local copilot", func() string { return LocalPath(cop, "go") }, ".github/instructions/go.local.instructions.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.fn())
		})
	}
}

func TestExpandBraces_Cap(t *testing.T) {
	big := strings.Repeat("{a,b}", 9) // 512 alternatives

	got, ok := expandBraces(big)

	assert.False(t, ok)
	assert.Equal(t, []string{big}, got)
	joined, notes := joinExpanded("r", []string{big, "*.{a,a}"})
	assert.Equal(t, big+",*.a", joined)
	require.Len(t, notes, 1)
}

func TestPlan_RegistryCollisions(t *testing.T) {
	tg := &Target{Dir: ".claude/rules", Ext: ".md"}
	root := []config.ContentFile{cf("Foo", "root/Foo.md")}
	tests := []struct {
		name  string
		first []config.ContentFile
		scope ScopeInfo
		next  []config.ContentFile
		nscp  ScopeInfo
		fail  bool
	}{
		{"cross plan", root, ScopeInfo{}, []config.ContentFile{cf("foo", "s/foo.md")}, ScopeInfo{}, true},
		{"same slug twice", []config.ContentFile{cf("x", "a/x.md")}, ScopeInfo{Slug: "api", Prefix: "api"},
			[]config.ContentFile{cf("x", "b/x.md")}, ScopeInfo{Slug: "api", Prefix: "api"}, true},
		{"different slugs", []config.ContentFile{cf("x", "a/x.md")}, ScopeInfo{Slug: "api", Prefix: "api"},
			[]config.ContentFile{cf("x", "b/x.md")}, ScopeInfo{Slug: "web", Prefix: "web"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := NewRegistry()
			_, _, _, err := Plan(tt.first, nil, tg, RoutingAll, tt.scope, reg)
			require.NoError(t, err)

			_, _, _, err = Plan(tt.next, nil, tg, RoutingAll, tt.nscp, reg)

			assert.Equal(t, tt.fail, err != nil)
		})
	}
}

func TestPlan_Collisions(t *testing.T) {
	tests := []struct {
		name    string
		rules   []config.ContentFile
		context []config.ContentFile
		wantErr string
	}{
		{"case-only rule names", []config.ContentFile{cf("Foo", "a.md"), cf("foo", "b.md")}, nil, "a.md"},
		{"names equal after sanitizing", []config.ContentFile{cf("a b", "a.md"), cf("a_b", "b.md")}, nil, "a.md"},
		{"rule context-x vs context x", []config.ContentFile{cf("context-x", "r.md")},
			[]config.ContentFile{cf("x", "c.md")}, "c.md"},
		{"case-only rule vs context", []config.ContentFile{cf("Context-X", "r.md")},
			[]config.ContentFile{cf("x", "c.md")}, "r.md"},
		{"case-only context names", nil, []config.ContentFile{cf("Ctx", "a.md"), cf("ctx", "b.md")}, "b.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tg := &Target{Preset: "cline", Ext: ".md"}

			_, _, _, err := Plan(tt.rules, tt.context, tg, RoutingEverything, ScopeInfo{}, nil)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Contains(t, err.Error(), "collide")
		})
	}
}

func TestPlan_EmptyIDFallsBackToHash(t *testing.T) {
	tg := &Target{Ext: ".md"}

	files, _, _, err := Plan([]config.ContentFile{cf("!!!", "c.md"), cf("日本語", "d.md")},
		[]config.ContentFile{cf("文脈", "e.md")}, tg, RoutingEverything, ScopeInfo{}, nil)

	require.NoError(t, err)
	require.Len(t, files, 3)
	assert.Equal(t, "rule-9a7b006d", ItemID("!!!"), "stable hash")
	assert.Equal(t, ItemID("!!!")+".md", FileName(*tg, files[0]))
	assert.Equal(t, ItemID("日本語")+".md", FileName(*tg, files[1]))
	assert.Equal(t, "context-"+ItemID("文脈")+".md", FileName(*tg, files[2]))
	assert.NotEqual(t, files[0].ID, files[1].ID)
}

func TestPlan_RoutingEverythingRoutesUnscopedContext(t *testing.T) {
	tg := &Target{Ext: ".md"}
	rules := []config.ContentFile{cf("r", "r.md")}
	context := []config.ContentFile{cf("c", "c.md")}
	tests := []struct {
		name        string
		routing     Routing
		wantFiles   int
		wantInlineC int
	}{
		{"all keeps unscoped context inline", RoutingAll, 1, 1},
		{"everything routes context", RoutingEverything, 2, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files, _, inlineC, err := Plan(rules, context, tg, tt.routing, ScopeInfo{}, nil)

			require.NoError(t, err)
			assert.Len(t, files, tt.wantFiles)
			assert.Len(t, inlineC, tt.wantInlineC)
		})
	}
}

func TestPlan_ScopeActivationRules(t *testing.T) {
	tg := &Target{Ext: ".md"}
	scope := ScopeInfo{Slug: "api", Prefix: "services/api"}
	auto := cf("auto", "a.md")
	auto.Metadata.Activation = "auto"
	auto.Metadata.Extra = map[string]string{"description": "when relevant"}
	manual := cf("manual", "m.md")
	manual.Metadata.Activation = "manual"
	neg := cf("neg", "n.md", "*.go", "!gen/*.go")

	files, _, _, err := Plan([]config.ContentFile{auto, manual, neg}, nil, tg, RoutingAll, scope, nil)

	require.NoError(t, err)
	assert.Equal(t, config.ActivationAuto, files[0].Activation.Mode)
	assert.Empty(t, files[0].Activation.Globs)
	assert.Equal(t, config.ActivationManual, files[1].Activation.Mode)
	assert.Equal(t, []string{"services/api/*.go", "!services/api/gen/*.go"}, files[2].Activation.Globs)

	_, _, _, err = Plan([]config.ContentFile{cf("esc", "e.md", "../x/*.go")}, nil, tg, RoutingAll, scope, nil)
	require.Error(t, err)
}

func TestPlan_ScopedContextRecursive(t *testing.T) {
	tg := &Target{Ext: ".md", Recursive: true}

	files, _, _, err := Plan(nil, []config.ContentFile{cf("x", "c.md", "a/**")}, tg, RoutingAll,
		ScopeInfo{Slug: "api", Prefix: "api"}, nil)

	require.NoError(t, err)
	assert.Equal(t, "api/context-x.md", FileName(*tg, files[0]))
}

func TestFrontmatter_ActivationEdgeCases(t *testing.T) {
	type m = map[string]any
	tests := []struct {
		name     string
		dialect  Dialect
		it       Item
		want     m
		wantNote string
	}{
		{"glob without globs (trigger glob, no glob) is manual", DialectTrigger,
			item("r", config.ActivationGlob, ""), m{"trigger": "manual"}, "glob activation without globs"},
		{"glob without globs in cursor", DialectCursor,
			item("r", config.ActivationGlob, ""), m{"alwaysApply": false}, "glob activation without globs"},
		{"auto without description is manual", DialectTrigger,
			item("r", config.ActivationAuto, ""), m{"trigger": "manual"}, "auto activation without a description"},
		{"auto with blank description in cursor", DialectCursor,
			item("r", config.ActivationAuto, "  "), m{"alwaysApply": false}, "auto activation without a description"},
		{"cursor auto is explicitly not always-on", DialectCursor,
			item("r", config.ActivationAuto, "when sql"), m{"description": "when sql", "alwaysApply": false}, ""},
		{"copilot drops negated globs", DialectCopilot,
			item("r", config.ActivationGlob, "", "src/**", "!src/gen/**"), m{"applyTo": "src/**"}, "no negated globs"},
		{"copilot with only negated globs applies everywhere", DialectCopilot,
			item("r", config.ActivationGlob, "", "!src/gen/**"), m{"applyTo": "**"}, "no negated globs"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm, notes := Frontmatter(tt.dialect, tt.it)

			assert.Equal(t, tt.want, m(fm))
			if tt.wantNote == "" {
				assert.Empty(t, notes)
				return
			}
			require.NotEmpty(t, notes)
			assert.Contains(t, notes[0].Text, tt.wantNote)
			assert.False(t, notes[0].Downgrade)
		})
	}
}

func TestFallbackNote_NamesContext(t *testing.T) {
	it := item("overview", config.ActivationManual, "")
	it.Kind = KindContext

	_, notes := Frontmatter(DialectClaude, it)

	require.Len(t, notes, 1)
	assert.True(t, strings.HasPrefix(notes[0].Text, `context "overview":`), notes[0].Text)
	assert.True(t, notes[0].Downgrade)
	assert.Equal(t, KindContext, notes[0].Kind)
}

func TestReportNotes_DowngradesAggregateAcrossPresets(t *testing.T) {
	// Arrange
	type call struct {
		msg  string
		args []any
	}
	var calls []call
	t.Cleanup(SetWarnSink(func(msg string, args ...any) { calls = append(calls, call{msg, args}) }))
	ResetDowngrades()
	it := item("shared", config.ActivationManual, "")
	_, claudeNotes := Frontmatter(DialectClaude, it)
	_, clineNotes := Frontmatter(DialectCline, it)
	limit := Note{Kind: KindRule, Name: "big", Text: `rule "big": too long`}

	// Act: the same item downgraded in two presets, plus one soft-limit note.
	ReportNotes("/p/.claude/rules/shared.md", claudeNotes)
	ReportNotes("/p/.clinerules/shared.md", clineNotes)
	ReportNotes("/p/.claude/rules/big.md", []Note{limit})
	FlushDowngrades()

	// Assert: one aggregated downgrade warning and one per-file warning.
	require.Len(t, calls, 2)
	assert.Equal(t, `rule "big": too long`, calls[0].msg)
	assert.Equal(t, []any{"file", "/p/.claude/rules/big.md"}, calls[0].args)
	assert.Contains(t, calls[1].msg, "1 rules/context items use an activation the target tool cannot express")
	assert.Equal(t, []any{"items", "rule shared"}, calls[1].args)
}

func TestFrontmatter_OneNotePerCause(t *testing.T) {
	tests := []struct {
		name          string
		dialect       Dialect
		it            Item
		wantDowngrade bool
	}{
		{"claude glob without globs", DialectClaude, item("r", config.ActivationGlob, ""), true},
		{"cline glob without globs", DialectCline, item("r", config.ActivationGlob, ""), true},
		{"copilot glob without globs", DialectCopilot, item("r", config.ActivationGlob, ""), false},
		{"claude auto without description", DialectClaude, item("r", config.ActivationAuto, ""), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, notes := Frontmatter(tt.dialect, tt.it)

			require.Len(t, notes, 1)
			assert.Equal(t, tt.wantDowngrade, notes[0].Downgrade)
			assert.Contains(t, notes[0].Text, "without")
		})
	}
}
