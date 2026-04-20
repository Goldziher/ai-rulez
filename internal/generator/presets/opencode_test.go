package presets

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func TestOpencodePresetGenerator_GetName(t *testing.T) {
	g := &OpencodePresetGenerator{}
	if got := g.GetName(); got != "opencode" {
		t.Errorf("GetName() = %q, want %q", got, "opencode")
	}
}

func TestOpencodePresetGenerator_Generate_WithSkillsAndAgents(t *testing.T) {
	g := &OpencodePresetGenerator{}
	cfg := &config.ConfigV3{Name: "test"}

	content := &config.ContentTreeV3{
		Skills: []config.ContentFile{
			{
				Name:    "deploy",
				Content: "Deploy skill",
				Path:    "/test/.ai-rulez/skills/deploy/SKILL.md",
			},
		},
		Agents: []config.ContentFile{
			{
				Name:    "reviewer",
				Content: "Review code changes.",
				Metadata: &config.MetadataV3{
					Extra: map[string]string{
						"description": "Reviews code",
						"mode":        "subagent",
						"model":       "claude-sonnet-4",
					},
				},
			},
		},
	}

	outputs, err := g.Generate(content, "/test", cfg)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	// Base: .opencode, .opencode/skills, .opencode/agents, AGENTS.md = 4
	// Skill: dir + SKILL.md = 2
	// Agent: .md = 1
	if len(outputs) != 7 {
		t.Errorf("Generate() got %d outputs, want 7", len(outputs))
	}

	// Verify skill
	var foundSkill bool
	for _, o := range outputs {
		if strings.Contains(filepath.ToSlash(o.Path), ".opencode/skills/deploy/SKILL.md") {
			foundSkill = true
		}
	}
	if !foundSkill {
		t.Error("Expected .opencode/skills/deploy/SKILL.md")
	}

	// Verify agent
	var foundAgent bool
	for _, o := range outputs {
		if strings.Contains(filepath.ToSlash(o.Path), ".opencode/agents/reviewer.md") {
			foundAgent = true
			if !strings.Contains(o.Content, "name: reviewer") {
				t.Error("Agent file should contain name")
			}
			if !strings.Contains(o.Content, "mode: subagent") {
				t.Error("Agent file should contain mode")
			}
			if !strings.Contains(o.Content, "model: claude-sonnet-4") {
				t.Error("Agent file should contain model")
			}
		}
	}
	if !foundAgent {
		t.Error("Expected .opencode/agents/reviewer.md")
	}
}

func TestOpencodePresetGenerator_GetOutputPaths(t *testing.T) {
	g := &OpencodePresetGenerator{}
	paths := g.GetOutputPaths("/base")

	wantPaths := []string{
		filepath.Join("/base", "AGENTS.md"),
		filepath.Join("/base", ".opencode"),
		filepath.Join("/base", ".opencode", "skills"),
		filepath.Join("/base", ".opencode", "agents"),
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
