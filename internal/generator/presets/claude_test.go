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
		content     *config.ContentTree
		baseDir     string
		wantOutputs int
		wantErr     bool
	}{
		{
			name: "generates skill and agent files",
			content: &config.ContentTree{
				Rules: []config.ContentFile{
					{
						Name:    "rule1",
						Content: "Rule content",
						Metadata: &config.Metadata{
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
						Metadata: &config.Metadata{
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
			baseDir:     "/test",
			wantOutputs: 5, // CLAUDE.md, .claude dir, skills dir, agents dir, agents/test-agent.md
			wantErr:     false,
		},
		{
			name: "handles no skills",
			content: &config.ContentTree{
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
			cfg := &config.Config{
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
		Metadata: &config.Metadata{
			Priority: "high",
			Targets:  []string{"claude", "cursor"},
			Extra: map[string]string{
				"description": "A test skill",
			},
		},
	}

	content := &config.ContentTree{
		Rules: []config.ContentFile{
			{
				Name:    "coding-standards",
				Content: "Follow best practices",
				Metadata: &config.Metadata{
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

	cfg := &config.Config{
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

	content := &config.ContentTree{
		Rules: []config.ContentFile{
			{
				Name:    "included-rule",
				Content: "This rule should be included",
				Metadata: &config.Metadata{
					Targets: []string{".claude/skills/*/SKILL.md"},
				},
			},
			{
				Name:    "excluded-rule",
				Content: "This rule should be excluded",
				Metadata: &config.Metadata{
					Targets: []string{"CLAUDE.md", ".cursor/rules/*"},
				},
			},
		},
		Context: []config.ContentFile{
			{
				Name:    "included-context",
				Content: "This context should be included",
				Metadata: &config.Metadata{
					Targets: []string{".claude/skills/*/SKILL.md"},
				},
			},
			{
				Name:    "excluded-context",
				Content: "This context should be excluded",
				Metadata: &config.Metadata{
					Targets: []string{"CLAUDE.md"},
				},
			},
		},
	}

	cfg := &config.Config{
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

	content := &config.ContentTree{
		Rules: []config.ContentFile{
			{
				Name:    "claude-md-only-rule",
				Content: "Rule content",
				Metadata: &config.Metadata{
					Targets: []string{"CLAUDE.md"},
				},
			},
		},
		Context: []config.ContentFile{
			{
				Name:    "claude-md-only-context",
				Content: "Context content",
				Metadata: &config.Metadata{
					Targets: []string{"CLAUDE.md"},
				},
			},
		},
	}

	cfg := &config.Config{
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
	cfg := &config.Config{
		Name:        "test",
		Description: "test config",
	}

	content := &config.ContentTree{
		Domains: map[string]*config.Domain{
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
	cfg := &config.Config{
		Name:        "test",
		Description: "test config",
	}

	content := &config.ContentTree{
		Domains: map[string]*config.Domain{
			"backend": {
				Name: "backend",
				Agents: []config.ContentFile{
					{
						Name:    "domain-agent",
						Content: "Domain agent prompt",
						Metadata: &config.Metadata{
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
	cfg := &config.Config{
		Name:        "test",
		Description: "test config",
	}

	content := &config.ContentTree{
		Domains: map[string]*config.Domain{
			"backend": {
				Name: "backend",
				Commands: []config.ContentFile{
					{
						Name:    "domain-command",
						Content: "Domain command content",
						Metadata: &config.Metadata{
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
	cfg := &config.Config{
		Name:        "test",
		Description: "test config",
	}

	content := &config.ContentTree{
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
				Metadata: &config.Metadata{
					Targets: []string{"claude"},
				},
			},
		},
		Domains: map[string]*config.Domain{
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
						Metadata: &config.Metadata{
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
	cfg := &config.Config{
		Name:        "test",
		Description: "test config",
		BaseDir:     "/test",
	}

	content := &config.ContentTree{
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

	cfg := &config.Config{
		MCPServers: map[string]*config.MCPServer{
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

func TestClaudePresetGenerator_buildAgentFrontmatter_Effort(t *testing.T) {
	g := &ClaudePresetGenerator{}

	tests := []struct {
		name          string
		agent         config.ContentFile
		cfg           *config.Config
		wantEffort    interface{}
		wantHasEffort bool
	}{
		{
			name: "agent effort wins over default",
			agent: config.ContentFile{
				Name:     "reviewer",
				Metadata: &config.Metadata{Effort: "high"},
			},
			cfg:           &config.Config{Defaults: &config.DefaultsConfig{Effort: "medium"}},
			wantEffort:    "high",
			wantHasEffort: true,
		},
		{
			name: "default fills in when agent has no effort",
			agent: config.ContentFile{
				Name:     "noop",
				Metadata: &config.Metadata{},
			},
			cfg:           &config.Config{Defaults: &config.DefaultsConfig{Effort: "medium"}},
			wantEffort:    "medium",
			wantHasEffort: true,
		},
		{
			name: "default applies when metadata is nil",
			agent: config.ContentFile{
				Name: "bare",
			},
			cfg:           &config.Config{Defaults: &config.DefaultsConfig{Effort: "low"}},
			wantEffort:    "low",
			wantHasEffort: true,
		},
		{
			name: "no effort emitted when neither set",
			agent: config.ContentFile{
				Name:     "plain",
				Metadata: &config.Metadata{},
			},
			cfg:           &config.Config{},
			wantHasEffort: false,
		},
		{
			name: "agent effort respected when no defaults block at all",
			agent: config.ContentFile{
				Name:     "solo",
				Metadata: &config.Metadata{Effort: "max"},
			},
			cfg:           &config.Config{},
			wantEffort:    "max",
			wantHasEffort: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm := g.buildAgentFrontmatter(tt.agent, tt.cfg)
			got, ok := fm["effort"]
			assert.Equal(t, tt.wantHasEffort, ok, "effort presence mismatch")
			if tt.wantHasEffort {
				assert.Equal(t, tt.wantEffort, got)
			}
		})
	}
}

func TestClaudePresetGenerator_renderPluginsJSON(t *testing.T) {
	g := &ClaudePresetGenerator{}

	enabled := true
	cfg := &config.Config{
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

// TestClaudePresetGenerator_PreservesSkillResourcesLayout verifies that a
// skill's references/, scripts/, and assets/ are emitted as separate files
// under the rendered skill directory rather than concatenated into SKILL.md.
// This is the core behavior change: progressive disclosure — SKILL.md links
// to references, the agent reads them on demand.
func TestClaudePresetGenerator_PreservesSkillResourcesLayout(t *testing.T) {
	t.Parallel()

	g := &ClaudePresetGenerator{}
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
			{
				Kind:    config.SkillKindScripts,
				RelPath: "scripts/run.sh",
				Content: []byte("#!/bin/sh\necho hi\n"),
			},
			{
				Kind:    config.SkillKindAssets,
				RelPath: "assets/blob.bin",
				Content: binaryAsset,
			},
		},
	}

	cfg := &config.Config{Name: "demo-cfg"}
	outputs, err := g.Generate(&config.ContentTree{Skills: []config.ContentFile{skill}}, "/test", cfg)
	require.NoError(t, err)

	skillDir := filepath.Join("/test", ".claude", "skills", "demo")
	skillMD := filepath.Join(skillDir, "SKILL.md")
	apiRef := filepath.Join(skillDir, "references", "api.md")
	runSh := filepath.Join(skillDir, "scripts", "run.sh")
	blob := filepath.Join(skillDir, "assets", "blob.bin")

	files := make(map[string]config.OutputFile)
	for _, o := range outputs {
		if !o.IsDir {
			files[o.Path] = o
		}
	}

	// SKILL.md exists and contains the resources index but NOT the inlined references body.
	skillFile, ok := files[skillMD]
	require.True(t, ok, "expected SKILL.md output at %s", skillMD)
	assert.Contains(t, skillFile.Content, "Body of skill.")
	assert.Contains(t, skillFile.Content, "## Resources")
	assert.Contains(t, skillFile.Content, "[`references/api.md`](references/api.md)")
	assert.Contains(t, skillFile.Content, "API reference")
	assert.NotContains(t, skillFile.Content, "API body.",
		"reference content must not be inlined into SKILL.md")

	// Reference is emitted as a separate file, with original frontmatter intact
	// (RawContent path skips ai-rulez header injection).
	apiFile, ok := files[apiRef]
	require.True(t, ok, "expected reference file at %s", apiRef)
	assert.Equal(t, []byte("---\ndescription: API reference\n---\n\nAPI body.\n"), apiFile.RawContent)
	assert.Empty(t, apiFile.Content, "reference must use RawContent, not Content")

	// Script preserved verbatim — no markdown header would have been valid here.
	scriptFile, ok := files[runSh]
	require.True(t, ok, "expected script file at %s", runSh)
	assert.Equal(t, []byte("#!/bin/sh\necho hi\n"), scriptFile.RawContent)

	// Binary asset round-trips byte-for-byte.
	blobFile, ok := files[blob]
	require.True(t, ok, "expected asset file at %s", blob)
	assert.Equal(t, binaryAsset, blobFile.RawContent)
}
