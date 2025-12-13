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
			wantOutputs: 6, // .claude dir, skills dir, agents dir, skill-id dir, skill/SKILL.md, agents/test-skill.md
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
			wantOutputs: 3, // .claude dir, skills dir, agents dir
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
				}
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

	result, err := g.renderSkillFile(skill, content)
	if err != nil {
		t.Fatalf("renderSkillFile() error = %v", err)
	}

	// Check frontmatter
	if !strings.HasPrefix(result, "---\n") {
		t.Error("Expected frontmatter at start of file")
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
