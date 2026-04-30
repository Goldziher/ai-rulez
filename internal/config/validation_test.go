package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateEffort(t *testing.T) {
	t.Run("accepts empty string", func(t *testing.T) {
		assert.NoError(t, validateEffort("", "defaults.effort"))
	})

	t.Run("accepts all spec values", func(t *testing.T) {
		for _, v := range []string{"low", "medium", "high", "xhigh", "max", "inherit"} {
			assert.NoError(t, validateEffort(v, "defaults.effort"), "value %q should pass", v)
		}
	})

	t.Run("rejects unknown value", func(t *testing.T) {
		err := validateEffort("extreme", "defaults.effort")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "extreme")
		assert.Contains(t, err.Error(), "defaults.effort")
	})

	t.Run("is case-sensitive", func(t *testing.T) {
		err := validateEffort("HIGH", "defaults.effort")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "HIGH")
	})
}

func TestConfigValidateDefaults(t *testing.T) {
	base := func() *Config {
		return &Config{
			Version: "4.0",
			Name:    "test",
			Presets: []Preset{{BuiltIn: "claude"}},
		}
	}

	t.Run("nil defaults is valid", func(t *testing.T) {
		cfg := base()
		assert.NoError(t, cfg.Validate())
	})

	t.Run("valid defaults.effort passes", func(t *testing.T) {
		cfg := base()
		cfg.Defaults = &DefaultsConfig{Effort: "high"}
		assert.NoError(t, cfg.Validate())
	})

	t.Run("invalid defaults.effort fails", func(t *testing.T) {
		cfg := base()
		cfg.Defaults = &DefaultsConfig{Effort: "extreme"}
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "defaults.effort")
	})

	t.Run("empty defaults.effort is treated as not set", func(t *testing.T) {
		cfg := base()
		cfg.Defaults = &DefaultsConfig{Effort: ""}
		assert.NoError(t, cfg.Validate())
	})

	t.Run("valid defaults.effort_by_preset passes", func(t *testing.T) {
		cfg := base()
		cfg.Defaults = &DefaultsConfig{
			EffortByPreset: map[string]string{"claude": "xhigh", "codex": "high"},
		}
		assert.NoError(t, cfg.Validate())
	})

	t.Run("invalid effort value in effort_by_preset fails with preset path", func(t *testing.T) {
		cfg := base()
		cfg.Defaults = &DefaultsConfig{
			EffortByPreset: map[string]string{"codex": "extreme"},
		}
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "defaults.effort_by_preset.codex")
	})

	t.Run("unknown preset key in effort_by_preset fails", func(t *testing.T) {
		cfg := base()
		cfg.Defaults = &DefaultsConfig{
			EffortByPreset: map[string]string{"not-a-preset": "high"},
		}
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not-a-preset")
	})

	// Normalization contract: users always write the canonical vocabulary, never
	// preset-native values. Provider-specific values like Codex's "minimal" must
	// be rejected — they only exist in generated output, never in user config.
	t.Run("provider-native values are rejected at every input site", func(t *testing.T) {
		providerNative := []string{"minimal", "MAX", "extreme", "xxhigh"}
		for _, v := range providerNative {
			t.Run("defaults.effort/"+v, func(t *testing.T) {
				cfg := base()
				cfg.Defaults = &DefaultsConfig{Effort: v}
				assert.Error(t, cfg.Validate(), "value %q must be rejected from defaults.effort", v)
			})
			t.Run("defaults.effort_by_preset/"+v, func(t *testing.T) {
				cfg := base()
				cfg.Defaults = &DefaultsConfig{EffortByPreset: map[string]string{"codex": v}}
				assert.Error(t, cfg.Validate(), "value %q must be rejected from defaults.effort_by_preset", v)
			})
			t.Run("agent.metadata.effort/"+v, func(t *testing.T) {
				cfg := base()
				cfg.Content = &ContentTree{Agents: []ContentFile{
					{Name: "x", Metadata: &Metadata{Effort: v}},
				}}
				assert.Error(t, cfg.Validate(), "value %q must be rejected from agent metadata", v)
			})
		}
	})
}

func TestConfigValidateAgentEffort(t *testing.T) {
	base := func() *Config {
		return &Config{
			Version: "4.0",
			Name:    "test",
			Presets: []Preset{{BuiltIn: "claude"}},
			Content: &ContentTree{},
		}
	}

	t.Run("valid root agent effort passes", func(t *testing.T) {
		cfg := base()
		cfg.Content.Agents = []ContentFile{
			{Name: "reviewer", Metadata: &Metadata{Effort: "high"}},
		}
		assert.NoError(t, cfg.Validate())
	})

	t.Run("invalid root agent effort fails with agent name in error", func(t *testing.T) {
		cfg := base()
		cfg.Content.Agents = []ContentFile{
			{Name: "reviewer", Metadata: &Metadata{Effort: "extreme"}},
		}
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reviewer")
		assert.Contains(t, err.Error(), "extreme")
	})

	t.Run("invalid domain agent effort fails", func(t *testing.T) {
		cfg := base()
		cfg.Content.Domains = map[string]*Domain{
			"backend": {
				Name: "backend",
				Agents: []ContentFile{
					{Name: "db-tuner", Metadata: &Metadata{Effort: "BOGUS"}},
				},
			},
		}
		err := cfg.Validate()
		require.Error(t, err)
		assert.True(t,
			strings.Contains(err.Error(), "backend") || strings.Contains(err.Error(), "db-tuner"),
			"error should reference scope or agent name; got: %s", err.Error())
	})

	t.Run("agent without metadata is fine", func(t *testing.T) {
		cfg := base()
		cfg.Content.Agents = []ContentFile{{Name: "noop"}}
		assert.NoError(t, cfg.Validate())
	})
}
