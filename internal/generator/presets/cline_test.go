package presets

import (
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func TestClinePresetGenerator_GetName(t *testing.T) {
	g := &ClinePresetGenerator{}
	if got := g.GetName(); got != "cline" {
		t.Errorf("GetName() = %q, want %q", got, "cline")
	}
}

func TestClinePresetGenerator_Generate_WithSkills(t *testing.T) {
	g := &ClinePresetGenerator{}
	cfg := &config.ConfigV3{Name: "test"}

	content := &config.ContentTreeV3{
		Rules: []config.ContentFile{
			{Name: "rule1", Content: "Rule content"},
		},
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

	// Base: .clinerules, .cline, .cline/skills = 3 dirs
	// Rule: 1 file
	// Skill: dir + SKILL.md = 2
	if len(outputs) != 6 {
		t.Errorf("Generate() got %d outputs, want 6", len(outputs))
	}

	var foundSkill bool
	for _, o := range outputs {
		if strings.Contains(o.Path, ".cline/skills/deploy/SKILL.md") {
			foundSkill = true
			if !strings.Contains(o.Content, "name: deploy") {
				t.Error("SKILL.md should contain skill name")
			}
		}
	}
	if !foundSkill {
		t.Error("Expected .cline/skills/deploy/SKILL.md in outputs")
	}
}
