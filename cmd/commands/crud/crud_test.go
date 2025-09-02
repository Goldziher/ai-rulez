package crud_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/internal/config"
)

// Helper function to create a test config
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

// ========== Rules CRUD Tests ==========

func TestRuleCRUD(t *testing.T) {
	t.Run("add rule", func(t *testing.T) {
		configFile, _ := createTestConfig(t)

		cfg, err := config.LoadConfig(configFile)
		require.NoError(t, err)

		newRule := config.Rule{
			Name:     "test-rule",
			Content:  "Test rule content",
			Priority: 5,
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
		cfg.Rules = []config.Rule{{Name: "rule1", Content: "Original", Priority: 3}}
		config.SaveConfig(cfg, configFile)

		cfg, _ = config.LoadConfig(configFile)
		cfg.Rules[0].Content = "Updated"
		cfg.Rules[0].Priority = 10
		require.NoError(t, config.SaveConfig(cfg, configFile))

		reloaded, _ := config.LoadConfig(configFile)
		assert.Equal(t, "Updated", reloaded.Rules[0].Content)
		assert.Equal(t, 10, reloaded.Rules[0].Priority)
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

// ========== Sections CRUD Tests ==========

func TestSectionCRUD(t *testing.T) {
	t.Run("add section", func(t *testing.T) {
		configFile, _ := createTestConfig(t)

		cfg, err := config.LoadConfig(configFile)
		require.NoError(t, err)

		newSection := config.Section{
			Name:     "Test Section",
			Content:  "Section content",
			Priority: 5,
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
		cfg.Sections = []config.Section{{Name: "Section1", Content: "Original", Priority: 3}}
		config.SaveConfig(cfg, configFile)

		cfg, _ = config.LoadConfig(configFile)
		cfg.Sections[0].Content = "Updated"
		cfg.Sections[0].Priority = 10
		require.NoError(t, config.SaveConfig(cfg, configFile))

		reloaded, _ := config.LoadConfig(configFile)
		assert.Equal(t, "Updated", reloaded.Sections[0].Content)
		assert.Equal(t, 10, reloaded.Sections[0].Priority)
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

// ========== Agents CRUD Tests ==========

func TestAgentCRUD(t *testing.T) {
	t.Run("add agent", func(t *testing.T) {
		configFile, _ := createTestConfig(t)

		cfg, err := config.LoadConfig(configFile)
		require.NoError(t, err)

		newAgent := config.Agent{
			Name:         "test-agent",
			Description:  "Test agent",
			Priority:     5,
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
			{Name: "agent1", Description: "Original", Priority: 3},
		}
		config.SaveConfig(cfg, configFile)

		cfg, _ = config.LoadConfig(configFile)
		cfg.Agents[0].Description = "Updated"
		cfg.Agents[0].Priority = 10
		cfg.Agents[0].Tools = []string{"execute"}
		require.NoError(t, config.SaveConfig(cfg, configFile))

		reloaded, _ := config.LoadConfig(configFile)
		assert.Equal(t, "Updated", reloaded.Agents[0].Description)
		assert.Equal(t, 10, reloaded.Agents[0].Priority)
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

// ========== Outputs CRUD Tests ==========

func TestOutputCRUD(t *testing.T) {
	t.Run("add output", func(t *testing.T) {
		configFile, _ := createTestConfig(t)

		cfg, err := config.LoadConfig(configFile)
		require.NoError(t, err)

		newOutput := config.Output{
			Path:     "new-output.md",
			Template: "custom",
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
		cfg.Outputs[0].Template = "updated-template"
		require.NoError(t, config.SaveConfig(cfg, configFile))

		reloaded, _ := config.LoadConfig(configFile)
		assert.Equal(t, "updated-template", reloaded.Outputs[0].Template)
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

// ========== Validation Tests ==========

func TestDefaultPriorities(t *testing.T) {
	configFile, _ := createTestConfig(t)

	cfg, _ := config.LoadConfig(configFile)
	cfg.Rules = []config.Rule{{Name: "rule", Content: "content"}}
	cfg.Sections = []config.Section{{Name: "section", Content: "content"}}
	cfg.Agents = []config.Agent{{Name: "agent", Description: "desc"}}
	config.SaveConfig(cfg, configFile)

	reloaded, err := config.LoadConfigWithIncludes(context.Background(), configFile)
	require.NoError(t, err)

	assert.Equal(t, 1, reloaded.Rules[0].Priority)
	assert.Equal(t, 1, reloaded.Sections[0].Priority)
	assert.Equal(t, 1, reloaded.Agents[0].Priority)
}
