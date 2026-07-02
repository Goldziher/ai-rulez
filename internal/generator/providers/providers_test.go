package providers_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// claudeGen loads the embedded "claude" provider for testing. Fails fast if
// the embedded spec doesn't parse, which would mean a regression in the spec
// or the loader rather than a test setup issue.
func claudeGen(t *testing.T) *providers.Generator {
	t.Helper()
	gen, err := providers.LoadBuiltin("claude")
	require.NoError(t, err)
	return gen
}

func TestLoadBuiltin_Claude(t *testing.T) {
	t.Parallel()

	gen := claudeGen(t)
	assert.Equal(t, "claude", gen.GetName())

	paths := gen.GetOutputPaths("/repo")
	assert.Contains(t, paths, filepath.Join("/repo", "CLAUDE.md"))
	assert.Contains(t, paths, filepath.Join("/repo", ".claude"))
	assert.Contains(t, paths, filepath.Join("/repo", ".claude", "skills"))
	assert.Contains(t, paths, filepath.Join("/repo", ".claude", "agents"))
}

func TestBuiltinNames(t *testing.T) {
	t.Parallel()

	names, err := providers.BuiltinNames()
	require.NoError(t, err)
	assert.Contains(t, names, "claude")
}

// TestClaude_Generate verifies the output-count and structural-presence
// assertions previously held by presets.TestClaudePresetGenerator_Generate.
func TestClaude_Generate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		content     *config.ContentTree
		wantOutputs int
	}{
		{
			name: "generates skill and agent files",
			content: &config.ContentTree{
				Rules: []config.ContentFile{
					{Name: "rule1", Content: "Rule content", Metadata: &config.Metadata{Priority: "high"}},
				},
				Context: []config.ContentFile{
					{Name: "context1", Content: "Context content"},
				},
				Skills: []config.ContentFile{
					{
						Name:    "test-skill",
						Path:    "/test/skills/test-skill/SKILL.md",
						Content: "Skill content",
						Metadata: &config.Metadata{
							Priority: "medium",
							Extra:    map[string]string{"description": "Test skill"},
						},
					},
				},
			},
			wantOutputs: 6,
		},
		{
			name: "generates agents",
			content: &config.ContentTree{
				Agents: []config.ContentFile{
					{
						Name:    "test-agent",
						Content: "Agent system prompt",
						Metadata: &config.Metadata{
							Extra: map[string]string{
								"description": "Test agent description",
								"model":       "haiku",
							},
						},
					},
				},
			},
			wantOutputs: 5,
		},
		{
			name:        "handles no skills",
			content:     &config.ContentTree{},
			wantOutputs: 4,
		},
	}

	gen := claudeGen(t)
	cfg := &config.Config{Name: "test", Description: "test config"}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			outputs, err := gen.Generate(tc.content, "/test", cfg)
			require.NoError(t, err)
			require.Len(t, outputs, tc.wantOutputs)

			var hasClaudeDir, hasSkillsDir, hasAgentsDir, hasClaudeMD bool
			for _, o := range outputs {
				switch {
				case o.IsDir && strings.HasSuffix(o.Path, ".claude"):
					hasClaudeDir = true
				case o.IsDir && strings.HasSuffix(o.Path, filepath.Join(".claude", "skills")):
					hasSkillsDir = true
				case o.IsDir && strings.HasSuffix(o.Path, filepath.Join(".claude", "agents")):
					hasAgentsDir = true
				case !o.IsDir && strings.HasSuffix(o.Path, "CLAUDE.md"):
					hasClaudeMD = true
					assert.Contains(t, o.Content, "AI-RULEZ :: GENERATED FILE", "CLAUDE.md must carry the generated-file header")
				}
			}
			assert.True(t, hasClaudeMD, "CLAUDE.md should be emitted")
			assert.True(t, hasClaudeDir, ".claude directory entry should be emitted")
			assert.True(t, hasSkillsDir, ".claude/skills directory entry should be emitted")
			assert.True(t, hasAgentsDir, ".claude/agents directory entry should be emitted")
		})
	}
}

// TestClaude_SkillFile_NoHeader covers the "skill bodies are prompts, no HTML
// comment header" invariant from the legacy renderSkillFile test.
func TestClaude_SkillFile_NoHeader(t *testing.T) {
	t.Parallel()

	gen := claudeGen(t)
	skill := config.ContentFile{
		Name:    "test-skill",
		Path:    "/test/skills/test-skill/SKILL.md",
		Content: "# Test Skill\n\nThis is a test skill.",
		Metadata: &config.Metadata{
			Priority: "high",
			Targets:  []string{"claude", "cursor"},
			Extra:    map[string]string{"description": "A test skill"},
		},
	}
	content := &config.ContentTree{
		Rules: []config.ContentFile{
			{Name: "coding-standards", Content: "Follow best practices", Metadata: &config.Metadata{Priority: "critical"}},
		},
		Context: []config.ContentFile{
			{Name: "project-info", Content: "This is a test project"},
		},
		Skills: []config.ContentFile{skill},
	}
	cfg := &config.Config{Name: "test-project"}

	body := findSkillBody(t, gen, content, cfg, "/test", "test-skill")
	assert.NotContains(t, body, "<!--", "skill bodies must not carry HTML comment headers")
	assert.Contains(t, body, "---\n")
	assert.Contains(t, body, "# Test Skill")
	// Untargeted rules/context belong in CLAUDE.md, not duplicated into skill bodies.
	assert.NotContains(t, body, "## Rules")
	assert.NotContains(t, body, "### coding-standards")
	assert.NotContains(t, body, "## Context")
	assert.NotContains(t, body, "### project-info")
}

// TestClaude_SkillFile_TargetedFiltering covers the explicit-targets filter
// for rules and context embedded into skill bodies.
func TestClaude_SkillFile_TargetedFiltering(t *testing.T) {
	t.Parallel()

	gen := claudeGen(t)
	skill := config.ContentFile{
		Name:    "targeted-skill",
		Path:    "/test/skills/targeted-skill/SKILL.md",
		Content: "# Targeted Skill",
	}
	content := &config.ContentTree{
		Skills: []config.ContentFile{skill},
		Rules: []config.ContentFile{
			{Name: "included-rule", Content: "Should be included", Metadata: &config.Metadata{Targets: []string{".claude/skills/*/SKILL.md"}}},
			{Name: "excluded-rule", Content: "Should be excluded", Metadata: &config.Metadata{Targets: []string{"CLAUDE.md", ".cursor/rules/*"}}},
		},
		Context: []config.ContentFile{
			{Name: "included-context", Content: "Should be included", Metadata: &config.Metadata{Targets: []string{".claude/skills/*/SKILL.md"}}},
			{Name: "excluded-context", Content: "Should be excluded", Metadata: &config.Metadata{Targets: []string{"CLAUDE.md"}}},
		},
	}
	cfg := &config.Config{Name: "test", BaseDir: "/test"}

	body := findSkillBody(t, gen, content, cfg, "/test", "targeted-skill")
	assert.Contains(t, body, "### included-rule")
	assert.NotContains(t, body, "### excluded-rule")
	assert.Contains(t, body, "### included-context")
	assert.NotContains(t, body, "### excluded-context")
}

// TestClaude_SkillFile_OmitsSectionsWithNoTargetMatch covers the empty-list
// section-suppression behavior.
func TestClaude_SkillFile_OmitsSectionsWithNoTargetMatch(t *testing.T) {
	t.Parallel()

	gen := claudeGen(t)
	skill := config.ContentFile{
		Name:    "no-target-match-skill",
		Path:    "/test/skills/no-target-match-skill/SKILL.md",
		Content: "# No Target Match Skill",
	}
	content := &config.ContentTree{
		Skills: []config.ContentFile{skill},
		Rules: []config.ContentFile{
			{Name: "claude-md-only-rule", Content: "x", Metadata: &config.Metadata{Targets: []string{"CLAUDE.md"}}},
		},
		Context: []config.ContentFile{
			{Name: "claude-md-only-context", Content: "y", Metadata: &config.Metadata{Targets: []string{"CLAUDE.md"}}},
		},
	}
	cfg := &config.Config{Name: "test", BaseDir: "/test"}

	body := findSkillBody(t, gen, content, cfg, "/test", "no-target-match-skill")
	assert.NotContains(t, body, "\n## Rules\n", "Rules section must be omitted when nothing targets the skill")
	assert.NotContains(t, body, "\n## Context\n", "Context section must be omitted when nothing targets the skill")
}

// TestClaude_Generate_DomainCollections covers the domain-skill / domain-agent
// / domain-command / mixed-content assertions from the legacy tests.
func TestClaude_Generate_DomainCollections(t *testing.T) {
	t.Parallel()

	gen := claudeGen(t)
	cfg := &config.Config{Name: "test", Description: "test config"}

	t.Run("domain_skills", func(t *testing.T) {
		t.Parallel()
		content := &config.ContentTree{
			Domains: map[string]*config.Domain{
				"backend": {
					Name: "backend",
					Skills: []config.ContentFile{
						{Name: "domain-skill", Path: "/test/skills/domain-skill/SKILL.md", Content: "x"},
					},
				},
			},
		}
		outputs, err := gen.Generate(content, "/test", cfg)
		require.NoError(t, err)
		assert.Len(t, outputs, 6)
		assert.True(t, hasOutputPathSuffix(outputs, filepath.Join("domain-skill", "SKILL.md")))
	})

	t.Run("domain_agents", func(t *testing.T) {
		t.Parallel()
		content := &config.ContentTree{
			Domains: map[string]*config.Domain{
				"backend": {
					Name: "backend",
					Agents: []config.ContentFile{
						{Name: "domain-agent", Content: "x", Metadata: &config.Metadata{Extra: map[string]string{"description": "d"}}},
					},
				},
			},
		}
		outputs, err := gen.Generate(content, "/test", cfg)
		require.NoError(t, err)
		assert.Len(t, outputs, 5)
		assert.True(t, hasOutputPathSuffix(outputs, filepath.Join("agents", "domain-agent.md")))
	})

	t.Run("domain_commands", func(t *testing.T) {
		t.Parallel()
		content := &config.ContentTree{
			Domains: map[string]*config.Domain{
				"backend": {
					Name: "backend",
					Commands: []config.ContentFile{
						{Name: "domain-command", Content: "x", Metadata: &config.Metadata{Targets: []string{"claude"}}},
					},
				},
			},
		}
		outputs, err := gen.Generate(content, "/test", cfg)
		require.NoError(t, err)
		assert.Len(t, outputs, 6)
		assert.True(t, hasOutputPathSuffix(outputs, filepath.Join("domain-command", "SKILL.md")))
	})

	t.Run("root_and_domain_mixed", func(t *testing.T) {
		t.Parallel()
		content := &config.ContentTree{
			Skills:   []config.ContentFile{{Name: "root-skill", Path: "/test/skills/root-skill/SKILL.md", Content: "x"}},
			Agents:   []config.ContentFile{{Name: "root-agent", Content: "x"}},
			Commands: []config.ContentFile{{Name: "root-command", Content: "x", Metadata: &config.Metadata{Targets: []string{"claude"}}}},
			Domains: map[string]*config.Domain{
				"backend": {
					Name:     "backend",
					Skills:   []config.ContentFile{{Name: "domain-skill", Path: "/test/skills/domain-skill/SKILL.md", Content: "x"}},
					Agents:   []config.ContentFile{{Name: "domain-agent", Content: "x"}},
					Commands: []config.ContentFile{{Name: "domain-command", Content: "x", Metadata: &config.Metadata{Targets: []string{"claude"}}}},
				},
			},
		}
		outputs, err := gen.Generate(content, "/test", cfg)
		require.NoError(t, err)
		assert.Len(t, outputs, 14, "4 base + 2 skills*2 + 2 agents*1 + 2 commands*2 = 14")
		for _, name := range []string{"root-skill", "domain-skill", "root-agent", "domain-agent", "root-command", "domain-command"} {
			assert.True(t, hasOutputPathContains(outputs, name), "%s should be present in outputs", name)
		}
	})
}

// TestClaude_CLAUDEMD_InlinesContext covers the "all context is inlined, no
// `@path` references" behavior from the legacy renderClaudeMarkdown test.
func TestClaude_CLAUDEMD_InlinesContext(t *testing.T) {
	t.Parallel()

	gen := claudeGen(t)
	cfg := &config.Config{Name: "test", Description: "test config", BaseDir: "/test"}
	content := &config.ContentTree{
		Context: []config.ContentFile{
			{Name: "builtin-context", Path: "builtin://some-builtin/context.md", Content: "Inlined builtin content here"},
			{Name: "local-context", Path: "/test/context/local-context.md", Content: "Local context content"},
		},
	}

	outputs, err := gen.Generate(content, "/test", cfg)
	require.NoError(t, err)
	claudeMD := requireFile(t, outputs, "CLAUDE.md")
	assert.Contains(t, claudeMD.Content, "Inlined builtin content here")
	assert.Contains(t, claudeMD.Content, "Local context content")
	assert.NotContains(t, claudeMD.Content, "@context/local-context.md", "local context must inline, not reference")
}

// TestClaude_SettingsJSON covers the MCP-servers-aware settings.json sidecar
// (legacy renderSettingsJSON test).
func TestClaude_SettingsJSON(t *testing.T) {
	t.Parallel()

	gen := claudeGen(t)
	cfg := &config.Config{
		Name: "test",
		MCPServers: map[string]*config.MCPServer{
			"test-server": {Command: "npx", Args: []string{"-y", "test-server"}, Env: map[string]string{"KEY": "val"}},
			"http-server": {Command: "python", Transport: "http", URL: "http://localhost:8080"},
		},
	}
	outputs, err := gen.Generate(&config.ContentTree{}, "/test", cfg)
	require.NoError(t, err)

	settings := requireFile(t, outputs, filepath.Join(".claude", "settings.json"))
	assert.Contains(t, settings.Content, "test-server")
	assert.Contains(t, settings.Content, "http-server")
	assert.Contains(t, settings.Content, "mcpServers")
	assert.Contains(t, settings.Content, "npx")
	assert.Contains(t, settings.Content, "http://localhost:8080")

	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(settings.Content), &parsed))
	servers := parsed["mcpServers"].(map[string]any)
	assert.Len(t, servers, 2)

	// Remote (http/sse) servers must use Claude Code's `type` key, carry no
	// command, and must not emit the `transport` key. See
	// https://code.claude.com/docs/en/mcp.
	httpEntry := servers["http-server"].(map[string]any)
	assert.Equal(t, "http", httpEntry["type"], "remote server must use the `type` key")
	assert.Equal(t, "http://localhost:8080", httpEntry["url"])
	assert.NotContains(t, httpEntry, "transport", "must not emit `transport`; Claude Code keys on `type`")
	assert.NotContains(t, httpEntry, "command", "remote server must not carry a command")

	// stdio servers are unchanged: command-based, no `type` key.
	stdioEntry := servers["test-server"].(map[string]any)
	assert.Equal(t, "npx", stdioEntry["command"], "stdio server keeps its command")
	assert.NotContains(t, stdioEntry, "type", "stdio server must not emit a `type` key")
}

// TestClaude_PluginsJSON covers the plugins-aware plugins.json sidecar
// (legacy renderPluginsJSON test).
func TestClaude_PluginsJSON(t *testing.T) {
	t.Parallel()

	gen := claudeGen(t)
	enabled := true
	cfg := &config.Config{
		Name: "test",
		Plugins: []config.PluginConfig{
			{Marketplace: "official", Name: "github", Scope: "project", Enabled: &enabled},
			{Marketplace: "custom", Name: "tool", Scope: "user"},
		},
	}
	outputs, err := gen.Generate(&config.ContentTree{}, "/test", cfg)
	require.NoError(t, err)

	pluginsFile := requireFile(t, outputs, filepath.Join(".claude", "plugins.json"))
	assert.Contains(t, pluginsFile.Content, "official")
	assert.Contains(t, pluginsFile.Content, "github")
	assert.Contains(t, pluginsFile.Content, "custom")

	var parsed []any
	require.NoError(t, json.Unmarshal([]byte(pluginsFile.Content), &parsed))
	assert.Len(t, parsed, 2)
}

// TestClaude_Agent_EffortFrontmatter covers the agent-effort resolver chain
// (legacy buildAgentFrontmatter_Effort table-driven test).
func TestClaude_Agent_EffortFrontmatter(t *testing.T) {
	t.Parallel()

	gen := claudeGen(t)
	cases := []struct {
		name          string
		agent         config.ContentFile
		cfg           *config.Config
		wantEffort    string
		wantHasEffort bool
	}{
		{
			name:          "agent effort wins over default",
			agent:         config.ContentFile{Name: "reviewer", Metadata: &config.Metadata{Effort: "high"}},
			cfg:           &config.Config{Defaults: &config.DefaultsConfig{Effort: "medium"}},
			wantEffort:    "high",
			wantHasEffort: true,
		},
		{
			name:          "default fills in when agent has no effort",
			agent:         config.ContentFile{Name: "noop", Metadata: &config.Metadata{}},
			cfg:           &config.Config{Defaults: &config.DefaultsConfig{Effort: "medium"}},
			wantEffort:    "medium",
			wantHasEffort: true,
		},
		{
			name:          "default applies when metadata is nil",
			agent:         config.ContentFile{Name: "bare"},
			cfg:           &config.Config{Defaults: &config.DefaultsConfig{Effort: "low"}},
			wantEffort:    "low",
			wantHasEffort: true,
		},
		{
			name:          "no effort emitted when neither set",
			agent:         config.ContentFile{Name: "plain", Metadata: &config.Metadata{}},
			cfg:           &config.Config{},
			wantHasEffort: false,
		},
		{
			name:          "agent effort respected when no defaults block at all",
			agent:         config.ContentFile{Name: "solo", Metadata: &config.Metadata{Effort: "max"}},
			cfg:           &config.Config{},
			wantEffort:    "max",
			wantHasEffort: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			content := &config.ContentTree{Agents: []config.ContentFile{tc.agent}}
			outputs, err := gen.Generate(content, "/test", tc.cfg)
			require.NoError(t, err)

			agentFile := requireFile(t, outputs, filepath.Join("agents", tc.agent.Name+".md"))
			line := frontmatterValue(agentFile.Content, "effort")
			if tc.wantHasEffort {
				assert.Equal(t, tc.wantEffort, line, "expected effort %q in agent frontmatter", tc.wantEffort)
			} else {
				assert.Empty(t, line, "no effort field should appear in agent frontmatter")
			}
		})
	}
}

// TestClaude_PreservesSkillResourcesLayout covers the skill-resources bundling
// behavior (legacy PreservesSkillResourcesLayout test).
func TestClaude_PreservesSkillResourcesLayout(t *testing.T) {
	t.Parallel()

	gen := claudeGen(t)
	binaryAsset := []byte{0x00, 0xff, 0x10, 0x20}

	skill := config.ContentFile{
		Name:    "demo",
		Path:    "/source/skills/demo/SKILL.md",
		Content: "Body of skill.",
		Metadata: &config.Metadata{
			Extra: map[string]string{"description": "demo skill"},
		},
		Resources: []config.SkillResource{
			{
				Kind:        config.SkillKindReferences,
				RelPath:     "references/api.md",
				Content:     []byte("---\ndescription: API reference\n---\n\nAPI body.\n"),
				Description: "API reference",
			},
			{Kind: config.SkillKindScripts, RelPath: "scripts/run.sh", Content: []byte("#!/bin/sh\necho hi\n")},
			{Kind: config.SkillKindAssets, RelPath: "assets/blob.bin", Content: binaryAsset},
		},
	}
	cfg := &config.Config{Name: "demo-cfg"}
	outputs, err := gen.Generate(&config.ContentTree{Skills: []config.ContentFile{skill}}, "/test", cfg)
	require.NoError(t, err)

	skillDir := filepath.Join("/test", ".claude", "skills", "demo")
	files := make(map[string]config.OutputFile)
	for _, o := range outputs {
		if !o.IsDir {
			files[o.Path] = o
		}
	}

	skillMD, ok := files[filepath.Join(skillDir, "SKILL.md")]
	require.True(t, ok)
	assert.Contains(t, skillMD.Content, "Body of skill.")
	assert.Contains(t, skillMD.Content, "## Resources")
	assert.Contains(t, skillMD.Content, "[`references/api.md`](references/api.md)")
	assert.Contains(t, skillMD.Content, "API reference")
	assert.NotContains(t, skillMD.Content, "API body.", "reference content must not be inlined into SKILL.md")

	apiFile, ok := files[filepath.Join(skillDir, "references", "api.md")]
	require.True(t, ok)
	assert.Equal(t, []byte("---\ndescription: API reference\n---\n\nAPI body.\n"), apiFile.RawContent)
	assert.Empty(t, apiFile.Content, "reference must use RawContent, not Content")

	scriptFile, ok := files[filepath.Join(skillDir, "scripts", "run.sh")]
	require.True(t, ok)
	assert.Equal(t, []byte("#!/bin/sh\necho hi\n"), scriptFile.RawContent)

	blobFile, ok := files[filepath.Join(skillDir, "assets", "blob.bin")]
	require.True(t, ok)
	assert.Equal(t, binaryAsset, blobFile.RawContent)
}

// TestClaude_PerPresetModel covers the `claude_model:` agent override (and the
// `defaults.model_by_preset[claude]` fallback) — moved here when claude.go
// migrated to the DSL renderer.
func TestClaude_PerPresetModel(t *testing.T) {
	t.Parallel()

	gen := claudeGen(t)

	t.Run("per_agent_override_wins", func(t *testing.T) {
		t.Parallel()
		agent := config.ContentFile{
			Name: "research-helper",
			Metadata: &config.Metadata{Extra: map[string]string{
				"claude_model":  "opus",
				"copilot_model": "gpt-5",
				"description":   "Research helper",
			}},
		}
		outputs, err := gen.Generate(&config.ContentTree{Agents: []config.ContentFile{agent}}, "/test", &config.Config{})
		require.NoError(t, err)
		agentFile := requireFile(t, outputs, filepath.Join("agents", "research-helper.md"))
		assert.Equal(t, "opus", frontmatterValue(agentFile.Content, "model"))
	})

	t.Run("defaults_apply_without_frontmatter", func(t *testing.T) {
		t.Parallel()
		agent := config.ContentFile{Name: "bare"}
		cfg := &config.Config{Defaults: &config.DefaultsConfig{
			ModelByPreset: map[string]string{"claude": "haiku"},
		}}
		outputs, err := gen.Generate(&config.ContentTree{Agents: []config.ContentFile{agent}}, "/test", cfg)
		require.NoError(t, err)
		agentFile := requireFile(t, outputs, filepath.Join("agents", "bare.md"))
		assert.Equal(t, "haiku", frontmatterValue(agentFile.Content, "model"))
	})
}

// --- helpers ---

func findSkillBody(t *testing.T, gen *providers.Generator, content *config.ContentTree, cfg *config.Config, baseDir, skillID string) string {
	t.Helper()
	outputs, err := gen.Generate(content, baseDir, cfg)
	require.NoError(t, err)
	skillPath := filepath.Join(baseDir, ".claude", "skills", skillID, "SKILL.md")
	for _, o := range outputs {
		if o.Path == skillPath {
			return o.Content
		}
	}
	t.Fatalf("skill %q not found in outputs", skillID)
	return ""
}

func requireFile(t *testing.T, outputs []config.OutputFile, suffix string) config.OutputFile {
	t.Helper()
	for _, o := range outputs {
		if !o.IsDir && strings.HasSuffix(o.Path, suffix) {
			return o
		}
	}
	t.Fatalf("no output with suffix %q", suffix)
	return config.OutputFile{}
}

func hasOutputPathSuffix(outputs []config.OutputFile, suffix string) bool {
	for _, o := range outputs {
		if strings.HasSuffix(o.Path, suffix) {
			return true
		}
	}
	return false
}

func hasOutputPathContains(outputs []config.OutputFile, fragment string) bool {
	for _, o := range outputs {
		if strings.Contains(o.Path, fragment) {
			return true
		}
	}
	return false
}

// frontmatterValue returns the string value for `key` in the leading YAML
// frontmatter block, or "" when absent. Lightweight — handles only top-level
// scalar entries which is all this test needs.
func frontmatterValue(content, key string) string {
	if !strings.HasPrefix(content, "---\n") {
		return ""
	}
	end := strings.Index(content[4:], "\n---\n")
	if end < 0 {
		return ""
	}
	for _, line := range strings.Split(content[4:4+end], "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, key+":") {
			return strings.TrimSpace(strings.TrimPrefix(line, key+":"))
		}
	}
	return ""
}
