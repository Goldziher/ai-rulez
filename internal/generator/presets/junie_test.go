package presets

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func TestJuniePresetGenerator_GetName(t *testing.T) {
	g := &JuniePresetGenerator{}
	if got := g.GetName(); got != "junie" {
		t.Errorf("GetName() = %q, want %q", got, "junie")
	}
}

func TestJuniePresetGenerator_Generate_WithSkillsAndAgents(t *testing.T) {
	g := &JuniePresetGenerator{}
	cfg := &config.ConfigV3{Name: "test"}

	content := &config.ContentTreeV3{
		Skills: []config.ContentFile{
			{
				Name:    "kotlin-review",
				Content: "Review Kotlin code",
				Path:    "/test/.ai-rulez/skills/kotlin-review/SKILL.md",
			},
		},
		Agents: []config.ContentFile{
			{
				Name:    "code-review-helper",
				Content: "You are a careful code reviewer.",
				Metadata: &config.MetadataV3{
					Extra: map[string]string{
						"description": "Reviews code changes",
						"model":       "sonnet",
						"tools":       "Read,Grep,Edit",
					},
				},
			},
		},
	}

	outputs, err := g.Generate(content, "/test", cfg)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	// Base: .junie, .junie/skills, .junie/agents, guidelines.md = 4
	// Skill: dir + SKILL.md = 2
	// Agent: .md = 1
	if len(outputs) != 7 {
		t.Errorf("Generate() got %d outputs, want 7", len(outputs))
	}

	// Verify skill
	var foundSkill bool
	for _, o := range outputs {
		if strings.Contains(filepath.ToSlash(o.Path), ".junie/skills/kotlin-review/SKILL.md") {
			foundSkill = true
			if !strings.Contains(o.Content, "name: kotlin-review") {
				t.Error("SKILL.md should contain skill name")
			}
		}
	}
	if !foundSkill {
		t.Error("Expected .junie/skills/kotlin-review/SKILL.md")
	}

	// Verify agent
	var foundAgent bool
	for _, o := range outputs {
		if strings.Contains(filepath.ToSlash(o.Path), ".junie/agents/code-review-helper.md") {
			foundAgent = true
			if !strings.Contains(o.Content, "name: code-review-helper") {
				t.Error("Agent file should contain name")
			}
			if !strings.Contains(o.Content, "description: Reviews code changes") {
				t.Error("Agent file should contain description")
			}
			if !strings.Contains(o.Content, "You are a careful code reviewer.") {
				t.Error("Agent file should contain content")
			}
		}
	}
	if !foundAgent {
		t.Error("Expected .junie/agents/code-review-helper.md")
	}
}

func TestJuniePresetGenerator_GetOutputPaths(t *testing.T) {
	g := &JuniePresetGenerator{}
	paths := g.GetOutputPaths("/base")

	wantPaths := []string{
		filepath.Join("/base", ".junie"),
		filepath.Join("/base", ".junie", "guidelines.md"),
		filepath.Join("/base", ".junie", "skills"),
		filepath.Join("/base", ".junie", "agents"),
	}

	if len(paths) != len(wantPaths) {
		t.Fatalf("GetOutputPaths() returned %d paths, want %d", len(paths), len(wantPaths))
	}

	for i, want := range wantPaths {
		if paths[i] != want {
			t.Errorf("GetOutputPaths()[%d] = %q, want %q", i, paths[i], want)
		}
	}
}
