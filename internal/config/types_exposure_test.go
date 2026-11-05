package config

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExposureConfigValidate(t *testing.T) {
	tests := []struct {
		name string
		cfg  *ExposureConfig
		want []string
	}{
		{"nil", nil, nil},
		{"empty", &ExposureConfig{}, nil},
		{"valid", &ExposureConfig{Harness: "claude", Plugins: "all", ContextWindow: 1000, StartBudget: 10, CollisionMinScore: 0.5}, nil},
		{"harness", &ExposureConfig{Harness: "gemini"}, []string{`exposure.harness "gemini" must be "claude" or "codex"`}},
		{"plugins", &ExposureConfig{Plugins: "some"}, []string{`exposure.plugins "some" must be "none", "enabled" or "all"`}},
		{"negative numbers", &ExposureConfig{ContextWindow: -1, StartBudget: -1}, []string{"exposure.context_window must not be negative", "exposure.start_budget must not be negative"}},
		{"score", &ExposureConfig{CollisionMinScore: 1.5}, []string{"exposure.collision_min_score must be between 0 and 1, got 1.5"}},
		{"nan score", &ExposureConfig{CollisionMinScore: math.NaN()}, []string{"exposure.collision_min_score must be between 0 and 1, got NaN"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.cfg.Validate())
		})
	}
}

func TestExposureConfigDefaults(t *testing.T) {
	var unset *ExposureConfig
	assert.Equal(t, ExposureDefaultContextWindow, unset.EffectiveContextWindow())
	assert.Equal(t, ExposurePluginsEnabled, unset.EffectivePlugins())
	set := &ExposureConfig{ContextWindow: 1000, Plugins: ExposurePluginsNone}
	assert.Equal(t, 1000, set.EffectiveContextWindow())
	assert.Equal(t, ExposurePluginsNone, set.EffectivePlugins())
}

func TestExposureTableLoads(t *testing.T) {
	cfg, err := decodeConfigTOML([]byte("version = \"5.0\"\nname = \"x\"\n[exposure]\nareas_file = \"tools/areas.toml\"\n[[exposure.areas]]\nname = \"billing\"\npaths = [\"services/billing\"]\nshared = true\n[exposure.skill_areas]\ndeploy = \"billing\"\n"), "config.toml")
	if assert.NoError(t, err) && assert.NotNil(t, cfg.Exposure) {
		assert.Equal(t, "tools/areas.toml", cfg.Exposure.AreasFile)
		assert.Equal(t, []ExposureArea{{Name: "billing", Paths: []string{"services/billing"}, Shared: true}}, cfg.Exposure.Areas)
		assert.Equal(t, map[string]string{"deploy": "billing"}, cfg.Exposure.SkillAreas)
	}
}

func TestExposureConfig_CodexAndMcpKeys(t *testing.T) {
	assert.Empty(t, (&ExposureConfig{Harness: "codex", CodexStartBudget: 5800, McpServerTokens: 400}).Validate())
	assert.Equal(t,
		[]string{"exposure.codex_start_budget must not be negative", "exposure.mcp_server_tokens must not be negative"},
		(&ExposureConfig{CodexStartBudget: -1, McpServerTokens: -1}).Validate())
	assert.Contains(t, (&ExposureConfig{Harness: "gemini"}).Validate()[0], "must be")

	c := &ExposureConfig{StartBudget: 5350, CodexStartBudget: 5800}
	assert.Equal(t, 5350, c.EffectiveStartBudget(ExposureHarnessClaude))
	assert.Equal(t, 5800, c.EffectiveStartBudget(ExposureHarnessCodex))
	assert.Equal(t, 0, (*ExposureConfig)(nil).EffectiveStartBudget(ExposureHarnessCodex))
}
