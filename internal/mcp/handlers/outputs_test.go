package handlers_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/crud"
)

func TestOutputHandlers(t *testing.T) {
	configFile := setupTestConfig(t)

	t.Run("get outputs", func(t *testing.T) {
		cfg, err := config.LoadConfig(configFile)
		require.NoError(t, err)

		assert.Len(t, cfg.Outputs, 1)
		assert.Equal(t, "test.md", cfg.Outputs[0].Path)
	})

	t.Run("add output", func(t *testing.T) {
		cfg, err := config.LoadConfig(configFile)
		require.NoError(t, err)

		newOutput := config.Output{
			Path:     "new-output.md",
			Template: crud.CreateTemplateConfig("builtin", "custom"),
		}

		cfg.Outputs = append(cfg.Outputs, newOutput)
		require.NoError(t, config.SaveConfig(cfg, configFile))

		reloaded, err := config.LoadConfig(configFile)
		require.NoError(t, err)
		assert.Greater(t, len(reloaded.Outputs), 1)
	})

	t.Run("update output", func(t *testing.T) {
		cfg, err := config.LoadConfig(configFile)
		require.NoError(t, err)

		if len(cfg.Outputs) > 0 {
			cfg.Outputs[0].Template = crud.CreateTemplateConfig("builtin", "updated-template")
			require.NoError(t, config.SaveConfig(cfg, configFile))

			reloaded, err := config.LoadConfig(configFile)
			require.NoError(t, err)
			template, err := reloaded.Outputs[0].GetTemplate()
			require.NoError(t, err)
			if template != nil {
				assert.Equal(t, "updated-template", template.Value)
			}
		}
	})

	t.Run("delete output", func(t *testing.T) {
		cfg, err := config.LoadConfig(configFile)
		require.NoError(t, err)

		if len(cfg.Outputs) == 1 {
			cfg.Outputs = append(cfg.Outputs, config.Output{
				Path: "to-delete.md",
			})
			require.NoError(t, config.SaveConfig(cfg, configFile))
			cfg, err = config.LoadConfig(configFile)
			require.NoError(t, err)
		}

		if len(cfg.Outputs) > 1 {
			initialCount := len(cfg.Outputs)
			cfg.Outputs = cfg.Outputs[:1]
			require.NoError(t, config.SaveConfig(cfg, configFile))

			reloaded, err := config.LoadConfig(configFile)
			require.NoError(t, err)
			assert.Less(t, len(reloaded.Outputs), initialCount)
		}
	})
}
