package providers_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/providers"
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

func TestSplitRulesDir_IsTheFolderOfASplitRulesOutput(t *testing.T) {
	spec := "name = \"demo\"\n[root]\nfile = \"D.md\"\nsections = [\"rules_inline\"]\n" +
		"[outputs.rules]\nmode = \"per_item_file\"\ndir = \".w3-demo/rules\"\nfilename = \"{id}.md\"\n" +
		"split = true\ndialect = \"claude\"\n"

	loaded, err := providers.LoadProviderSpec([]byte(spec), "demo.toml", providers.FormatTOML)

	require.NoError(t, err)
	assert.Equal(t, ".w3-demo/rules", providers.New(loaded).SplitRulesDir())
	assert.False(t, config.InRulesDir(".w3-demo/rules/x.md"), "loading a spec registers nothing process-wide")
}
