package presets

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	cfg := &config.Config{Name: "test"}

	content := &config.ContentTree{
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
	// Agents dir = 1
	if len(outputs) != 6 {
		t.Errorf("Generate() got %d outputs, want 6", len(outputs))
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
		Metadata: &config.Metadata{
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

func TestAmpPresetGenerator_renderAmpAgentFile(t *testing.T) {
	g := &AmpPresetGenerator{}

	agent := config.ContentFile{
		Name:    "test-agent",
		Content: "You are a test agent.",
		Metadata: &config.Metadata{
			Extra: map[string]string{
				"description": "A test agent",
				"model":       "sonnet",
				"tools":       "Read,Grep",
			},
		},
	}

	content, err := g.renderAmpAgentFile(agent)
	require.NoError(t, err)
	assert.Contains(t, content, "---")
	assert.Contains(t, content, "name: test-agent")
	assert.Contains(t, content, "description: A test agent")
	assert.Contains(t, content, "model: sonnet")
	assert.Contains(t, content, "You are a test agent.")
}

func TestAmpPresetGenerator_buildAmpAgentFrontmatter(t *testing.T) {
	g := &AmpPresetGenerator{}

	t.Run("with metadata", func(t *testing.T) {
		agent := config.ContentFile{
			Name: "agent",
			Metadata: &config.Metadata{
				Extra: map[string]string{"description": "desc", "model": "opus"},
			},
		}
		fm := g.buildAmpAgentFrontmatter(agent)
		assert.Equal(t, "agent", fm["name"])
		assert.Equal(t, "desc", fm["description"])
		assert.Equal(t, "opus", fm["model"])
	})

	t.Run("without metadata", func(t *testing.T) {
		agent := config.ContentFile{Name: "agent"}
		fm := g.buildAmpAgentFrontmatter(agent)
		assert.Equal(t, "agent", fm["name"])
		assert.Len(t, fm, 1)
	})
}
