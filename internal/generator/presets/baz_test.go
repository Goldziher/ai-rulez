package presets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGlobStaticDir(t *testing.T) {
	tests := []struct {
		glob string
		want string
	}{
		{"services/api/**/*.py", "services/api"},
		{"services/api/**", "services/api"},
		{"services/api/*.py", "services/api"},
		{"services/api/main.py", "services/api"},
		{"services/api/", "services/api"},
		{"./services/api/**", "services/api"},
		{"/services/api/**", "services/api"},
		{"services/*/src/**", "services"},
		{"**/*.py", ""},
		{"*.py", ""},
		{"../other/**", ""},
		{"a/../b/**", ""},
		{"src/[ab]/x.go", "src"},
		{"src/a?/x.go", "src"},
	}
	for _, tt := range tests {
		t.Run(tt.glob, func(t *testing.T) {
			assert.Equal(t, tt.want, globStaticDir(tt.glob))
		})
	}
}

func TestMinimalDirs(t *testing.T) {
	assert.Equal(t, []string{"a", "b/c"}, minimalDirs([]string{"b/c", "a/x", "a", "a", "b/c"}))
	assert.Equal(t, []string{"a"}, minimalDirs([]string{"a", "a/b"}))
	assert.Equal(t, []string{"a", "ab"}, minimalDirs([]string{"ab", "a"}), "a sibling sharing a prefix is not a child")
}

func bazProject(t *testing.T, dirs ...string) *config.Config {
	t.Helper()
	root := t.TempDir()
	for _, dir := range dirs {
		require.NoError(t, os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), 0o755))
	}
	return &config.Config{
		Name:    "proj",
		BaseDir: root,
		Presets: []config.Preset{{BuiltIn: "baz"}},
	}
}

func globRule(name string, globs ...string) config.ContentFile {
	return config.ContentFile{
		Name:     name,
		Path:     "rules/" + name + ".md",
		Content:  name + " BODY",
		Metadata: &config.Metadata{Globs: globs},
	}
}

func TestBazNestedDirs(t *testing.T) {
	cfg := bazProject(t, "services/api", "services/web", ".ai-rulez/rules", "scoped")
	cfg.Scopes = []config.ScopeConfig{{Path: "scoped"}}

	tests := []struct {
		name string
		item config.ContentFile
		want []string
	}{
		{"always-on", config.ContentFile{Name: "r", Metadata: &config.Metadata{}}, nil},
		{"single dir", globRule("r", "services/api/**/*.py"), []string{"services/api"}},
		{"two dirs", globRule("r", "services/web/**", "services/api/**"), []string{"services/api", "services/web"}},
		{"child covered by parent", globRule("r", "services/**/x.py", "services/api/**"), []string{"services"}},
		{"brace expansion", globRule("r", "services/{api,web}/**"), []string{"services/api", "services/web"}},
		{"any dir glob stays at root", globRule("r", "**/*.py"), nil},
		{"one rootless glob keeps the whole item at root", globRule("r", "services/api/**", "*.md"), nil},
		{"missing directory stays at root", globRule("r", "nope/**"), nil},
		{"negated only stays at root", globRule("r", "!services/api/**"), nil},
		{"negation beside a positive glob", globRule("r", "services/api/**", "!services/api/legacy/**"), []string{"services/api"}},
		{"config dir is never a target", globRule("r", ".ai-rulez/rules/**"), nil},
		{"scope dir is left to the scope", globRule("r", "scoped/**"), nil},
		{"escaping glob", globRule("r", "../x/**"), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, bazNestedDirs(tt.item, cfg))
		})
	}
}

func bazContent(rules ...config.ContentFile) *config.ContentTree {
	return &config.ContentTree{Rules: rules}
}

func outputByPath(outputs []config.OutputFile, base, rel string) (config.OutputFile, bool) {
	for _, o := range outputs {
		if o.Path == filepath.Join(base, filepath.FromSlash(rel)) {
			return o, true
		}
	}
	return config.OutputFile{}, false
}

func TestBazGenerate_NestedScopedRules(t *testing.T) {
	cfg := bazProject(t, "services/api", "services/web")
	always := config.ContentFile{Name: "always", Path: "rules/always.md", Content: "ALWAYS BODY", Metadata: &config.Metadata{}}
	content := bazContent(
		always,
		globRule("api-rule", "services/api/**/*.py"),
		globRule("web-rule", "services/web/**"),
		globRule("anywhere", "**/*.sql"),
	)
	cfg.Content = content

	outputs, err := (&BazPresetGenerator{}).Generate(content, cfg.BaseDir, cfg)
	require.NoError(t, err)

	root, ok := outputByPath(outputs, cfg.BaseDir, "AGENTS.md")
	require.True(t, ok)
	assert.Contains(t, root.Content, "ALWAYS BODY")
	assert.Contains(t, root.Content, "anywhere BODY", "a glob without a directory stays in the root file")
	assert.Contains(t, root.Content, "`**/*.sql`")
	assert.NotContains(t, root.Content, "api-rule BODY")
	assert.NotContains(t, root.Content, "web-rule BODY")

	api, ok := outputByPath(outputs, cfg.BaseDir, "services/api/AGENTS.md")
	require.True(t, ok)
	assert.Contains(t, api.Content, "api-rule BODY")
	assert.Contains(t, api.Content, "`services/api/**/*.py`")
	assert.NotContains(t, api.Content, "web-rule BODY")

	web, ok := outputByPath(outputs, cfg.BaseDir, "services/web/AGENTS.md")
	require.True(t, ok)
	assert.Contains(t, web.Content, "web-rule BODY")
}

func TestBazGenerate_RootModeKeepsEverythingInRootFile(t *testing.T) {
	cfg := bazProject(t, "services/api")
	cfg.Rules = &config.RulesConfig{BazScoped: config.BazScopedRoot}
	content := bazContent(globRule("api-rule", "services/api/**"))
	cfg.Content = content

	outputs, err := (&BazPresetGenerator{}).Generate(content, cfg.BaseDir, cfg)
	require.NoError(t, err)

	root, _ := outputByPath(outputs, cfg.BaseDir, "AGENTS.md")
	assert.Contains(t, root.Content, "api-rule BODY")
	_, nested := outputByPath(outputs, cfg.BaseDir, "services/api/AGENTS.md")
	assert.False(t, nested)
}

func TestBazGenerate_RootAgentsMDMatchesCodex(t *testing.T) {
	cfg := bazProject(t, "services/api")
	cfg.Presets = []config.Preset{{BuiltIn: "baz"}, {BuiltIn: "codex"}}
	content := bazContent(
		config.ContentFile{Name: "always", Path: "rules/always.md", Content: "ALWAYS BODY", Metadata: &config.Metadata{}},
		globRule("api-rule", "services/api/**"),
	)
	cfg.Content = content

	bazOut, err := (&BazPresetGenerator{}).Generate(content, cfg.BaseDir, cfg)
	require.NoError(t, err)
	codexOut, err := (&CodexPresetGenerator{}).Generate(content, cfg.BaseDir, cfg)
	require.NoError(t, err)

	bazRoot, _ := outputByPath(bazOut, cfg.BaseDir, "AGENTS.md")
	codexRoot, _ := outputByPath(codexOut, cfg.BaseDir, "AGENTS.md")
	assert.Equal(t, bazRoot.Content, codexRoot.Content, "both write AGENTS.md, so the files must be identical")
}

func TestBazGenerate_TargetsKeepItemsAtRoot(t *testing.T) {
	cfg := bazProject(t, "services/api")
	only := globRule("gemini-only", "services/api/**")
	only.Metadata.Targets = []string{"gemini"}
	content := bazContent(only)
	cfg.Content = content

	// Nothing selects the item for AGENTS.md, so it is written nowhere.
	outputs, err := (&BazPresetGenerator{}).Generate(content, cfg.BaseDir, cfg)
	require.NoError(t, err)
	root, _ := outputByPath(outputs, cfg.BaseDir, "AGENTS.md")
	assert.NotContains(t, root.Content, "gemini-only BODY")
	_, nested := outputByPath(outputs, cfg.BaseDir, "services/api/AGENTS.md")
	assert.False(t, nested)
}

func bazSkillsAgentsContent() *config.ContentTree {
	return &config.ContentTree{
		Skills: []config.ContentFile{
			{Name: "alpha", Path: "skills/alpha/SKILL.md", Content: "ALPHA", Metadata: &config.Metadata{Extra: map[string]string{"description": "Alpha skill"}}},
			{Name: "beta", Path: "skills/beta/SKILL.md", Content: "BETA", Metadata: &config.Metadata{Targets: []string{"claude"}}},
		},
		Agents: []config.ContentFile{
			{Name: "Helper", Path: "agents/helper.md", Content: "HELPER", Metadata: &config.Metadata{Extra: map[string]string{"description": "Helps"}}},
		},
		Commands: []config.ContentFile{
			{Name: "deploy", Path: "commands/deploy.md", Content: "DEPLOY"},
		},
	}
}

func TestBazGenerate_SkillsAgentsAndNoCommands(t *testing.T) {
	tests := []struct {
		name       string
		presets    []string
		wantSkills bool
		wantAgents bool
	}{
		{"baz alone writes skills and agents", []string{"baz"}, true, true},
		{"next to claude it leaves skills and agents to claude", []string{"baz", "claude"}, false, false},
		{"next to codex it still writes them", []string{"baz", "codex"}, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := bazProject(t)
			cfg.Presets = nil
			for _, p := range tt.presets {
				cfg.Presets = append(cfg.Presets, config.Preset{BuiltIn: p})
			}
			content := bazSkillsAgentsContent()
			cfg.Content = content

			outputs, err := (&BazPresetGenerator{}).Generate(content, cfg.BaseDir, cfg)
			require.NoError(t, err)

			skill, hasSkill := outputByPath(outputs, cfg.BaseDir, ".agents/skills/alpha/SKILL.md")
			assert.Equal(t, tt.wantSkills, hasSkill)
			if hasSkill {
				assert.Contains(t, skill.Content, "name: alpha")
				assert.Contains(t, skill.Content, `description: "Alpha skill"`)
			}
			_, hasTargeted := outputByPath(outputs, cfg.BaseDir, ".agents/skills/beta/SKILL.md")
			assert.False(t, hasTargeted, "a skill targeted at another preset is not written")

			agent, hasAgent := outputByPath(outputs, cfg.BaseDir, ".claude/agents/helper.md")
			assert.Equal(t, tt.wantAgents, hasAgent)
			if hasAgent {
				assert.Contains(t, agent.Content, "name: helper")
				assert.Contains(t, agent.Content, "HELPER")
			}

			for _, o := range outputs {
				rel, _ := filepath.Rel(cfg.BaseDir, o.Path)
				assert.False(t, strings.Contains(filepath.ToSlash(rel), "commands"), "commands are ignored by Baz: %s", rel)
				assert.False(t, strings.Contains(filepath.ToSlash(rel), ".claude/rules"), "Baz does not read .claude/rules: %s", rel)
			}
		})
	}
}

func TestBazGenerate_ScopeRunWritesOnlyAgentsMD(t *testing.T) {
	cfg := bazProject(t, "services/api")
	content := bazSkillsAgentsContent()
	content.Rules = []config.ContentFile{globRule("api-rule", "services/api/**")}
	cfg.Content = content
	cfg.Run = &config.RunState{Scope: &config.ScopeRun{RootDir: cfg.BaseDir, Path: "pkg"}}

	outputs, err := (&BazPresetGenerator{}).Generate(content, cfg.BaseDir, cfg)
	require.NoError(t, err)
	require.Len(t, outputs, 1)
	assert.True(t, strings.HasSuffix(outputs[0].Path, "AGENTS.md"))
	assert.Contains(t, outputs[0].Content, "api-rule BODY", "inside a scope the rule stays in the scoped file")
}

func TestBazScopeDir_BareDirectoryScopesToItself(t *testing.T) {
	cfg := bazProject(t, "src/api", "src/web")
	tests := map[string]string{
		"src/api":          "src/api",
		"./src/api":        "src/api",
		"src/api/":         "src/api",
		"src/nope":         "src",
		"src/api/main.py":  "src/api",
		"src/api/*.py":     "src/api",
		"src/*/api":        "src",
		"src/{api,web}/**": "src",
	}
	for glob, want := range tests {
		assert.Equal(t, want, bazScopeDir(glob, cfg), glob)
	}
}

func TestWithoutBazNested_SharedRootFileHelpers(t *testing.T) {
	cfg := bazProject(t, "src/api")
	cfg.Rules = &config.RulesConfig{BazScoped: config.BazScopedNested}
	cfg.Presets = []config.Preset{{BuiltIn: "baz"}, {BuiltIn: "opencode"}}
	content := bazContent(globRule("api", "src/api/**"), config.ContentFile{Name: "all", Content: "ALL", Metadata: &config.Metadata{}})

	got := rootRules(content, cfg, "opencode", "AGENTS.md")

	require.Len(t, got, 1)
	assert.Equal(t, "all", got[0].Name)
	assert.Len(t, rootRules(content, cfg, "gemini", "GEMINI.md"), 2, "only AGENTS.md is filtered")
}
