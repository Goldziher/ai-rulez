package presets

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	cfg := &config.Config{Name: "test"}

	content := &config.ContentTree{
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
	// Agents dir = 1
	if len(outputs) != 7 {
		t.Errorf("Generate() got %d outputs, want 7", len(outputs))
	}

	var foundSkill bool
	for _, o := range outputs {
		if strings.Contains(filepath.ToSlash(o.Path), ".cline/skills/deploy/SKILL.md") {
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

func TestClinePresetGenerator_renderClineAgentFile(t *testing.T) {
	g := &ClinePresetGenerator{}

	agent := config.ContentFile{
		Name:    "test-agent",
		Content: "You are a test agent.",
		Metadata: &config.Metadata{
			Extra: map[string]string{
				"description": "A test agent",
				"model":       "sonnet",
			},
		},
	}

	content, err := g.renderClineAgentFile(agent)
	require.NoError(t, err)
	assert.Contains(t, content, "---")
	assert.Contains(t, content, "name: test-agent")
	assert.Contains(t, content, "description: A test agent")
	assert.Contains(t, content, "model: sonnet")
	assert.Contains(t, content, "You are a test agent.")
}

func TestClinePresetGenerator_Generate_WithContext(t *testing.T) {
	g := &ClinePresetGenerator{}

	content := &config.ContentTree{
		Rules: []config.ContentFile{
			{Name: "rule1", Content: "Rule content"},
		},
		Context: []config.ContentFile{
			{Name: "project-arch", Content: "Uses hexagonal architecture"},
		},
		Skills:   []config.ContentFile{},
		Agents:   []config.ContentFile{},
		Commands: []config.ContentFile{},
		Domains:  map[string]*config.Domain{},
	}
	cfg := &config.Config{Name: "test"}

	outputs, err := g.Generate(content, "/tmp/test", cfg)
	require.NoError(t, err)

	// Check that context is rendered as a file in .clinerules
	found := false
	for _, o := range outputs {
		if strings.Contains(o.Content, "hexagonal architecture") {
			found = true
			break
		}
	}
	assert.True(t, found, "Context should be rendered in output")
}
