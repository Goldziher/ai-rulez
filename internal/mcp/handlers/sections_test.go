package handlers_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func TestSectionHandlers(t *testing.T) {
	configFile := setupTestConfig(t)

	t.Run("get sections", func(t *testing.T) {
		cfg, err := config.LoadConfig(configFile)
		require.NoError(t, err)

		assert.Len(t, cfg.Sections, 1)
		assert.Equal(t, "Test Section", cfg.Sections[0].Name)
	})

	t.Run("add section", func(t *testing.T) {
		cfg, err := config.LoadConfig(configFile)
		require.NoError(t, err)

		newSection := config.Section{
			Name:     "New Section",
			Content:  "New content",
			Priority: config.PriorityCritical,
		}

		cfg.Sections = append(cfg.Sections, newSection)
		require.NoError(t, config.SaveConfig(cfg, configFile))

		reloaded, err := config.LoadConfig(configFile)
		require.NoError(t, err)
		assert.Greater(t, len(reloaded.Sections), 1)
	})

	t.Run("update section", func(t *testing.T) {
		cfg, err := config.LoadConfig(configFile)
		require.NoError(t, err)

		if len(cfg.Sections) > 0 {
			cfg.Sections[0].Content = "Updated content"
			cfg.Sections[0].Priority = config.PriorityCritical
			require.NoError(t, config.SaveConfig(cfg, configFile))

			reloaded, err := config.LoadConfig(configFile)
			require.NoError(t, err)
			assert.Equal(t, "Updated content", reloaded.Sections[0].Content)
			assert.Equal(t, config.PriorityCritical, reloaded.Sections[0].Priority)
		}
	})

	t.Run("delete section", func(t *testing.T) {
		cfg, err := config.LoadConfig(configFile)
		require.NoError(t, err)

		if len(cfg.Sections) > 1 {
			cfg.Sections = cfg.Sections[1:]
			require.NoError(t, config.SaveConfig(cfg, configFile))

			reloaded, err := config.LoadConfig(configFile)
			require.NoError(t, err)
			assert.Less(t, len(reloaded.Sections), len(cfg.Sections)+1)
		}
	})
}
