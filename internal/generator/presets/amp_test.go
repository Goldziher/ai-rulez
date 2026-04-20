package presets

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func TestAmpPresetGenerator_GetName(t *testing.T) {
	g := &AmpPresetGenerator{}
	if got := g.GetName(); got != "amp" {
		t.Errorf("GetName() = %q, want %q", got, "amp")
	}
}

func TestAmpPresetGenerator_Generate_WithSkills(t *testing.T) {
	g := &AmpPresetGenerator{}
	cfg := &config.ConfigV3{Name: "test"}

	content := &config.ContentTreeV3{
		Skills: []config.ContentFile{
			{
				Name:    "deploy",
				Content: "Deploy instructions",
				Path:    "/test/.ai-rulez/skills/deploy/SKILL.md",
			},
		},
	}

	outputs, err := g.Generate(content, "/test", cfg)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	// Base: .agents, .agents/skills, AGENTS.md = 3
	// Skill: dir + SKILL.md = 2
	if len(outputs) != 5 {
		t.Errorf("Generate() got %d outputs, want 5", len(outputs))
	}

	var foundSkill bool
	for _, o := range outputs {
		if strings.HasSuffix(filepath.ToSlash(o.Path), "deploy/SKILL.md") {
			foundSkill = true
			if !strings.Contains(o.Content, "name: deploy") {
				t.Error("SKILL.md should contain skill name")
			}
		}
	}
	if !foundSkill {
		t.Error("Expected deploy/SKILL.md in outputs")
	}
}

func TestAmpPresetGenerator_renderSkillFile(t *testing.T) {
	g := &AmpPresetGenerator{}

	skill := config.ContentFile{
		Name:    "test-skill",
		Content: "Skill content here",
		Metadata: &config.MetadataV3{
			Extra: map[string]string{"description": "A test skill"},
		},
	}

	result := g.renderSkillFile(skill)

	if !strings.HasPrefix(result, "---\n") {
		t.Error("Expected YAML frontmatter")
	}
	if !strings.Contains(result, "name: test-skill") {
		t.Error("Expected skill name")
	}
	if !strings.Contains(result, "Skill content here") {
		t.Error("Expected skill content")
	}
}
