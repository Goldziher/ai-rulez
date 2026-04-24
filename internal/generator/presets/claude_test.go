package presets

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudePresetGenerator_Generate(t *testing.T) {
	tests := []struct {
		name        string
		content     *config.ContentTreeV3
		baseDir     string
		wantOutputs int
		wantErr     bool
	}{
		{
			name: "generates skill and agent files",
			content: &config.ContentTreeV3{
				Rules: []config.ContentFile{
					{
						Name:    "rule1",
						Content: "Rule content",
						Metadata: &config.MetadataV3{
							Priority: "high",
						},
					},
				},
				Context: []config.ContentFile{
					{
						Name:    "context1",
						Content: "Context content",
					},
				},
				Skills: []config.ContentFile{
					{
						Name:    "test-skill",
						Path:    "/test/skills/test-skill/SKILL.md",
						Content: "Skill content",
						Metadata: &config.MetadataV3{
							Priority: "medium",
							Extra: map[string]string{
								"description": "Test skill",
							},
						},
					},
				},
			},
			baseDir:     "/test",
			wantOutputs: 6, // CLAUDE.md, .claude dir, skills dir, agents dir, skill-id dir, skill/SKILL.md
			wantErr:     false,
		},
		{
			name: "generates agents",
			content: &config.ContentTreeV3{
				Agents: []config.ContentFile{
					{
						Name:    "test-agent",
						Content: "Agent system prompt",
						Metadata: &config.MetadataV3{
							Extra: map[string]string{
								"description": "Test agent description",
								"model":       "haiku",
							},
						},
					},
				},
			},
			baseDir:     "/test",
			wantOutputs: 5, // CLAUDE.md, .claude dir, skills dir, agents dir, agents/test-agent.md
			wantErr:     false,
		},
		{
			name: "handles no skills",
			content: &config.ContentTreeV3{
				Rules:   []config.ContentFile{},
				Context: []config.ContentFile{},
				Skills:  []config.ContentFile{},
			},
			baseDir:     "/test",
			wantOutputs: 4, // CLAUDE.md, .claude dir, skills dir, agents dir
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &ClaudePresetGenerator{}
			cfg := &config.ConfigV3{
				Name:        "test",
				Description: "test config",
			}

			outputs, err := g.Generate(tt.content, tt.baseDir, cfg)

			if (err != nil) != tt.wantErr {
				t.Errorf("Generate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if len(outputs) != tt.wantOutputs {
				t.Errorf("Generate() got %d outputs, want %d", len(outputs), tt.wantOutputs)
			}

			// Verify directory outputs
			hasClaudeDir := false
			hasSkillsDir := false
			hasAgentsDir := false
			hasClaudeMD := false

			for _, output := range outputs {
				if output.IsDir {
					switch {
					case strings.HasSuffix(output.Path, ".claude"):
						hasClaudeDir = true
					case strings.HasSuffix(output.Path, filepath.Join(".claude", "skills")):
						hasSkillsDir = true
					case strings.HasSuffix(output.Path, filepath.Join(".claude", "agents")):
						hasAgentsDir = true
					}
				} else if strings.HasSuffix(output.Path, "CLAUDE.md") {
					hasClaudeMD = true
					// Verify CLAUDE.md has header
					if !strings.Contains(output.Content, "AI-RULEZ :: GENERATED FILE") {
						t.Error("Expected header in CLAUDE.md")
					}
				}
			}

			if !hasClaudeMD {
				t.Error("Expected CLAUDE.md file output")
			}
			if !hasClaudeDir {
				t.Error("Expected .claude directory output")
			}
			if !hasSkillsDir {
				t.Error("Expected .claude/skills directory output")
			}
			if !hasAgentsDir {
				t.Error("Expected .claude/agents directory output")
			}
		})
	}
}

func TestClaudePresetGenerator_renderSkillFile(t *testing.T) {
	g := &ClaudePresetGenerator{}

	skill := config.ContentFile{
		Name:    "test-skill",
		Path:    "/test/skills/test-skill/SKILL.md",
		Content: "# Test Skill\n\nThis is a test skill.",
		Metadata: &config.MetadataV3{
			Priority: "high",
			Targets:  []string{"claude", "cursor"},
			Extra: map[string]string{
				"description": "A test skill",
			},
		},
	}

	content := &config.ContentTreeV3{
		Rules: []config.ContentFile{
			{
				Name:    "coding-standards",
				Content: "Follow best practices",
				Metadata: &config.MetadataV3{
					Priority: "critical",
				},
			},
		},
		Context: []config.ContentFile{
			{
				Name:    "project-info",
				Content: "This is a test project",
			},
		},
	}

	cfg := &config.ConfigV3{
		Name:        "test-project",
		Description: "Test project description",
	}

	result, err := g.renderSkillFile(skill, content, cfg, ".claude/skills/test-skill/SKILL.md")
	if err != nil {
		t.Fatalf("renderSkillFile() error = %v", err)
	}

	// Skills/agents/commands should NOT have header comments — the body is a prompt
	if strings.Contains(result, "<!--") {
		t.Error("Skill files should not contain HTML comment headers (body is a prompt)")
	}

	// Check frontmatter
	if !strings.Contains(result, "---\n") {
		t.Error("Expected frontmatter in file")
	}

	// Check skill content
	if !strings.Contains(result, "# Test Skill") {
		t.Error("Expected skill content in output")
	}

	// Untargeted rules and context should NOT be embedded in skill files
	// (they are already in CLAUDE.md)
	if strings.Contains(result, "## Rules") {
		t.Error("Expected untargeted Rules section to be omitted from skill file")
	}
	if strings.Contains(result, "### coding-standards") {
		t.Error("Expected untargeted rule to be omitted from skill file")
	}

	if strings.Contains(result, "## Context") {
		t.Error("Expected untargeted Context section to be omitted from skill file")
	}
	if strings.Contains(result, "### project-info") {
		t.Error("Expected untargeted context to be omitted from skill file")
	}
}

func TestClaudePresetGenerator_renderSkillFile_FiltersEmbeddedContentByTargets(t *testing.T) {
	g := &ClaudePresetGenerator{}

	skill := config.ContentFile{
		Name:    "targeted-skill",
		Path:    "/test/skills/targeted-skill/SKILL.md",
		Content: "# Targeted Skill",
	}

	content := &config.ContentTreeV3{
		Rules: []config.ContentFile{
			{
				Name:    "included-rule",
				Content: "This rule should be included",
				Metadata: &config.MetadataV3{
					Targets: []string{".claude/skills/*/SKILL.md"},
				},
			},
			{
				Name:    "excluded-rule",
				Content: "This rule should be excluded",
				Metadata: &config.MetadataV3{
					Targets: []string{"CLAUDE.md", ".cursor/rules/*"},
				},
			},
		},
		Context: []config.ContentFile{
			{
				Name:    "included-context",
				Content: "This context should be included",
				Metadata: &config.MetadataV3{
					Targets: []string{".claude/skills/*/SKILL.md"},
				},
			},
			{
				Name:    "excluded-context",
				Content: "This context should be excluded",
				Metadata: &config.MetadataV3{
					Targets: []string{"CLAUDE.md"},
				},
			},
		},
	}

	cfg := &config.ConfigV3{
		Name:    "test-project",
		BaseDir: "/test",
	}

	result, err := g.renderSkillFile(skill, content, cfg, "/test/.claude/skills/targeted-skill/SKILL.md")
	if err != nil {
		t.Fatalf("renderSkillFile() error = %v", err)
	}

	if !strings.Contains(result, "### included-rule") {
		t.Error("Expected included rule in output")
	}
	if strings.Contains(result, "### excluded-rule") {
		t.Error("Expected excluded rule to be filtered out")
	}
	if !strings.Contains(result, "### included-context") {
		t.Error("Expected included context in output")
	}
	if strings.Contains(result, "### excluded-context") {
		t.Error("Expected excluded context to be filtered out")
	}
}

func TestClaudePresetGenerator_renderSkillFile_OmitsSectionsWhenNoEmbeddedContentMatchesTargets(t *testing.T) {
	g := &ClaudePresetGenerator{}

	skill := config.ContentFile{
		Name:    "no-target-match-skill",
		Path:    "/test/skills/no-target-match-skill/SKILL.md",
		Content: "# No Target Match Skill",
	}

	content := &config.ContentTreeV3{
		Rules: []config.ContentFile{
			{
				Name:    "claude-md-only-rule",
				Content: "Rule content",
				Metadata: &config.MetadataV3{
					Targets: []string{"CLAUDE.md"},
				},
			},
		},
		Context: []config.ContentFile{
			{
				Name:    "claude-md-only-context",
				Content: "Context content",
				Metadata: &config.MetadataV3{
					Targets: []string{"CLAUDE.md"},
				},
			},
		},
	}

	cfg := &config.ConfigV3{
		Name:    "test-project",
		BaseDir: "/test",
	}

	result, err := g.renderSkillFile(skill, content, cfg, "/test/.claude/skills/no-target-match-skill/SKILL.md")
	if err != nil {
		t.Fatalf("renderSkillFile() error = %v", err)
	}

	if strings.Contains(result, "\n## Rules\n") {
		t.Error("Expected Rules section to be omitted when no rules match targets")
	}
	if strings.Contains(result, "\n## Context\n") {
		t.Error("Expected Context section to be omitted when no context matches targets")
	}
}

func TestClaudePresetGenerator_Generate_CollectsDomainSkills(t *testing.T) {
	g := &ClaudePresetGenerator{}
	cfg := &config.ConfigV3{
		Name:        "test",
		Description: "test config",
	}

	content := &config.ContentTreeV3{
		Domains: map[string]*config.DomainV3{
			"backend": {
				Name: "backend",
				Skills: []config.ContentFile{
					{
						Name:    "domain-skill",
						Path:    "/test/skills/domain-skill/SKILL.md",
						Content: "Domain skill content",
					},
				},
			},
		},
	}

	outputs, err := g.Generate(content, "/test", cfg)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Expect: 4 base (CLAUDE.md, .claude, skills, agents) + 2 for domain skill (dir + SKILL.md) = 6
	if len(outputs) != 6 {
		t.Errorf("Generate() got %d outputs, want 6", len(outputs))
	}

	// Verify the domain skill file was generated
	hasSkillFile := false
	for _, output := range outputs {
		if strings.HasSuffix(output.Path, filepath.Join("domain-skill", "SKILL.md")) {
			hasSkillFile = true
			break
		}
	}
	if !hasSkillFile {
		t.Error("Expected domain skill SKILL.md to be generated")
	}
}

func TestClaudePresetGenerator_Generate_CollectsDomainAgents(t *testing.T) {
	g := &ClaudePresetGenerator{}
	cfg := &config.ConfigV3{
		Name:        "test",
		Description: "test config",
	}

	content := &config.ContentTreeV3{
		Domains: map[string]*config.DomainV3{
			"backend": {
				Name: "backend",
				Agents: []config.ContentFile{
					{
						Name:    "domain-agent",
						Content: "Domain agent prompt",
						Metadata: &config.MetadataV3{
							Extra: map[string]string{
								"description": "A domain agent",
							},
						},
					},
				},
			},
		},
	}

	outputs, err := g.Generate(content, "/test", cfg)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Expect: 4 base + 1 agent file = 5
	if len(outputs) != 5 {
		t.Errorf("Generate() got %d outputs, want 5", len(outputs))
	}

	hasAgentFile := false
	for _, output := range outputs {
		if strings.HasSuffix(output.Path, filepath.Join("agents", "domain-agent.md")) {
			hasAgentFile = true
			break
		}
	}
	if !hasAgentFile {
		t.Error("Expected domain agent file to be generated")
	}
}

func TestClaudePresetGenerator_Generate_CollectsDomainCommands(t *testing.T) {
	g := &ClaudePresetGenerator{}
	cfg := &config.ConfigV3{
		Name:        "test",
		Description: "test config",
	}

	content := &config.ContentTreeV3{
		Domains: map[string]*config.DomainV3{
			"backend": {
				Name: "backend",
				Commands: []config.ContentFile{
					{
						Name:    "domain-command",
						Content: "Domain command content",
						Metadata: &config.MetadataV3{
							Targets: []string{"claude"},
						},
					},
				},
			},
		},
	}

	outputs, err := g.Generate(content, "/test", cfg)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Expect: 4 base + 2 for command-as-skill (dir + SKILL.md) = 6
	if len(outputs) != 6 {
		t.Errorf("Generate() got %d outputs, want 6", len(outputs))
	}

	hasCommandSkillFile := false
	for _, output := range outputs {
		if strings.HasSuffix(output.Path, filepath.Join("domain-command", "SKILL.md")) {
			hasCommandSkillFile = true
			break
		}
	}
	if !hasCommandSkillFile {
		t.Error("Expected domain command to be generated as skill file")
	}
}

func TestClaudePresetGenerator_Generate_CollectsDomainAndRootContent(t *testing.T) {
	g := &ClaudePresetGenerator{}
	cfg := &config.ConfigV3{
		Name:        "test",
		Description: "test config",
	}

	content := &config.ContentTreeV3{
		Skills: []config.ContentFile{
			{
				Name:    "root-skill",
				Path:    "/test/skills/root-skill/SKILL.md",
				Content: "Root skill content",
			},
		},
		Agents: []config.ContentFile{
			{
				Name:    "root-agent",
				Content: "Root agent prompt",
			},
		},
		Commands: []config.ContentFile{
			{
				Name:    "root-command",
				Content: "Root command content",
				Metadata: &config.MetadataV3{
					Targets: []string{"claude"},
				},
			},
		},
		Domains: map[string]*config.DomainV3{
			"backend": {
				Name: "backend",
				Skills: []config.ContentFile{
					{
						Name:    "domain-skill",
						Path:    "/test/skills/domain-skill/SKILL.md",
						Content: "Domain skill content",
					},
				},
				Agents: []config.ContentFile{
					{
						Name:    "domain-agent",
						Content: "Domain agent prompt",
					},
				},
				Commands: []config.ContentFile{
					{
						Name:    "domain-command",
						Content: "Domain command content",
						Metadata: &config.MetadataV3{
							Targets: []string{"claude"},
						},
					},
				},
			},
		},
	}

	outputs, err := g.Generate(content, "/test", cfg)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// 4 base + 2 skills * 2 (dir+file) + 2 agents * 1 + 2 commands * 2 (dir+file) = 4 + 4 + 2 + 4 = 14
	if len(outputs) != 14 {
		t.Errorf("Generate() got %d outputs, want 14", len(outputs))
	}

	// Verify both root and domain content is present
	foundPaths := make(map[string]bool)
	for _, output := range outputs {
		if strings.Contains(output.Path, "root-skill") {
			foundPaths["root-skill"] = true
		}
		if strings.Contains(output.Path, "domain-skill") {
			foundPaths["domain-skill"] = true
		}
		if strings.Contains(output.Path, "root-agent") {
			foundPaths["root-agent"] = true
		}
		if strings.Contains(output.Path, "domain-agent") {
			foundPaths["domain-agent"] = true
		}
		if strings.Contains(output.Path, "root-command") {
			foundPaths["root-command"] = true
		}
		if strings.Contains(output.Path, "domain-command") {
			foundPaths["domain-command"] = true
		}
	}

	for _, name := range []string{"root-skill", "domain-skill", "root-agent", "domain-agent", "root-command", "domain-command"} {
		if !foundPaths[name] {
			t.Errorf("Expected %s to be present in outputs", name)
		}
	}
}

func TestClaudePresetGenerator_renderClaudeMarkdown_InlinesBuiltinContext(t *testing.T) {
	g := &ClaudePresetGenerator{}
	cfg := &config.ConfigV3{
		Name:        "test",
		Description: "test config",
		BaseDir:     "/test",
	}

	content := &config.ContentTreeV3{
		Context: []config.ContentFile{
			{
				Name:    "builtin-context",
				Path:    "builtin://some-builtin/context.md",
				Content: "Inlined builtin content here",
			},
			{
				Name:    "local-context",
				Path:    "/test/context/local-context.md",
				Content: "Local context content",
			},
		},
	}

	result := g.renderClaudeMarkdown(content, cfg)

	// Builtin context should be inlined (content appears directly)
	if !strings.Contains(result, "Inlined builtin content here") {
		t.Error("Expected builtin context content to be inlined in output")
	}

	// Local context should also be inlined (not use @ reference)
	if !strings.Contains(result, "Local context content") {
		t.Error("Expected local context content to be inlined in output")
	}

	// Local context should NOT use @ reference
	if strings.Contains(result, "@context/local-context.md") {
		t.Error("Expected local context to NOT use @ path reference")
	}
}

func TestClaudePresetGenerator_GetName(t *testing.T) {
	g := &ClaudePresetGenerator{}
	if g.GetName() != "claude" {
		t.Errorf("GetName() = %v, want %v", g.GetName(), "claude")
	}
}

func TestClaudePresetGenerator_GetOutputPaths(t *testing.T) {
	g := &ClaudePresetGenerator{}
	baseDir := "/test"
	paths := g.GetOutputPaths(baseDir)

	expectedPaths := []string{
		filepath.Join(baseDir, "CLAUDE.md"),
		filepath.Join(baseDir, ".claude"),
		filepath.Join(baseDir, ".claude", "skills"),
		filepath.Join(baseDir, ".claude", "agents"),
	}

	if len(paths) != len(expectedPaths) {
		t.Errorf("GetOutputPaths() returned %d paths, want %d", len(paths), len(expectedPaths))
	}

	for i, expected := range expectedPaths {
		if i >= len(paths) {
			break
		}
		if paths[i] != expected {
			t.Errorf("GetOutputPaths()[%d] = %v, want %v", i, paths[i], expected)
		}
	}
}

func TestClaudePresetGenerator_renderSettingsJSON(t *testing.T) {
	g := &ClaudePresetGenerator{}

	cfg := &config.ConfigV3{
		MCPServers: map[string]*config.MCPServerV3{
			"test-server": {
				Command: "npx",
				Args:    []string{"-y", "test-server"},
				Env:     map[string]string{"KEY": "val"},
			},
			"http-server": {
				Command:   "python",
				Transport: "http",
				URL:       "http://localhost:8080",
			},
		},
	}

	content, err := g.renderSettingsJSON(cfg)
	require.NoError(t, err)
	assert.Contains(t, content, "test-server")
	assert.Contains(t, content, "http-server")
	assert.Contains(t, content, "mcpServers")
	assert.Contains(t, content, "npx")
	assert.Contains(t, content, "http://localhost:8080")

	// Verify it's valid JSON
	var parsed map[string]interface{}
	err = json.Unmarshal([]byte(content), &parsed)
	require.NoError(t, err)
	servers := parsed["mcpServers"].(map[string]interface{})
	assert.Len(t, servers, 2)
}

func TestClaudePresetGenerator_renderPluginsJSON(t *testing.T) {
	g := &ClaudePresetGenerator{}

	enabled := true
	cfg := &config.ConfigV3{
		Plugins: []config.PluginConfig{
			{Marketplace: "official", Name: "github", Scope: "project", Enabled: &enabled},
			{Marketplace: "custom", Name: "tool", Scope: "user"},
		},
	}

	content, err := g.renderPluginsJSON(cfg)
	require.NoError(t, err)
	assert.Contains(t, content, "official")
	assert.Contains(t, content, "github")
	assert.Contains(t, content, "custom")

	// Verify valid JSON array
	var parsed []interface{}
	err = json.Unmarshal([]byte(content), &parsed)
	require.NoError(t, err)
	assert.Len(t, parsed, 2)
}
