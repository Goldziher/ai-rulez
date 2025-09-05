package handlers_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func TestProjectOperations(t *testing.T) {
	configFile := setupTestConfig(t)

	t.Run("validate config", func(t *testing.T) {
		cfg, err := config.LoadConfig(configFile)
		require.NoError(t, err)
		assert.NotNil(t, cfg)

		assert.NotEmpty(t, cfg.Metadata.Name)
		assert.NotEmpty(t, cfg.Outputs)
	})

	t.Run("get version", func(t *testing.T) {
		cfg, err := config.LoadConfig(configFile)
		require.NoError(t, err)
		assert.Equal(t, "1.0.0", cfg.Metadata.Version)
	})
}

func setupTestConfig(t *testing.T) string {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "ai_rulez.yaml")

	cfg := &config.Config{
		Metadata: config.Metadata{
			Name:        "Test Project",
			Version:     "1.0.0",
			Description: "Test project for MCP handlers",
		},
		Outputs: []config.Output{
			{Path: "test.md"},
		},
		Rules: []config.Rule{
			{
				Name:     "test-rule",
				Content:  "Test rule content",
				Priority: config.PriorityMedium,
			},
		},
		Sections: []config.Section{
			{
				Name:     "Test Section",
				Content:  "Test section content",
				Priority: config.PriorityLow,
			},
		},
		Agents: []config.Agent{
			{
				Name:         "test-agent",
				Description:  "Test agent",
				Priority:     config.PriorityMedium,
				Tools:        []string{"tool1", "tool2"},
				SystemPrompt: "You are a test agent",
			},
		},
	}

	err := config.SaveConfig(cfg, configFile)
	require.NoError(t, err)

	originalDir, _ := os.Getwd()
	t.Cleanup(func() {
		_ = os.Chdir(originalDir)
	})
	_ = os.Chdir(tempDir)

	return configFile
}
