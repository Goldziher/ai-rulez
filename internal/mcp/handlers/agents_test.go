package handlers_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func TestAgentHandlers(t *testing.T) {
	configFile := setupTestConfig(t)

	t.Run("get agents", func(t *testing.T) {
		cfg, err := config.LoadConfig(configFile)
		require.NoError(t, err)

		assert.Len(t, cfg.Agents, 1)
		assert.Equal(t, "test-agent", cfg.Agents[0].Name)
		assert.ElementsMatch(t, []string{"tool1", "tool2"}, cfg.Agents[0].Tools)
	})

	t.Run("add agent", func(t *testing.T) {
		cfg, err := config.LoadConfig(configFile)
		require.NoError(t, err)

		newAgent := config.Agent{
			Name:         "new-agent",
			Description:  "New agent",
			Priority:     config.PriorityCritical,
			Tools:        []string{"read", "write"},
			SystemPrompt: "You are a new agent",
		}

		cfg.Agents = append(cfg.Agents, newAgent)
		require.NoError(t, config.SaveConfig(cfg, configFile))

		reloaded, err := config.LoadConfig(configFile)
		require.NoError(t, err)
		assert.Greater(t, len(reloaded.Agents), 1)

		var found bool
		for _, agent := range reloaded.Agents {
			if agent.Name == "new-agent" {
				found = true
				assert.ElementsMatch(t, []string{"read", "write"}, agent.Tools)
				break
			}
		}
		assert.True(t, found, "New agent should be found")
	})

	t.Run("update agent", func(t *testing.T) {
		cfg, err := config.LoadConfig(configFile)
		require.NoError(t, err)

		if len(cfg.Agents) > 0 {
			cfg.Agents[0].Description = "Updated description"
			cfg.Agents[0].Priority = config.PriorityCritical
			cfg.Agents[0].Tools = []string{"execute"}
			require.NoError(t, config.SaveConfig(cfg, configFile))

			reloaded, err := config.LoadConfig(configFile)
			require.NoError(t, err)
			assert.Equal(t, "Updated description", reloaded.Agents[0].Description)
			assert.Equal(t, config.PriorityCritical, reloaded.Agents[0].Priority)
			assert.Equal(t, []string{"execute"}, reloaded.Agents[0].Tools)
		}
	})

	t.Run("delete agent", func(t *testing.T) {
		cfg, err := config.LoadConfig(configFile)
		require.NoError(t, err)

		if len(cfg.Agents) > 1 {
			initialCount := len(cfg.Agents)
			cfg.Agents = cfg.Agents[1:]
			require.NoError(t, config.SaveConfig(cfg, configFile))

			reloaded, err := config.LoadConfig(configFile)
			require.NoError(t, err)
			assert.Len(t, reloaded.Agents, initialCount-1)
		}
	})
}
