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

func TestCopilotPresetGenerator_GetName(t *testing.T) {
	g := &CopilotPresetGenerator{}
	if got := g.GetName(); got != "copilot" {
		t.Errorf("GetName() = %q, want %q", got, "copilot")
	}
}

func TestCopilotPresetGenerator_Generate_WithSkillsAndAgents(t *testing.T) {
	g := &CopilotPresetGenerator{}
	cfg := &config.ConfigV3{Name: "test"}

	content := &config.ContentTreeV3{
		Skills: []config.ContentFile{
			{
				Name:    "deploy",
				Content: "Deploy instructions",
				Path:    "/test/.ai-rulez/skills/deploy/SKILL.md",
			},
		},
		Agents: []config.ContentFile{
			{
				Name:    "reviewer",
				Content: "You review code.",
				Metadata: &config.MetadataV3{
					Extra: map[string]string{
						"description": "Reviews code changes",
						"model":       "gpt-5",
					},
				},
			},
		},
	}

	outputs, err := g.Generate(content, "/test", cfg)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	// Base: .github, .github/skills, .github/agents, copilot-instructions.md = 4
	// Skill: dir + SKILL.md = 2
	// Agent: .agent.md = 1
	// Commands dir = 1
	if len(outputs) != 8 {
		t.Errorf("Generate() got %d outputs, want 8", len(outputs))
	}

	// Verify agent uses .agent.md extension
	var foundAgent bool
	for _, o := range outputs {
		if strings.HasSuffix(o.Path, ".agent.md") {
			foundAgent = true
			if !strings.Contains(o.Content, "name: reviewer") {
				t.Error("Agent file should contain name")
			}
			if !strings.Contains(o.Content, "description: Reviews code changes") {
				t.Error("Agent file should contain description")
			}
			if !strings.Contains(o.Content, "model: gpt-5") {
				t.Error("Agent file should contain model")
			}
		}
	}
	if !foundAgent {
		t.Error("Expected .agent.md file in outputs")
	}

	// Verify skill in .github/skills/
	var foundSkill bool
	for _, o := range outputs {
		if strings.Contains(filepath.ToSlash(o.Path), ".github/skills/deploy/SKILL.md") {
			foundSkill = true
		}
	}
	if !foundSkill {
		t.Error("Expected .github/skills/deploy/SKILL.md in outputs")
	}
}

func TestCopilotPresetGenerator_GetOutputPaths(t *testing.T) {
	g := &CopilotPresetGenerator{}
	paths := g.GetOutputPaths("/base")

	wantPaths := []string{
		filepath.Join("/base", ".github"),
		filepath.Join("/base", ".github", "copilot-instructions.md"),
		filepath.Join("/base", ".github", "skills"),
		filepath.Join("/base", ".github", "agents"),
		filepath.Join("/base", ".github", "commands"),
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

func TestCopilotPresetGenerator_shouldIncludeCommand(t *testing.T) {
	g := &CopilotPresetGenerator{}

	tests := []struct {
		name     string
		command  config.ContentFile
		expected bool
	}{
		{
			name:     "no metadata includes",
			command:  config.ContentFile{Name: "cmd"},
			expected: true,
		},
		{
			name: "no targets includes",
			command: config.ContentFile{
				Name:     "cmd",
				Metadata: &config.MetadataV3{},
			},
			expected: true,
		},
		{
			name: "matching target includes",
			command: config.ContentFile{
				Name:     "cmd",
				Metadata: &config.MetadataV3{Targets: []string{"copilot"}},
			},
			expected: true,
		},
		{
			name: "non-matching target excludes",
			command: config.ContentFile{
				Name:     "cmd",
				Metadata: &config.MetadataV3{Targets: []string{"claude", "cursor"}},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, g.shouldIncludeCommand(tt.command))
		})
	}
}

func TestCopilotPresetGenerator_renderMCPJSON(t *testing.T) {
	g := &CopilotPresetGenerator{}

	cfg := &config.ConfigV3{
		MCPServers: map[string]*config.MCPServerV3{
			"test-server": {
				Command: "npx",
				Args:    []string{"-y", "test-mcp"},
				Env:     map[string]string{"API_KEY": "key"},
			},
		},
	}

	content, err := g.renderMCPJSON(cfg)
	require.NoError(t, err)
	assert.Contains(t, content, "test-server")
	assert.Contains(t, content, "npx")
	assert.Contains(t, content, "mcpServers")

	// Verify valid JSON
	var parsed map[string]interface{}
	err = json.Unmarshal([]byte(content), &parsed)
	require.NoError(t, err)
	servers := parsed["mcpServers"].(map[string]interface{})
	assert.Len(t, servers, 1)
}
