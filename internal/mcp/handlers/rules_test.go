package handlers_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func TestRuleHandlers(t *testing.T) {
	configFile := setupTestConfig(t)

	t.Run("get rules", func(t *testing.T) {
		cfg, err := config.LoadConfig(configFile)
		require.NoError(t, err)

		assert.Len(t, cfg.Rules, 1)
		assert.Equal(t, "test-rule", cfg.Rules[0].Name)
	})

	t.Run("add rule", func(t *testing.T) {
		cfg, err := config.LoadConfig(configFile)
		require.NoError(t, err)

		newRule := config.Rule{
			Name:     "new-rule",
			Content:  "New content",
			Priority: 10,
		}

		cfg.Rules = append(cfg.Rules, newRule)
		require.NoError(t, config.SaveConfig(cfg, configFile))

		// Verify
		reloaded, err := config.LoadConfig(configFile)
		require.NoError(t, err)
		assert.Len(t, reloaded.Rules, 2)
		assert.Equal(t, "new-rule", reloaded.Rules[1].Name)
	})

	t.Run("update rule", func(t *testing.T) {
		cfg, err := config.LoadConfig(configFile)
		require.NoError(t, err)

		if len(cfg.Rules) > 0 {
			cfg.Rules[0].Content = "Updated content"
			cfg.Rules[0].Priority = 10
			require.NoError(t, config.SaveConfig(cfg, configFile))

			// Verify
			reloaded, err := config.LoadConfig(configFile)
			require.NoError(t, err)
			assert.Equal(t, "Updated content", reloaded.Rules[0].Content)
			assert.Equal(t, 10, reloaded.Rules[0].Priority)
		}
	})

	t.Run("delete rule", func(t *testing.T) {
		// Setup: ensure we have at least 2 rules
		cfg, err := config.LoadConfig(configFile)
		require.NoError(t, err)

		if len(cfg.Rules) < 2 {
			cfg.Rules = append(cfg.Rules, config.Rule{
				Name:    "to-delete",
				Content: "Will be deleted",
			})
			require.NoError(t, config.SaveConfig(cfg, configFile))
			cfg, err = config.LoadConfig(configFile)
			require.NoError(t, err)
		}

		initialCount := len(cfg.Rules)
		cfg.Rules = cfg.Rules[1:]
		require.NoError(t, config.SaveConfig(cfg, configFile))

		// Verify
		reloaded, err := config.LoadConfig(configFile)
		require.NoError(t, err)
		assert.Len(t, reloaded.Rules, initialCount-1)
	})
}
