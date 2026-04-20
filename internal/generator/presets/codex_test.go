package presets

import (
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func TestCodexPresetGenerator_renderSkillFile_AlwaysIncludesDescription(t *testing.T) {
	g := &CodexPresetGenerator{}

	tests := []struct {
		name            string
		skill           config.ContentFile
		wantDescription string
	}{
		{
			name: "uses explicit description",
			skill: config.ContentFile{
				Name:    "core-workflows",
				Content: "# Core Workflows",
				Metadata: &config.MetadataV3{
					Extra: map[string]string{
						"description": "Handles core generation and validation workflows.",
					},
				},
			},
			wantDescription: "Handles core generation and validation workflows.",
		},
		{
			name: "falls back to short-description",
			skill: config.ContentFile{
				Name:    "release-and-distribution",
				Content: "# Release and Distribution",
				Metadata: &config.MetadataV3{
					Extra: map[string]string{
						"short-description": "Keeps release channels and versions aligned.",
					},
				},
			},
			wantDescription: "Keeps release channels and versions aligned.",
		},
		{
			name: "falls back to generated description",
			skill: config.ContentFile{
				Name:    "docs-and-site",
				Content: "# Docs and Site",
			},
			wantDescription: "docs-and-site",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := g.renderSkillFile(tt.skill)
			expectedLine := `description: "` + tt.wantDescription + `"`
			if !strings.Contains(result, expectedLine) {
				t.Fatalf("expected %q in output, got:\n%s", expectedLine, result)
			}
		})
	}
}

func TestCodexPresetGenerator_Generate_WithAgents(t *testing.T) {
	g := &CodexPresetGenerator{}
	cfg := &config.ConfigV3{Name: "test"}

	content := &config.ContentTreeV3{
		Agents: []config.ContentFile{
			{
				Name:    "explorer",
				Content: "You explore codebases.",
				Metadata: &config.MetadataV3{
					Extra: map[string]string{
						"description": "Read-only codebase explorer",
					},
				},
			},
		},
	}

	outputs, err := g.Generate(content, "/test", cfg)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	// Find the TOML agent file
	var agentContent string
	for _, o := range outputs {
		if strings.HasSuffix(o.Path, "explorer.toml") {
			agentContent = o.Content
		}
	}

	if agentContent == "" {
		t.Fatal("Expected explorer.toml agent file in outputs")
	}

	if !strings.Contains(agentContent, `name = "explorer"`) {
		t.Error("Expected name field in TOML")
	}
	if !strings.Contains(agentContent, `description = "Read-only codebase explorer"`) {
		t.Error("Expected description field in TOML")
	}
	if !strings.Contains(agentContent, `developer_instructions = """`) {
		t.Error("Expected multi-line developer_instructions in TOML")
	}
	if !strings.Contains(agentContent, "You explore codebases.") {
		t.Error("Expected agent content in developer_instructions")
	}
}

func TestCodexPresetGenerator_renderAgentTOML(t *testing.T) {
	g := &CodexPresetGenerator{}

	agent := config.ContentFile{
		Name:    "pr-reviewer",
		Content: "Review pull requests.\nCheck for bugs.",
		Metadata: &config.MetadataV3{
			Extra: map[string]string{
				"description": "Reviews PRs for issues",
			},
		},
	}

	result := g.renderAgentTOML(agent)

	if !strings.Contains(result, `name = "pr-reviewer"`) {
		t.Error("Expected name field")
	}
	if !strings.Contains(result, `description = "Reviews PRs for issues"`) {
		t.Error("Expected description field")
	}
	if !strings.Contains(result, `developer_instructions = """`) {
		t.Error("Expected multi-line string delimiter")
	}
	if !strings.Contains(result, "Review pull requests.\nCheck for bugs.") {
		t.Error("Expected multi-line content preserved")
	}
}

func TestCodexPresetGenerator_renderSkillFile_PreservesShortDescriptionMetadata(t *testing.T) {
	g := &CodexPresetGenerator{}

	skill := config.ContentFile{
		Name:    "config-schema-maintainer",
		Content: "# Config Schema Maintainer",
		Metadata: &config.MetadataV3{
			Extra: map[string]string{
				"short-description": "Maintains config schema contracts.",
			},
		},
	}

	result := g.renderSkillFile(skill)
	if !strings.Contains(result, `metadata:`) {
		t.Fatalf("expected metadata block in output, got:\n%s", result)
	}
	if !strings.Contains(result, `short-description: "Maintains config schema contracts."`) {
		t.Fatalf("expected quoted short-description in output, got:\n%s", result)
	}
}
