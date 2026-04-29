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
