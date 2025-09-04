package crud_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func createTestConfig(t *testing.T) (string, *config.Config) {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "ai_rulez.yaml")

	cfg := &config.Config{
		Metadata: config.Metadata{
			Name: "Test Project",
		},
		Outputs: []config.Output{
			{Path: "test.md"},
		},
	}

	err := config.SaveConfig(cfg, configFile)
	require.NoError(t, err)
	return configFile, cfg
}

func TestRuleCRUD(t *testing.T) {
	t.Run("add rule", func(t *testing.T) {
		configFile, _ := createTestConfig(t)

		cfg, err := config.LoadConfig(configFile)
		require.NoError(t, err)

		newRule := config.Rule{
			Name:     "test-rule",
			Content:  "Test rule content",
			Priority: config.PriorityMedium,
		}

		cfg.Rules = append(cfg.Rules, newRule)
		require.NoError(t, config.SaveConfig(cfg, configFile))

		reloaded, err := config.LoadConfig(configFile)
		require.NoError(t, err)
		assert.Len(t, reloaded.Rules, 1)
		assert.Equal(t, "test-rule", reloaded.Rules[0].Name)
	})

	t.Run("update rule", func(t *testing.T) {
		configFile, _ := createTestConfig(t)

		cfg, _ := config.LoadConfig(configFile)
		cfg.Rules = []config.Rule{{Name: "rule1", Content: "Original", Priority: config.PriorityLow}}
		config.SaveConfig(cfg, configFile)

		cfg, _ = config.LoadConfig(configFile)
		cfg.Rules[0].Content = "Updated"
		cfg.Rules[0].Priority = config.PriorityCritical
		require.NoError(t, config.SaveConfig(cfg, configFile))

		reloaded, _ := config.LoadConfig(configFile)
		assert.Equal(t, "Updated", reloaded.Rules[0].Content)
		assert.Equal(t, config.PriorityCritical, reloaded.Rules[0].Priority)
	})

	t.Run("delete rule", func(t *testing.T) {
		configFile, _ := createTestConfig(t)

		cfg, _ := config.LoadConfig(configFile)
		cfg.Rules = []config.Rule{
			{Name: "rule1", Content: "Content1"},
			{Name: "rule2", Content: "Content2"},
			{Name: "rule3", Content: "Content3"},
		}
		config.SaveConfig(cfg, configFile)

		cfg, _ = config.LoadConfig(configFile)
		var newRules []config.Rule
		for _, r := range cfg.Rules {
			if r.Name != "rule2" {
				newRules = append(newRules, r)
			}
		}
		cfg.Rules = newRules
		config.SaveConfig(cfg, configFile)

		reloaded, _ := config.LoadConfig(configFile)
		assert.Len(t, reloaded.Rules, 2)
		for _, r := range reloaded.Rules {
			assert.NotEqual(t, "rule2", r.Name)
		}
	})
}

func TestSectionCRUD(t *testing.T) {
	t.Run("add section", func(t *testing.T) {
		configFile, _ := createTestConfig(t)

		cfg, err := config.LoadConfig(configFile)
		require.NoError(t, err)

		newSection := config.Section{
			Name:     "Test Section",
			Content:  "Section content",
			Priority: config.PriorityMedium,
		}

		cfg.Sections = append(cfg.Sections, newSection)
		require.NoError(t, config.SaveConfig(cfg, configFile))

		reloaded, err := config.LoadConfig(configFile)
		require.NoError(t, err)
		assert.Len(t, reloaded.Sections, 1)
		assert.Equal(t, "Test Section", reloaded.Sections[0].Name)
	})

	t.Run("update section", func(t *testing.T) {
		configFile, _ := createTestConfig(t)

		cfg, _ := config.LoadConfig(configFile)
		cfg.Sections = []config.Section{{Name: "Section1", Content: "Original", Priority: config.PriorityLow}}
		config.SaveConfig(cfg, configFile)

		cfg, _ = config.LoadConfig(configFile)
		cfg.Sections[0].Content = "Updated"
		cfg.Sections[0].Priority = config.PriorityCritical
		require.NoError(t, config.SaveConfig(cfg, configFile))

		reloaded, _ := config.LoadConfig(configFile)
		assert.Equal(t, "Updated", reloaded.Sections[0].Content)
		assert.Equal(t, config.PriorityCritical, reloaded.Sections[0].Priority)
	})

	t.Run("delete section", func(t *testing.T) {
		configFile, _ := createTestConfig(t)

		cfg, _ := config.LoadConfig(configFile)
		cfg.Sections = []config.Section{
			{Name: "Section1", Content: "Content1"},
			{Name: "Section2", Content: "Content2"},
		}
		config.SaveConfig(cfg, configFile)

		cfg, _ = config.LoadConfig(configFile)
		cfg.Sections = cfg.Sections[1:]
		config.SaveConfig(cfg, configFile)

		reloaded, _ := config.LoadConfig(configFile)
		assert.Len(t, reloaded.Sections, 1)
		assert.Equal(t, "Section2", reloaded.Sections[0].Name)
	})
}

func TestAgentCRUD(t *testing.T) {
	t.Run("add agent", func(t *testing.T) {
		configFile, _ := createTestConfig(t)

		cfg, err := config.LoadConfig(configFile)
		require.NoError(t, err)

		newAgent := config.Agent{
			Name:         "test-agent",
			Description:  "Test agent",
			Priority:     config.PriorityMedium,
			Tools:        []string{"read", "write"},
			SystemPrompt: "You are a test agent",
		}

		cfg.Agents = append(cfg.Agents, newAgent)
		require.NoError(t, config.SaveConfig(cfg, configFile))

		reloaded, err := config.LoadConfig(configFile)
		require.NoError(t, err)
		assert.Len(t, reloaded.Agents, 1)
		assert.Equal(t, "test-agent", reloaded.Agents[0].Name)
		assert.ElementsMatch(t, []string{"read", "write"}, reloaded.Agents[0].Tools)
	})

	t.Run("update agent", func(t *testing.T) {
		configFile, _ := createTestConfig(t)

		cfg, _ := config.LoadConfig(configFile)
		cfg.Agents = []config.Agent{
			{Name: "agent1", Description: "Original", Priority: config.PriorityLow},
		}
		config.SaveConfig(cfg, configFile)

		cfg, _ = config.LoadConfig(configFile)
		cfg.Agents[0].Description = "Updated"
		cfg.Agents[0].Priority = config.PriorityCritical
		cfg.Agents[0].Tools = []string{"execute"}
		require.NoError(t, config.SaveConfig(cfg, configFile))

		reloaded, _ := config.LoadConfig(configFile)
		assert.Equal(t, "Updated", reloaded.Agents[0].Description)
		assert.Equal(t, config.PriorityCritical, reloaded.Agents[0].Priority)
		assert.Equal(t, []string{"execute"}, reloaded.Agents[0].Tools)
	})

	t.Run("delete agent", func(t *testing.T) {
		configFile, _ := createTestConfig(t)

		cfg, _ := config.LoadConfig(configFile)
		cfg.Agents = []config.Agent{
			{Name: "agent1", Description: "Agent 1"},
			{Name: "agent2", Description: "Agent 2"},
		}
		config.SaveConfig(cfg, configFile)

		cfg, _ = config.LoadConfig(configFile)
		cfg.Agents = cfg.Agents[1:]
		config.SaveConfig(cfg, configFile)

		reloaded, _ := config.LoadConfig(configFile)
		assert.Len(t, reloaded.Agents, 1)
		assert.Equal(t, "agent2", reloaded.Agents[0].Name)
	})
}

func TestOutputCRUD(t *testing.T) {
	t.Run("add output", func(t *testing.T) {
		configFile, _ := createTestConfig(t)

		cfg, err := config.LoadConfig(configFile)
		require.NoError(t, err)

		newOutput := config.Output{
			Path: "new-output.md",
			Template: map[string]interface{}{
				"type":  "builtin",
				"value": "minimal",
			},
		}

		cfg.Outputs = append(cfg.Outputs, newOutput)
		require.NoError(t, config.SaveConfig(cfg, configFile))

		reloaded, err := config.LoadConfig(configFile)
		require.NoError(t, err)
		assert.Len(t, reloaded.Outputs, 2)
		assert.Equal(t, "new-output.md", reloaded.Outputs[1].Path)
	})

	t.Run("update output", func(t *testing.T) {
		configFile, _ := createTestConfig(t)

		cfg, _ := config.LoadConfig(configFile)
		cfg.Outputs[0].Template = map[string]interface{}{
			"type":  "builtin",
			"value": "documentation",
		}
		require.NoError(t, config.SaveConfig(cfg, configFile))

		reloaded, _ := config.LoadConfig(configFile)
		template, err := reloaded.Outputs[0].GetTemplate()
		require.NoError(t, err)
		assert.Equal(t, "documentation", template.Value)
	})

	t.Run("delete output", func(t *testing.T) {
		configFile, _ := createTestConfig(t)

		cfg, _ := config.LoadConfig(configFile)
		cfg.Outputs = []config.Output{
			{Path: "output1.md"},
			{Path: "output2.md"},
			{Path: "output3.md"},
		}
		config.SaveConfig(cfg, configFile)

		cfg, _ = config.LoadConfig(configFile)
		cfg.Outputs = append(cfg.Outputs[:1], cfg.Outputs[2:]...)
		config.SaveConfig(cfg, configFile)

		reloaded, _ := config.LoadConfig(configFile)
		assert.Len(t, reloaded.Outputs, 2)
		assert.Equal(t, "output1.md", reloaded.Outputs[0].Path)
		assert.Equal(t, "output3.md", reloaded.Outputs[1].Path)
	})
}

func TestDefaultPriorities(t *testing.T) {
	configFile, _ := createTestConfig(t)

	cfg, _ := config.LoadConfig(configFile)
	cfg.Rules = []config.Rule{{Name: "rule", Content: "content"}}
	cfg.Sections = []config.Section{{Name: "section", Content: "content"}}
	cfg.Agents = []config.Agent{{Name: "agent", Description: "desc"}}
	config.SaveConfig(cfg, configFile)

	reloaded, err := config.LoadConfigWithIncludes(context.Background(), configFile)
	require.NoError(t, err)

	assert.Equal(t, config.PriorityMedium, reloaded.Rules[0].Priority)
	assert.Equal(t, config.PriorityMedium, reloaded.Sections[0].Priority)
	assert.Equal(t, config.PriorityMedium, reloaded.Agents[0].Priority)
}

func TestGetRule(t *testing.T) {
	configFile, _ := createTestConfig(t)

	cfg, _ := config.LoadConfig(configFile)
	cfg.Rules = []config.Rule{
		{Name: "test-rule", Content: "Test content", Priority: config.PriorityHigh, Targets: []string{"*.go"}},
	}
	config.SaveConfig(cfg, configFile)

	reloaded, err := config.LoadConfig(configFile)
	require.NoError(t, err)
	assert.Len(t, reloaded.Rules, 1)
	assert.Equal(t, "test-rule", reloaded.Rules[0].Name)
	assert.Equal(t, "Test content", reloaded.Rules[0].Content)
	assert.Equal(t, config.PriorityHigh, reloaded.Rules[0].Priority)
	assert.Equal(t, []string{"*.go"}, reloaded.Rules[0].Targets)
}

func TestGetSection(t *testing.T) {
	configFile, _ := createTestConfig(t)

	cfg, _ := config.LoadConfig(configFile)
	cfg.Sections = []config.Section{
		{Name: "test-section", Content: "Section content", Priority: config.PriorityLow, Targets: []string{"*.md"}},
	}
	config.SaveConfig(cfg, configFile)

	reloaded, err := config.LoadConfig(configFile)
	require.NoError(t, err)
	assert.Len(t, reloaded.Sections, 1)
	assert.Equal(t, "test-section", reloaded.Sections[0].Name)
	assert.Equal(t, "Section content", reloaded.Sections[0].Content)
	assert.Equal(t, config.PriorityLow, reloaded.Sections[0].Priority)
	assert.Equal(t, []string{"*.md"}, reloaded.Sections[0].Targets)
}

func TestGetAgent(t *testing.T) {
	configFile, _ := createTestConfig(t)

	cfg, _ := config.LoadConfig(configFile)
	cfg.Agents = []config.Agent{
		{
			Name:         "test-agent",
			Description:  "Test agent",
			Priority:     config.PriorityCritical,
			Tools:        []string{"Read", "Edit"},
			SystemPrompt: "You are a test agent",
			Targets:      []string{"*.yaml"},
		},
	}
	config.SaveConfig(cfg, configFile)

	reloaded, err := config.LoadConfig(configFile)
	require.NoError(t, err)
	assert.Len(t, reloaded.Agents, 1)
	assert.Equal(t, "test-agent", reloaded.Agents[0].Name)
	assert.Equal(t, "Test agent", reloaded.Agents[0].Description)
	assert.Equal(t, config.PriorityCritical, reloaded.Agents[0].Priority)
	assert.Equal(t, []string{"Read", "Edit"}, reloaded.Agents[0].Tools)
	assert.Equal(t, "You are a test agent", reloaded.Agents[0].SystemPrompt)
	assert.Equal(t, []string{"*.yaml"}, reloaded.Agents[0].Targets)
}

func TestTargetsSupport(t *testing.T) {
	t.Run("rule targets", func(t *testing.T) {
		configFile, _ := createTestConfig(t)

		cfg, _ := config.LoadConfig(configFile)
		cfg.Rules = []config.Rule{
			{Name: "go-rule", Content: "Go specific rule", Targets: []string{"*.go", "go.mod"}},
		}
		config.SaveConfig(cfg, configFile)

		reloaded, err := config.LoadConfig(configFile)
		require.NoError(t, err)
		assert.Equal(t, []string{"*.go", "go.mod"}, reloaded.Rules[0].Targets)
	})

	t.Run("section targets", func(t *testing.T) {
		configFile, _ := createTestConfig(t)

		cfg, _ := config.LoadConfig(configFile)
		cfg.Sections = []config.Section{
			{Name: "docs-section", Content: "Documentation section", Targets: []string{"docs/**", "*.md"}},
		}
		config.SaveConfig(cfg, configFile)

		reloaded, err := config.LoadConfig(configFile)
		require.NoError(t, err)
		assert.Equal(t, []string{"docs/**", "*.md"}, reloaded.Sections[0].Targets)
	})

	t.Run("agent targets", func(t *testing.T) {
		configFile, _ := createTestConfig(t)

		cfg, _ := config.LoadConfig(configFile)
		cfg.Agents = []config.Agent{
			{Name: "typescript-agent", Description: "TS specialist", Targets: []string{"**/*.ts", "**/*.tsx"}},
		}
		config.SaveConfig(cfg, configFile)

		reloaded, err := config.LoadConfig(configFile)
		require.NoError(t, err)
		assert.Equal(t, []string{"**/*.ts", "**/*.tsx"}, reloaded.Agents[0].Targets)
	})
}
