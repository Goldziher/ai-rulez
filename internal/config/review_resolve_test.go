package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReviewConfigValidateNewKeys(t *testing.T) {
	yes, no := true, false
	tests := []struct {
		name string
		cfg  *ReviewConfig
		want string
	}{
		{"defaults", &ReviewConfig{}, ""},
		{"redact", &ReviewConfig{OnSecret: "redact"}, ""},
		{"bad on_secret", &ReviewConfig{OnSecret: "mask"}, "review.on_secret"},
		{"host list", &ReviewConfig{AllowedHosts: []string{"gateway.internal", "gateway.internal:8443", "provider-default"}}, ""},
		{"host with a scheme", &ReviewConfig{AllowedHosts: []string{"https://gateway.internal"}}, "allowed_hosts"},
		{"host with a path", &ReviewConfig{AllowedHosts: []string{"gateway.internal/v1"}}, "allowed_hosts"},
		{"gate", &ReviewConfig{Gate: &ReviewGateConfig{Level: "info", RequireCalibration: &no, CalibrationMaxAgeDays: 30}}, ""},
		{"bad gate level", &ReviewConfig{Gate: &ReviewGateConfig{Level: "fatal"}}, "review.gate.level"},
		{"negative age", &ReviewConfig{Gate: &ReviewGateConfig{CalibrationMaxAgeDays: -1, RequireCalibration: &yes}}, "calibration_max_age_days"},
		{"fix", &ReviewConfig{Fix: &ReviewFixConfig{Model: "gemini-2.5-flash", MaxGrowthPercent: 40}}, ""},
		{"negative growth", &ReviewConfig{Fix: &ReviewFixConfig{MaxGrowthPercent: -5}}, "max_growth_percent"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			problems := tt.cfg.Validate()
			if tt.want == "" {
				assert.Empty(t, problems)
				return
			}
			require.NotEmpty(t, problems)
			assert.Contains(t, problems[0], tt.want)
		})
	}
}

func TestReviewConfigDefaults(t *testing.T) {
	var nilCfg *ReviewConfig
	no := false
	assert.Equal(t, ReviewOnSecretWithhold, nilCfg.OnSecretMode())
	assert.Equal(t, ReviewGateWarning, nilCfg.GateLevel())
	assert.True(t, nilCfg.GateRequiresCalibration())
	assert.Equal(t, 25, nilCfg.FixMaxGrowthPercent())
	cfg := &ReviewConfig{OnSecret: "redact", Gate: &ReviewGateConfig{Level: "info", RequireCalibration: &no}, Fix: &ReviewFixConfig{MaxGrowthPercent: 10}}
	assert.Equal(t, "redact", cfg.OnSecretMode())
	assert.Equal(t, "info", cfg.GateLevel())
	assert.False(t, cfg.GateRequiresCalibration())
	assert.Equal(t, 10, cfg.FixMaxGrowthPercent())
}

func TestResolveReviewIsUserScopeOnly(t *testing.T) {
	// Arrange
	home := t.TempDir()
	userDir := filepath.Join(home, "ai-rulez")
	require.NoError(t, os.MkdirAll(userDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(userDir, "config.toml"), []byte("[review]\nallowed_hosts = [\"gateway.internal\"]\nmax_cost_usd = 5\nmax_calls = 900\n"), 0o600))
	getenv := func(k string) string {
		if k == "XDG_CONFIG_HOME" {
			return home
		}
		return ""
	}
	repo := &Config{Review: &ReviewConfig{AllowedHosts: []string{"evil.example"}, MaxCostUSD: 100}}

	// Act
	got, err := repo.ResolveReview(getenv)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{"gateway.internal"}, got.AllowedHosts, "the repository list is ignored")
	assert.Equal(t, []string{"allowed_hosts"}, got.IgnoredRepoKeys)
	assert.True(t, got.HostAllowed("gateway.internal"))
	assert.False(t, got.HostAllowed("evil.example"))
	assert.False(t, got.HostAllowed(""), "the provider's own endpoint is not on the list")
	cost, calls := got.Caps(repo.Review, 0.5, 300)
	assert.Equal(t, 5.0, cost, "a user-scope value wins")
	assert.Equal(t, 900, calls)
}

func TestResolveReviewEnvironmentOverridesTheList(t *testing.T) {
	getenv := func(k string) string {
		if k == "AI_RULEZ_REVIEW_ALLOWED_HOSTS" {
			return "a.example, b.example:8443,"
		}
		return ""
	}

	got, err := (&Config{}).ResolveReview(getenv)

	require.NoError(t, err)
	assert.Equal(t, []string{"a.example", "b.example:8443"}, got.AllowedHosts)
	assert.True(t, got.HostAllowed("B.example:8443"))
	assert.True(t, ReviewResolution{AllowedHosts: []string{ReviewProviderDefaultHost}}.HostAllowed(""))
	assert.True(t, ReviewResolution{}.HostAllowed("anything"), "no list, no restriction beyond the trust rule")
}

func TestReviewCapsRepositoryOnlyTightens(t *testing.T) {
	tests := []struct {
		name      string
		user      ReviewResolution
		repo      *ReviewConfig
		wantCost  float64
		wantCalls int
	}{
		{"defaults", ReviewResolution{}, nil, 0.5, 300},
		{"a repository raising is ignored", ReviewResolution{}, &ReviewConfig{MaxCostUSD: 50, MaxCalls: 5000}, 0.5, 300},
		{"a repository lowering applies", ReviewResolution{}, &ReviewConfig{MaxCostUSD: 0.1, MaxCalls: 20}, 0.1, 20},
		{"user scope may raise", ReviewResolution{MaxCostUSD: 3, MaxCalls: 1000}, &ReviewConfig{MaxCostUSD: 0.1}, 3, 1000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cost, calls := tt.user.Caps(tt.repo, 0.5, 300)
			assert.Equal(t, tt.wantCost, cost)
			assert.Equal(t, tt.wantCalls, calls)
		})
	}
}
