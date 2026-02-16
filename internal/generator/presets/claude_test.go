package presets

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
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

	// Check header comment (HTML comment for .md files)
	if !strings.Contains(result, "<!--") {
		t.Error("Expected HTML comment header for .md file")
	}

	// Check frontmatter
	if !strings.Contains(result, "---\n") {
		t.Error("Expected frontmatter in file")
	}

	// Check skill content
	if !strings.Contains(result, "# Test Skill") {
		t.Error("Expected skill content in output")
	}

	// Check rules section
	if !strings.Contains(result, "## Rules") {
		t.Error("Expected Rules section in output")
	}
	if !strings.Contains(result, "### coding-standards") {
		t.Error("Expected rule name in output")
	}

	// Check context section
	if !strings.Contains(result, "## Context") {
		t.Error("Expected Context section in output")
	}
	if !strings.Contains(result, "### project-info") {
		t.Error("Expected context name in output")
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
