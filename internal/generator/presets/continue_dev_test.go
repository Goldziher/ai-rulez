package presets

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContinueDevPresetGenerator_GetName(t *testing.T) {
	g := &ContinueDevPresetGenerator{}
	assert.Equal(t, "continue-dev", g.GetName())
}

func TestContinueDevPresetGenerator_GetOutputPaths(t *testing.T) {
	g := &ContinueDevPresetGenerator{}
	paths := g.GetOutputPaths("/test")

	assert.Equal(t, 4, len(paths))
	assert.Contains(t, paths, filepath.Join("/test", ".continue"))
	assert.Contains(t, paths, filepath.Join("/test", ".continue", "rules"))
	assert.Contains(t, paths, filepath.Join("/test", ".continue", "prompts"))
	assert.Contains(t, paths, filepath.Join("/test", ".continue", "agents"))
}

func TestContinueDevPresetGenerator_Generate_WithSkillsAndAgents(t *testing.T) {
	g := &ContinueDevPresetGenerator{}
	content := &config.ContentTree{
		Rules: []config.ContentFile{
			{Name: "test-rule", Content: "Rule content"},
		},
		Context: []config.ContentFile{
			{Name: "test-context", Content: "Context content"},
		},
		Skills: []config.ContentFile{
			{Name: "test-skill", Path: "skills/test-skill/SKILL.md", Content: "Skill content"},
		},
		Agents: []config.ContentFile{
			{
				Name:    "test-agent",
				Content: "Agent content",
				Metadata: &config.Metadata{
					Extra: map[string]string{"description": "Test agent description", "model": "sonnet"},
				},
			},
		},
		Commands: []config.ContentFile{
			{
				Name:    "test-cmd",
				Content: "Command content",
				Metadata: &config.Metadata{
					Extra: map[string]string{"description": "Test command"},
				},
			},
		},
		Domains: map[string]*config.Domain{},
	}
	cfg := &config.Config{Name: "test-project"}

	outputs, err := g.Generate(content, "/tmp/test", cfg)
	require.NoError(t, err)

	// Should have: 3 dirs (.continue, rules, prompts, agents) + 1 rule + 1 prompts.yaml + 1 agent + 1 command prompt
	var files, dirs int
	for _, o := range outputs {
		if o.IsDir {
			dirs++
		} else {
			files++
		}
	}
	assert.GreaterOrEqual(t, dirs, 4, "Should have at least 4 directories")
	assert.GreaterOrEqual(t, files, 2, "Should have at least rule + prompts files")

	// Verify agent file exists with frontmatter
	var agentFile *config.OutputFile
	for i := range outputs {
		if strings.HasSuffix(outputs[i].Path, "test-agent.md") && !outputs[i].IsDir {
			agentFile = &outputs[i]
			break
		}
	}
	require.NotNil(t, agentFile, "Should generate agent file")
	assert.Contains(t, agentFile.Content, "name: test-agent")
	assert.Contains(t, agentFile.Content, "description: Test agent description")
	assert.Contains(t, agentFile.Content, "model: sonnet")
	assert.Contains(t, agentFile.Content, "Agent content")
}

func TestContinueDevPresetGenerator_Generate_WithDomains(t *testing.T) {
	g := &ContinueDevPresetGenerator{}
	cfg := &config.Config{Name: "test"}

	content := &config.ContentTree{
		Rules: []config.ContentFile{
			{Name: "root-rule", Content: "Root rule"},
		},
		Domains: map[string]*config.Domain{
			"backend": {
				Name: "backend",
				Rules: []config.ContentFile{
					{Name: "backend-rule", Content: "Backend rule"},
				},
			},
		},
	}

	outputs, err := g.Generate(content, "/test", cfg)
	require.NoError(t, err)

	// Both root and domain rules should be included
	var ruleCount int
	for _, o := range outputs {
		if strings.HasSuffix(o.Path, ".md") && strings.Contains(o.Path, "rules") {
			ruleCount++
		}
	}
	assert.GreaterOrEqual(t, ruleCount, 2, "Should have both root and domain rules")
}

func TestContinueDevPresetGenerator_shouldIncludeCommand(t *testing.T) {
	g := &ContinueDevPresetGenerator{}

	tests := []struct {
		name     string
		command  config.ContentFile
		expected bool
	}{
		{
			name:     "no metadata includes by default",
			command:  config.ContentFile{Name: "cmd"},
			expected: true,
		},
		{
			name: "no targets includes by default",
			command: config.ContentFile{
				Name:     "cmd",
				Metadata: &config.Metadata{},
			},
			expected: true,
		},
		{
			name: "matching target includes",
			command: config.ContentFile{
				Name:     "cmd",
				Metadata: &config.Metadata{Targets: []string{"continue-dev"}},
			},
			expected: true,
		},
		{
			name: "non-matching target excludes",
			command: config.ContentFile{
				Name:     "cmd",
				Metadata: &config.Metadata{Targets: []string{"claude", "cursor"}},
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

func TestContinueDevPresetGenerator_renderContinueDevAgentFile(t *testing.T) {
	g := &ContinueDevPresetGenerator{}

	agent := config.ContentFile{
		Name:    "my-agent",
		Content: "You are a helpful agent.",
		Metadata: &config.Metadata{
			Extra: map[string]string{
				"description": "Helpful agent",
				"model":       "opus",
			},
		},
	}

	content, err := g.renderContinueDevAgentFile(agent, nil)
	require.NoError(t, err)
	assert.Contains(t, content, "---")
	assert.Contains(t, content, "name: my-agent")
	assert.Contains(t, content, "description: Helpful agent")
	assert.Contains(t, content, "model: opus")
	assert.Contains(t, content, "You are a helpful agent.")
}

func TestContinueDevPresetGenerator_renderContinueDevAgentFile_WithoutMetadata(t *testing.T) {
	g := &ContinueDevPresetGenerator{}

	agent := config.ContentFile{
		Name:    "simple-agent",
		Content: "Simple agent instructions.",
	}

	content, err := g.renderContinueDevAgentFile(agent, nil)
	require.NoError(t, err)
	assert.Contains(t, content, "---")
	assert.Contains(t, content, "name: simple-agent")
	assert.Contains(t, content, "Simple agent instructions.")
}

func TestContinueDevPresetGenerator_renderPromptsYAML_IncludesCommands(t *testing.T) {
	g := &ContinueDevPresetGenerator{}

	content := &config.ContentTree{
		Context: []config.ContentFile{
			{Name: "ctx", Content: "Context here"},
		},
		Skills: []config.ContentFile{
			{Name: "skill1", Content: "Skill content"},
		},
		Commands: []config.ContentFile{
			{
				Name:    "run-tests",
				Content: "Run tests",
				Metadata: &config.Metadata{
					Extra: map[string]string{"description": "Run all tests"},
				},
			},
		},
		Domains: map[string]*config.Domain{},
	}
	cfg := &config.Config{Name: "test"}

	result, err := g.renderPromptsYAML(content, cfg)
	require.NoError(t, err)
	assert.Contains(t, result, "run-tests")
	assert.Contains(t, result, "Run all tests")
	assert.Contains(t, result, "ctx")
	assert.Contains(t, result, "skill1")
}

func TestContinueDevPresetGenerator_renderPromptsYAML_ExcludesNonTargetedCommands(t *testing.T) {
	g := &ContinueDevPresetGenerator{}

	content := &config.ContentTree{
		Commands: []config.ContentFile{
			{
				Name:    "continue-cmd",
				Content: "Continue command",
				Metadata: &config.Metadata{
					Targets: []string{"continue-dev"},
					Extra: map[string]string{
						"description": "Continue only",
					},
				},
			},
			{
				Name:    "claude-cmd",
				Content: "Claude command",
				Metadata: &config.Metadata{
					Targets: []string{"claude"},
					Extra: map[string]string{
						"description": "Claude only",
					},
				},
			},
		},
		Domains: map[string]*config.Domain{},
	}
	cfg := &config.Config{Name: "test"}

	result, err := g.renderPromptsYAML(content, cfg)
	require.NoError(t, err)
	assert.Contains(t, result, "continue-cmd")
	assert.NotContains(t, result, "claude-cmd")
}

func TestContinueDevPresetGenerator_renderRuleFile(t *testing.T) {
	g := &ContinueDevPresetGenerator{}

	rule := config.ContentFile{
		Name:    "test rule",
		Content: "Test content",
		Metadata: &config.Metadata{
			Priority: "high",
		},
	}

	result := g.renderRuleFile(rule, &config.Config{Name: "test"}, ".continue/rules/test-rule.md", 1)

	assert.Contains(t, result, "# test rule")
	assert.Contains(t, result, "**Priority:** high")
	assert.Contains(t, result, "Test content")
}

func TestContinueDevPresetGenerator_buildContinueDevAgentFrontmatter(t *testing.T) {
	g := &ContinueDevPresetGenerator{}

	tests := []struct {
		name     string
		agent    config.ContentFile
		hasDesc  bool
		hasModel bool
	}{
		{
			name: "with description and model",
			agent: config.ContentFile{
				Name: "agent1",
				Metadata: &config.Metadata{
					Extra: map[string]string{
						"description": "Test desc",
						"model":       "sonnet",
					},
				},
			},
			hasDesc:  true,
			hasModel: true,
		},
		{
			name:     "without metadata",
			agent:    config.ContentFile{Name: "agent2"},
			hasDesc:  false,
			hasModel: false,
		},
		{
			name: "with only description",
			agent: config.ContentFile{
				Name: "agent3",
				Metadata: &config.Metadata{
					Extra: map[string]string{"description": "Only desc"},
				},
			},
			hasDesc:  true,
			hasModel: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm := g.buildContinueDevAgentFrontmatter(tt.agent, nil)
			assert.Equal(t, tt.agent.Name, fm["name"])

			if tt.hasDesc {
				assert.NotNil(t, fm["description"])
			} else {
				assert.Nil(t, fm["description"])
			}

			if tt.hasModel {
				assert.NotNil(t, fm["model"])
			} else {
				assert.Nil(t, fm["model"])
			}
		})
	}
}

func TestContinueDevPresetGenerator_AgentFrontmatter_DoesNotEmitEffort(t *testing.T) {
	// Continue.dev reads reasoning settings from .continue/config.yaml model
	// blocks (reasoning, reasoningBudgetTokens), not from agent frontmatter.
	// We deliberately don't emit them: the file is user-managed and partial
	// generation would clobber unrelated settings.
	g := &ContinueDevPresetGenerator{}
	fm := g.buildContinueDevAgentFrontmatter(config.ContentFile{
		Name:     "reviewer",
		Metadata: &config.Metadata{Effort: "high"},
	}, nil)
	if _, ok := fm["effort"]; ok {
		t.Errorf("effort should not appear in continue.dev agent frontmatter")
	}
	if _, ok := fm["reasoning"]; ok {
		t.Errorf("reasoning should not appear in continue.dev agent frontmatter")
	}
	if _, ok := fm["reasoning_effort"]; ok {
		t.Errorf("reasoning_effort should not appear in continue.dev agent frontmatter")
	}
}
