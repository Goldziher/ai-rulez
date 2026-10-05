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

func backfillConfig() *config.Config {
	return &config.Config{
		Name: "proj",
		MCPServers: map[string]*config.MCPServer{
			"local":  {Command: "npx", Args: []string{"-y", "x"}, Env: map[string]string{"A": "b"}},
			"remote": {Transport: "http", URL: "https://example.com/mcp", Headers: map[string]string{"X-Key": "v"}},
			"off":    {Command: "off", Enabled: boolPtr(false)},
		},
	}
}

func boolPtr(b bool) *bool { return &b }

func backfillContent() *config.ContentTree {
	return &config.ContentTree{
		Commands: []config.ContentFile{
			{Name: "ship", Content: "Ship it.", Metadata: &config.Metadata{Extra: map[string]string{"description": "Ship a release"}}},
			{Name: "other-only", Content: "x", Metadata: &config.Metadata{Targets: []string{"claude"}}},
		},
		Skills: []config.ContentFile{{Name: "taken", Path: "/p/.ai-rulez/skills/taken/SKILL.md", Content: "skill body"}},
	}
}

func backfillOutput(t *testing.T, outputs []config.OutputFile, base, rel string) config.OutputFile {
	t.Helper()
	want := filepath.Join(base, filepath.FromSlash(rel))
	for _, o := range outputs {
		if o.Path == want {
			return o
		}
	}
	t.Fatalf("no output at %s", rel)
	return config.OutputFile{}
}

func hasBackfillOutput(outputs []config.OutputFile, base, rel string) bool {
	want := filepath.Join(base, filepath.FromSlash(rel))
	for _, o := range outputs {
		if o.Path == want {
			return true
		}
	}
	return false
}

func TestBackfill_MCPDocuments(t *testing.T) {
	cases := []struct {
		name       string
		gen        config.PresetGenerator
		path       string
		contains   []string
		notContain []string
	}{
		{
			name: "cursor writes .cursor/mcp.json",
			gen:  &CursorPresetGenerator{}, path: ".cursor/mcp.json",
			contains:   []string{`"mcpServers"`, `"command": "npx"`, `"url": "https://example.com/mcp"`, `"X-Key"`},
			notContain: []string{`"off"`, `"disabled"`},
		},
		{
			name: "copilot writes .vscode/mcp.json under servers with type",
			gen:  &CopilotPresetGenerator{}, path: ".vscode/mcp.json",
			contains:   []string{`"servers"`, `"type": "stdio"`, `"type": "http"`},
			notContain: []string{`"mcpServers"`, `"off"`},
		},
		{
			name: "devin writes .devin/mcp_config.json",
			gen:  &DevinPresetGenerator{}, path: ".devin/mcp_config.json",
			contains:   []string{`"mcpServers"`, `"args"`, `"url": "https://example.com/mcp"`},
			notContain: []string{`"off"`},
		},
		{
			name: "antigravity writes .agents/mcp_config.json with serverUrl",
			gen:  &AntigravityPresetGenerator{}, path: ".agents/mcp_config.json",
			contains:   []string{`"mcpServers"`, `"serverUrl": "https://example.com/mcp"`, `"ai-rulez"`},
			notContain: []string{`"url"`},
		},
		{
			name: "codex merges mcp_servers tables into .codex/config.toml",
			gen:  &CodexPresetGenerator{}, path: ".codex/config.toml",
			contains: []string{"[mcp_servers.local]", `command = "npx"`, "[mcp_servers.remote]",
				`url = "https://example.com/mcp"`, "http_headers", "[mcp_servers.off]", "enabled = false"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			outputs, err := tc.gen.Generate(backfillContent(), base, backfillConfig())
			require.NoError(t, err)

			doc := backfillOutput(t, outputs, base, tc.path)
			for _, want := range tc.contains {
				assert.Contains(t, doc.Content, want)
			}
			for _, bad := range tc.notContain {
				assert.NotContains(t, doc.Content, bad)
			}
			assert.False(t, doc.PartiallyOwned, "a fresh document is wholly ai-rulez's")
			assert.NotEmpty(t, doc.MergeClaims)
		})
	}
}

func TestBackfill_NoMCPServersNoDocuments(t *testing.T) {
	cfg := &config.Config{Name: "proj"}
	for rel, gen := range map[string]config.PresetGenerator{
		".cursor/mcp.json":        &CursorPresetGenerator{},
		".vscode/mcp.json":        &CopilotPresetGenerator{},
		".devin/mcp_config.json":  &DevinPresetGenerator{},
		".agents/mcp_config.json": &AntigravityPresetGenerator{},
		".codex/config.toml":      &CodexPresetGenerator{},
	} {
		base := t.TempDir()
		outputs, err := gen.Generate(&config.ContentTree{}, base, cfg)
		require.NoError(t, err)
		assert.False(t, hasBackfillOutput(outputs, base, rel), rel)
	}
}

// TestCodexConfigTOML_PreservesUserContent pins the point of the merged document:
// the user's own keys, tables and comments survive, and only ai-rulez's keys change.
func TestCodexConfigTOML_PreservesUserContent(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, ".codex", "config.toml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	existing := "# my settings\nmodel = \"gpt-5\"\n\n[mcp_servers.mine]\ncommand = \"mine\"\n\n[profiles.fast]\nmodel = \"x\"\n"
	require.NoError(t, os.WriteFile(path, []byte(existing), 0o644))

	cfg := backfillConfig()
	cfg.Defaults = &config.DefaultsConfig{Effort: "high"}
	outputs, err := (&CodexPresetGenerator{}).Generate(&config.ContentTree{}, base, cfg)
	require.NoError(t, err)

	doc := backfillOutput(t, outputs, base, ".codex/config.toml")
	assert.True(t, doc.PartiallyOwned)
	for _, keep := range []string{"# my settings", `model = "gpt-5"`, "[mcp_servers.mine]", `command = "mine"`, "[profiles.fast]"} {
		assert.Contains(t, doc.Content, keep)
	}
	assert.Contains(t, doc.Content, "[mcp_servers.local]")
	assert.Contains(t, doc.Content, "model_reasoning_effort")
}

func TestBackfill_CommandOutputs(t *testing.T) {
	cases := []struct {
		name     string
		gen      config.PresetGenerator
		path     string
		wantHas  []string
		wantNone []string
		absent   string
	}{
		{
			name: "opencode commands carry description frontmatter",
			gen:  &OpencodePresetGenerator{}, path: ".opencode/commands/ship.md",
			wantHas: []string{"---\n", "description: Ship a release", "Ship it."}, absent: ".opencode/commands/other-only.md",
		},
		{
			name: "antigravity workflows carry description frontmatter",
			gen:  &AntigravityPresetGenerator{}, path: ".agents/workflows/ship.md",
			wantHas: []string{"description: Ship a release", "Ship it."}, absent: ".agents/workflows/other-only.md",
		},
		{
			name: "cline workflows are plain markdown",
			gen:  &ClinePresetGenerator{}, path: ".clinerules/workflows/ship.md",
			wantHas: []string{"Ship it."}, wantNone: []string{"---"}, absent: ".clinerules/workflows/other-only.md",
		},
		{
			name: "devin commands become user-invocable skills",
			gen:  &DevinPresetGenerator{}, path: ".devin/skills/ship/SKILL.md",
			wantHas: []string{"name: ship", "description: Ship a release", "triggers:", "- user", "Ship it."}, absent: ".devin/skills/other-only/SKILL.md",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			outputs, err := tc.gen.Generate(backfillContent(), base, &config.Config{Name: "proj"})
			require.NoError(t, err)

			doc := backfillOutput(t, outputs, base, tc.path)
			for _, want := range tc.wantHas {
				assert.Contains(t, doc.Content, want)
			}
			for _, bad := range tc.wantNone {
				assert.NotContains(t, doc.Content, bad)
			}
			assert.False(t, hasBackfillOutput(outputs, base, tc.absent), "command targeted at another tool")
		})
	}
}

func TestDevinCommandSkill_SkillWithSameIDWins(t *testing.T) {
	content := &config.ContentTree{
		Skills:   []config.ContentFile{{Name: "ship", Path: "/p/.ai-rulez/skills/ship/SKILL.md", Content: "the skill"}},
		Commands: []config.ContentFile{{Name: "ship", Content: "the command"}},
	}
	base := t.TempDir()
	outputs, err := (&DevinPresetGenerator{}).Generate(content, base, &config.Config{Name: "proj"})
	require.NoError(t, err)

	skill := backfillOutput(t, outputs, base, ".devin/skills/ship/SKILL.md")
	assert.Contains(t, skill.Content, "the skill")
	assert.NotContains(t, skill.Content, "the command")
}

func TestGoPresets_GlobalOutputPaths(t *testing.T) {
	home := filepath.FromSlash("/home/u")
	abs := func(rel string) string { return filepath.Join(home, filepath.FromSlash(rel)) }

	cases := []struct {
		name string
		gen  GlobalOutputProvider
		want GlobalPaths
	}{
		{"codex", &CodexPresetGenerator{}, GlobalPaths{
			RootFile: abs(".codex/AGENTS.md"), SkillsDir: abs(".agents/skills"), AgentsDir: abs(".codex/agents"),
			CommandsDir: abs(".codex/prompts"), Sidecars: map[string]string{".codex/config.toml": abs(".codex/config.toml")},
		}},
		{"cursor", &CursorPresetGenerator{}, GlobalPaths{
			SkillsDir: abs(".cursor/skills"), AgentsDir: abs(".cursor/agents"), CommandsDir: abs(".cursor/commands"),
			Sidecars: map[string]string{".cursor/mcp.json": abs(".cursor/mcp.json")},
		}},
		{"copilot", &CopilotPresetGenerator{}, GlobalPaths{
			RootFile: abs(".copilot/copilot-instructions.md"), RulesDir: abs(".copilot/instructions"),
			SkillsDir: abs(".copilot/skills"), AgentsDir: abs(".copilot/agents"), Sidecars: map[string]string{},
		}},
		{"devin", &DevinPresetGenerator{}, GlobalPaths{
			RootFile: abs(".config/devin/AGENTS.md"), SkillsDir: abs(".config/devin/skills"), AgentsDir: abs(".config/devin/agents"),
			Sidecars: map[string]string{".devin/mcp_config.json": abs(".config/devin/mcp_config.json")},
		}},
		{"cline", &ClinePresetGenerator{}, GlobalPaths{
			RulesDir: abs("Documents/Cline/Rules"), SkillsDir: abs(".cline/skills"), AgentsDir: abs(".cline/agents"),
			CommandsDir: abs("Documents/Cline/Workflows"), Sidecars: map[string]string{},
		}},
		{"gemini", &GeminiPresetGenerator{}, GlobalPaths{
			RootFile: abs(".gemini/GEMINI.md"), SkillsDir: abs(".agents/skills"), AgentsDir: abs(".gemini/agents"),
			Sidecars: map[string]string{".gemini/settings.json": abs(".gemini/settings.json")},
		}},
		{"antigravity", &AntigravityPresetGenerator{}, GlobalPaths{
			RootFile: abs(".gemini/GEMINI.md"), RulesDir: abs(".gemini/config/rules"), SkillsDir: abs(".gemini/config/skills"),
			AgentsDir: abs(".gemini/config/agents"), CommandsDir: abs(".gemini/antigravity/global_workflows"),
			Sidecars: map[string]string{".agents/mcp_config.json": abs(".gemini/config/mcp_config.json")},
		}},
		{"opencode", &OpencodePresetGenerator{}, GlobalPaths{
			RootFile: abs(".config/opencode/AGENTS.md"), SkillsDir: abs(".config/opencode/skills"),
			AgentsDir: abs(".config/opencode/agents"), CommandsDir: abs(".config/opencode/commands"),
			Sidecars: map[string]string{"opencode.json": abs(".config/opencode/opencode.json")},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.gen.GlobalOutputPaths(home, func(string) string { return "" })
			require.NotNil(t, got)
			assert.Equal(t, tc.want, *got)
			assert.Nil(t, tc.gen.GlobalOutputPaths("relative/home", nil), "a relative home is rejected")
		})
	}
}

func TestCodexGlobalOutputPaths_HonoursCodexHome(t *testing.T) {
	home := filepath.FromSlash("/home/u")
	override := filepath.FromSlash("/data/codex")
	got := (&CodexPresetGenerator{}).GlobalOutputPaths(home, func(k string) string {
		if k == "CODEX_HOME" {
			return override
		}
		return ""
	})
	require.NotNil(t, got)
	assert.Equal(t, filepath.Join(override, "AGENTS.md"), got.RootFile)
	assert.Equal(t, filepath.Join(override, "config.toml"), got.Sidecars[".codex/config.toml"])
	assert.True(t, strings.HasPrefix(got.SkillsDir, home), "skills live outside CODEX_HOME: %s", got.SkillsDir)
}
