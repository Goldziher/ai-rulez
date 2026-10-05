package providers_test

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func splitContent() *config.ContentTree {
	return &config.ContentTree{
		Rules: []config.ContentFile{
			{Name: "always", Path: "/p/.ai-rulez/rules/always.md", Content: "ALWAYS_RULE"},
			{
				Name: "tsx", Path: "/p/.ai-rulez/rules/tsx.md", Content: "TSX_RULE",
				Metadata: &config.Metadata{Globs: []string{"**/*.tsx"}},
			},
		},
		Context: []config.ContentFile{
			{Name: "layout", Path: "/p/.ai-rulez/context/layout.md", Content: "LAYOUT_CTX"},
			{
				Name: "api docs", Path: "/p/.ai-rulez/context/api.md", Content: "API_CTX",
				Metadata: &config.Metadata{Globs: []string{"api/**"}},
			},
		},
	}
}

func splitCfg(preset, mode string) *config.Config {
	cfg := &config.Config{Name: "test", ConfigDir: "/p/.ai-rulez", ConfigDirName: ".ai-rulez"}
	if mode != "" {
		cfg.Rules = &config.RulesConfig{ModeByPreset: map[string]string{preset: mode}}
	}
	return cfg
}

func outputByPath(outputs []config.OutputFile, rel string) (config.OutputFile, bool) {
	for _, o := range outputs {
		if !o.IsDir && strings.HasSuffix(filepath.ToSlash(o.Path), "/"+rel) {
			return o, true
		}
	}
	return config.OutputFile{}, false
}

func fileSet(outputs []config.OutputFile, dir string) []string {
	var names []string
	for _, o := range outputs {
		if !o.IsDir && strings.Contains(filepath.ToSlash(o.Path), "/"+dir+"/") {
			names = append(names, filepath.Base(o.Path))
		}
	}
	return names
}

func TestClaude_RulesSplitMode(t *testing.T) {
	t.Parallel()

	// Arrange
	gen := claudeGen(t)

	// Act
	outputs, err := gen.Generate(splitContent(), "/test", splitCfg("claude", "split"))

	// Assert
	require.NoError(t, err)
	root, ok := outputByPath(outputs, "CLAUDE.md")
	require.True(t, ok)
	assert.NotContains(t, root.Content, "## Rules")
	assert.NotContains(t, root.Content, "ALWAYS_RULE")
	assert.NotContains(t, root.Content, "TSX_RULE")
	assert.Contains(t, root.Content, "LAYOUT_CTX", "unscoped context stays inline")
	assert.NotContains(t, root.Content, "API_CTX", "scoped context moves to a file")

	assert.ElementsMatch(t, []string{"always.md", "tsx.md", "context-api-docs.md"}, fileSet(outputs, ".claude/rules"))

	always, _ := outputByPath(outputs, ".claude/rules/always.md")
	assert.NotContains(t, always.Content, "paths:")
	assert.Contains(t, always.Content, "ALWAYS_RULE")
	assert.Contains(t, always.Content, ".ai-rulez/rules/always.md", "banner names the source")

	tsx, _ := outputByPath(outputs, ".claude/rules/tsx.md")
	assert.True(t, strings.HasPrefix(tsx.Content, "---\npaths:\n"), tsx.Content)
	assert.Contains(t, tsx.Content, "**/*.tsx")
	assert.Contains(t, tsx.Content, ".ai-rulez/rules/tsx.md")
}

func TestClaude_RulesInlineModeScopedToFiles(t *testing.T) {
	t.Parallel()

	// Arrange
	gen := claudeGen(t)

	// Act: in inline mode only path-scoped items go to files
	outputs, err := gen.Generate(splitContent(), "/test", splitCfg("claude", "inline"))

	// Assert
	require.NoError(t, err)
	root, _ := outputByPath(outputs, "CLAUDE.md")
	assert.Contains(t, root.Content, "ALWAYS_RULE")
	assert.NotContains(t, root.Content, "TSX_RULE")
	assert.Contains(t, root.Content, "LAYOUT_CTX")
	assert.NotContains(t, root.Content, "API_CTX", "path-scoped context already goes to the rules dir")
	assert.ElementsMatch(t, []string{"tsx.md", "context-api-docs.md"}, fileSet(outputs, ".claude/rules"))
}

func TestClaude_ScopedContextToRulesDir(t *testing.T) {
	t.Parallel()

	for _, mode := range []string{"inline", "split", ""} {
		t.Run("mode="+mode, func(t *testing.T) {
			t.Parallel()

			// Arrange / Act
			outputs, err := claudeGen(t).Generate(splitContent(), "/test", splitCfg("claude", mode))

			// Assert
			require.NoError(t, err)
			file, ok := outputByPath(outputs, ".claude/rules/context-api-docs.md")
			require.True(t, ok)
			assert.Contains(t, file.Content, "paths:")
			assert.Contains(t, file.Content, "api/**")
			assert.Contains(t, file.Content, "API_CTX")
			root, _ := outputByPath(outputs, "CLAUDE.md")
			assert.NotContains(t, root.Content, "API_CTX")
		})
	}
}

func TestJunie_RulesSplit(t *testing.T) {
	t.Parallel()

	gen, err := providers.LoadBuiltin("junie")
	require.NoError(t, err)

	tests := []struct {
		name      string
		mode      string
		wantFiles []string
		wantInRoot,
		notInRoot []string
	}{
		{
			name: "split", mode: "split",
			wantFiles:  []string{"always.md", "tsx.md", "context-api-docs.md"},
			wantInRoot: []string{"LAYOUT_CTX"},
			notInRoot:  []string{"ALWAYS_RULE", "TSX_RULE", "API_CTX"},
		},
		{
			name: "inline keeps everything in the root AGENTS.md", mode: "inline",
			wantFiles:  nil,
			wantInRoot: []string{"ALWAYS_RULE", "TSX_RULE", "LAYOUT_CTX", "API_CTX"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Act
			outputs, err := gen.Generate(splitContent(), "/test", splitCfg("junie", tt.mode))

			// Assert
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.wantFiles, fileSet(outputs, ".junie/rules"))
			root, ok := outputByPath(outputs, "AGENTS.md")
			require.True(t, ok)
			for _, s := range tt.wantInRoot {
				assert.Contains(t, root.Content, s)
			}
			for _, s := range tt.notInRoot {
				assert.NotContains(t, root.Content, s)
			}
		})
	}

	t.Run("rule file carries the applies-to line", func(t *testing.T) {
		t.Parallel()
		outputs, err := gen.Generate(splitContent(), "/test", splitCfg("junie", "split"))
		require.NoError(t, err)
		tsx, ok := outputByPath(outputs, ".junie/rules/tsx.md")
		require.True(t, ok)
		assert.NotContains(t, tsx.Content, "paths:")
		assert.Contains(t, tsx.Content, "_Applies to: `**/*.tsx`_")
	})
}

func TestValidateSpec_SplitFields(t *testing.T) {
	t.Parallel()
	t.Cleanup(func() { config.UnregisterRulesDir("r") }) // the valid case registers its dir

	const head = "name = \"demo\"\n[root]\nfile = \"D.md\"\nsections = [\"rules_inline\"]\n[outputs.rules]\nmode = \"per_item_file\"\ndir = \"r\"\nfilename = \"{id}.md\"\n"
	tests := []struct {
		name    string
		spec    string
		wantErr string
	}{
		{name: "valid split", spec: head + "split = true\ndialect = \"claude\"\ninline_filter = \"path_scoped\"\n"},
		{name: "legacy frontmatter paths without dialect", spec: head + "[outputs.rules.frontmatter]\npaths = true\n"},
		{name: "split with filter", spec: head + "split = true\ndialect = \"claude\"\nfilter = \"path_scoped\"\n", wantErr: "filter"},
		{name: "unknown dialect", spec: head + "split = true\ndialect = \"nope\"\n", wantErr: "unknown dialect"},
		{name: "unknown inline filter", spec: head + "split = true\ndialect = \"claude\"\ninline_filter = \"x\"\n", wantErr: "inline_filter"},
		{
			name:    "dialect with body",
			spec:    head + "split = true\ndialect = \"claude\"\n[outputs.rules.body]\nsections = [\"content\"]\n",
			wantErr: "cannot be combined",
		},
		{
			name:    "dialect with frontmatter",
			spec:    head + "split = true\ndialect = \"claude\"\n[outputs.rules.frontmatter]\npaths = true\n",
			wantErr: "cannot be combined",
		},
		{
			name:    "split without rules_inline root section",
			spec:    strings.Replace(head, "\"rules_inline\"", "\"title\"", 1) + "split = true\ndialect = \"claude\"\n",
			wantErr: "rules_inline",
		},
		{name: "dir with drive letter", spec: strings.Replace(head, "dir = \"r\"", "dir = \"C:/rules\"", 1) + "split = true\ndialect = \"claude\"\n", wantErr: "relative path"},
		{name: "dir with drive letter and backslash", spec: strings.Replace(head, "dir = \"r\"", "dir = 'C:\\rules'", 1) + "split = true\ndialect = \"claude\"\n", wantErr: "relative path"},
		{name: "UNC dir", spec: strings.Replace(head, "dir = \"r\"", "dir = '\\\\host\\share'", 1) + "split = true\ndialect = \"claude\"\n", wantErr: "relative path"},
		{name: "double slash dir", spec: strings.Replace(head, "dir = \"r\"", "dir = \"//host/share\"", 1) + "split = true\ndialect = \"claude\"\n", wantErr: "relative path"},
		{name: "dot dir", spec: strings.Replace(head, "dir = \"r\"", "dir = \".\"", 1) + "split = true\ndialect = \"claude\"\n", wantErr: "relative path"},
		{name: "split without dialect", spec: head + "split = true\n", wantErr: "dialect is required"},
		{
			name:    "split without dir",
			spec:    strings.Replace(head, "dir = \"r\"\n", "", 1) + "split = true\ndialect = \"claude\"\n",
			wantErr: "dir is required",
		},
		{
			name:    "split with dir escaping the project",
			spec:    strings.Replace(head, "dir = \"r\"", "dir = \"../r\"", 1) + "split = true\ndialect = \"claude\"\n",
			wantErr: "relative path",
		},
		{name: "dialect without split", spec: head + "dialect = \"claude\"\n", wantErr: "require split"},
		{
			name:    "nested filename",
			spec:    strings.Replace(head, "{id}.md", "{id}/x.md", 1) + "split = true\ndialect = \"claude\"\n",
			wantErr: "flat",
		},
		{
			name:    "split on skills",
			spec:    "name = \"demo\"\n[outputs.skills]\nmode = \"per_item_file\"\nsplit = true\n",
			wantErr: "only valid on outputs.rules",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Act
			_, err := providers.LoadProviderSpec([]byte(tt.spec), "demo.toml", providers.FormatTOML)

			// Assert
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestLoadProviderSpec_SplitRegistersRulesDir(t *testing.T) {
	// Not parallel: the registry of rules folders is process-wide.
	spec := "name = \"demo\"\n[root]\nfile = \"D.md\"\nsections = [\"rules_inline\"]\n" +
		"[outputs.rules]\nmode = \"per_item_file\"\ndir = \".w3-demo/rules\"\nfilename = \"{id}.md\"\n" +
		"split = true\ndialect = \"claude\"\n"
	require.False(t, config.InRulesDir(".w3-demo/rules/x.md"))
	t.Cleanup(func() { config.UnregisterRulesDir(".w3-demo/rules") })

	_, err := providers.LoadProviderSpec([]byte(spec), "demo.toml", providers.FormatTOML)

	require.NoError(t, err)
	assert.True(t, config.InRulesDir(".w3-demo/rules/x.md"))
}

func TestCustomProvider_LegacyRulesFilterUnchanged(t *testing.T) {
	t.Parallel()

	// Arrange: a legacy spec with a path_scoped filter and no split.
	spec := `name = "demo"
[root]
file = "DEMO.md"
sections = ["rules_inline", "context_inline"]
[outputs.rules]
mode = "per_item_file"
dir = "rules"
filename = "{id}.md"
filter = "path_scoped"
[outputs.rules.body]
sections = ["frontmatter", "content"]
[outputs.rules.frontmatter]
paths = true
`
	parsed, err := providers.LoadProviderSpec([]byte(spec), "demo.toml", providers.FormatTOML)
	require.NoError(t, err)

	// Act: split mode must not change a spec that did not opt in.
	outputs, err := providers.New(parsed).Generate(splitContent(), "/test", splitCfg("demo", "split"))

	// Assert
	require.NoError(t, err)
	root, _ := outputByPath(outputs, "DEMO.md")
	assert.Contains(t, root.Content, "ALWAYS_RULE")
	assert.NotContains(t, root.Content, "TSX_RULE")
	assert.Contains(t, root.Content, "API_CTX", "legacy specs keep scoped context inline")
	assert.Equal(t, []string{"tsx.md"}, fileSet(outputs, "rules"))
	file, _ := outputByPath(outputs, "rules/tsx.md")
	assert.Contains(t, file.Content, "paths:")
	assert.Contains(t, file.Content, "TSX_RULE")
}

func TestRenderRoot_SkipsExactlyAcceptedItems(t *testing.T) {
	t.Parallel()

	// A legacy spec whose filter accepts nothing path-scoped must not drop
	// path-scoped rules from the root file.
	spec := `name = "demo"
[root]
file = "DEMO.md"
sections = ["rules_inline"]
[outputs.rules]
mode = "per_item_file"
dir = "rules"
filename = "{id}.md"
filter = "include_if_targeting_provider"
[outputs.rules.body]
sections = ["content"]
`
	parsed, err := providers.LoadProviderSpec([]byte(spec), "demo.toml", providers.FormatTOML)
	require.NoError(t, err)

	for _, tt := range []struct {
		name     string
		gen      *providers.Generator
		mode     string
		preset   string
		accepted []string
	}{
		{"legacy accept-all", providers.New(parsed), "inline", "demo", []string{"always.md", "tsx.md"}},
		{"claude inline", claudeGen(t), "inline", "claude", []string{"tsx.md"}},
		{"claude split", claudeGen(t), "split", "claude", []string{"always.md", "tsx.md"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Act
			outputs, err := tt.gen.Generate(splitContent(), "/test", splitCfg(tt.preset, tt.mode))

			// Assert: each rule is in exactly one place.
			require.NoError(t, err)
			var rootContent string
			for _, o := range outputs {
				if !o.IsDir && (strings.HasSuffix(o.Path, "CLAUDE.md") || strings.HasSuffix(o.Path, "DEMO.md")) {
					rootContent = o.Content
				}
			}
			rulesDir := "rules"
			if tt.preset == "claude" {
				rulesDir = ".claude/rules"
			}
			var files []string
			for _, name := range fileSet(outputs, rulesDir) {
				if !strings.HasPrefix(name, "context-") {
					files = append(files, name)
				}
			}
			assert.ElementsMatch(t, tt.accepted, files)
			for rule, marker := range map[string]string{"always.md": "ALWAYS_RULE", "tsx.md": "TSX_RULE"} {
				inFile := false
				for _, f := range files {
					inFile = inFile || f == rule
				}
				assert.NotEqual(t, inFile, strings.Contains(rootContent, marker), "%s must be in exactly one place", rule)
			}
		})
	}
}

func TestCustomProvider_LegacyRuleIDKeepsSanitizeAgentID(t *testing.T) {
	t.Parallel()

	// Arrange
	spec := `name = "demo"
[outputs.rules]
mode = "per_item_file"
dir = "rules"
filename = "{id}.md"
filter = "path_scoped"
[outputs.rules.body]
sections = ["content"]
`
	parsed, err := providers.LoadProviderSpec([]byte(spec), "demo.toml", providers.FormatTOML)
	require.NoError(t, err)
	content := &config.ContentTree{Rules: []config.ContentFile{{
		Name: "Go Style.v2", Content: "X", Metadata: &config.Metadata{Globs: []string{"*.go"}},
	}}}

	// Act
	outputs, err := providers.New(parsed).Generate(content, "/test", splitCfg("demo", ""))

	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{"go-style.v2.md"}, fileSet(outputs, "rules"))
}

func TestClaude_RuleNameCollisionKeepsBothRules(t *testing.T) {
	t.Parallel()

	// Arrange: names that map to the same id once case is ignored.
	scoped := &config.Metadata{Globs: []string{"*.go"}}
	rules := []config.ContentFile{
		{Name: "Foo", Path: "a.md", Content: "A", Metadata: scoped},
		{Name: "foo", Path: "b.md", Content: "B", Metadata: scoped},
	}

	// Act
	outputs, err := claudeGen(t).Generate(&config.ContentTree{Rules: rules}, "/test", splitCfg("claude", ""))

	// Assert
	require.NoError(t, err)
	files := fileSet(outputs, "rules")
	require.Len(t, files, 2)
	assert.Equal(t, "Foo.md", files[0])
	assert.Regexp(t, `^foo-[0-9a-f]{6}\.md$`, files[1])
}

func TestClaude_HeaderRuleCountAfterRouting(t *testing.T) {
	t.Parallel()

	// Act: two rules, one path-scoped, so CLAUDE.md inlines one.
	detailed := func(mode string) *config.Config {
		cfg := splitCfg("claude", mode)
		cfg.Header = &config.HeaderConfig{Style: "detailed"}
		return cfg
	}
	inline, err := claudeGen(t).Generate(splitContent(), "/test", detailed("inline"))
	require.NoError(t, err)
	split, err := claudeGen(t).Generate(splitContent(), "/test", detailed("split"))
	require.NoError(t, err)

	// Assert
	rootInline, _ := outputByPath(inline, "CLAUDE.md")
	rootSplit, _ := outputByPath(split, "CLAUDE.md")
	assert.Contains(t, rootInline.Content, "rules=1")
	assert.Contains(t, rootSplit.Content, "rules=0")
}

func TestClaude_AutoAndManualRulesStayInlineInInlineMode(t *testing.T) {
	t.Parallel()

	// Arrange
	content := &config.ContentTree{Rules: []config.ContentFile{
		{Name: "auto-rule", Content: "AUTO_BODY", Metadata: &config.Metadata{Activation: "auto", Extra: map[string]string{"description": "when X"}}},
		{Name: "manual-rule", Content: "MANUAL_BODY", Metadata: &config.Metadata{Activation: "manual"}},
	}}

	// Act
	outputs, err := claudeGen(t).Generate(content, "/test", splitCfg("claude", "inline"))

	// Assert
	require.NoError(t, err)
	root, _ := outputByPath(outputs, "CLAUDE.md")
	assert.Contains(t, root.Content, "AUTO_BODY")
	assert.Contains(t, root.Content, "MANUAL_BODY")
	assert.Empty(t, fileSet(outputs, ".claude/rules"))
}

func TestClaude_SkillTargetedRulesSeeAllRulesInSplitMode(t *testing.T) {
	t.Parallel()

	// Arrange: a rule that goes to a file and targets the skill file.
	content := &config.ContentTree{
		Skills: []config.ContentFile{{Name: "s", Path: "/test/skills/s/SKILL.md", Content: "# S"}},
		Rules: []config.ContentFile{{
			Name: "targeted", Content: "T_BODY",
			Metadata: &config.Metadata{Targets: []string{".claude/skills/*/SKILL.md"}},
		}},
	}
	cfg := splitCfg("claude", "split")
	cfg.BaseDir = "/test"

	// Act
	body := findSkillBody(t, claudeGen(t), content, cfg, "/test", "s")

	// Assert
	assert.Contains(t, body, "### targeted")
	assert.Contains(t, body, "T_BODY")
}

func TestClaude_RuleFileNames(t *testing.T) {
	t.Parallel()

	// Arrange: spaces, underscores, case, dots and a name with no ASCII letters or digits.
	rules := []config.ContentFile{
		{Name: "My Rule", Content: "a"}, {Name: "snake_case_rule", Content: "b"}, {Name: "UPPER", Content: "c"},
		{Name: "v1.2 notes", Content: "d"}, {Name: "日本語", Content: "e"},
	}
	cfg := splitCfg("claude", "split")

	// Act
	outputs, err := claudeGen(t).Generate(&config.ContentTree{Rules: rules}, "/test", cfg)

	// Assert
	require.NoError(t, err)
	var got []string
	for _, o := range outputs {
		if p := filepath.ToSlash(o.Path); !o.IsDir && strings.Contains(p, "/.claude/rules/") {
			got = append(got, p[strings.Index(p, "/.claude/rules/")+len("/.claude/rules/"):])
		}
	}
	sort.Strings(got)
	assert.Equal(t, []string{"My-Rule.md", "UPPER.md", "rule-c12140a0.md", "snake-case-rule.md", "v12-notes.md"}, got)
}

func targetsContent() *config.ContentTree {
	mk := func(name string, targets ...string) config.ContentFile {
		return config.ContentFile{
			Name: name, Content: strings.ToUpper(strings.ReplaceAll(name, "-", "_")) + "_BODY",
			Path:     "/p/.ai-rulez/rules/" + name + ".md",
			Metadata: &config.Metadata{Targets: targets},
		}
	}
	return &config.ContentTree{Rules: []config.ContentFile{
		mk("free"),
		mk("to-claude-md", "CLAUDE.md"),
		mk("to-cursor", "cursor"),
		mk("to-cursor-dir", ".cursor/rules/"),
		mk("to-glob", "*.mdc"),
		mk("to-claude-dir", ".claude/rules/"),
	}}
}

func TestClaude_TargetsFilterRuleOutputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		mode      string
		wantFiles []string
		wantRoot  []string
		notRoot   []string
	}{
		{
			name:      "split",
			mode:      "split",
			wantFiles: []string{"free.md", "to-claude-md.md", "to-claude-dir.md"},
			notRoot:   []string{"FREE_BODY", "TO_CURSOR_BODY", "TO_CURSOR_DIR_BODY", "TO_GLOB_BODY"},
		},
		{
			name:      "inline",
			mode:      "inline",
			wantFiles: []string{"to-claude-dir.md"},
			wantRoot:  []string{"FREE_BODY", "TO_CLAUDE_MD_BODY"},
			notRoot:   []string{"TO_CURSOR_BODY", "TO_CURSOR_DIR_BODY", "TO_GLOB_BODY"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			gen := claudeGen(t)

			// Act
			outputs, err := gen.Generate(targetsContent(), "/test", splitCfg("claude", tt.mode))

			// Assert
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.wantFiles, fileSet(outputs, ".claude/rules"))
			root, ok := outputByPath(outputs, "CLAUDE.md")
			require.True(t, ok)
			for _, body := range tt.wantRoot {
				assert.Contains(t, root.Content, body)
			}
			for _, body := range tt.notRoot {
				assert.NotContains(t, root.Content, body)
			}
		})
	}
}
