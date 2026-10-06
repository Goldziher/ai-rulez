package policy

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestValidMode(t *testing.T) {
	tests := []struct {
		mode string
		want bool
	}{
		{"", true}, {"enforce", true}, {"warn", true}, {"Warn", false}, {"off", false}, {"report", false},
	}
	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			assert.Equal(t, tt.want, ValidMode(tt.mode))
		})
	}
}

func TestEnforcerWarnModeKeepsTheClampAndMarksTheOutcome(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		wantWarn bool
	}{
		{"default mode enforces", "", false},
		{"enforce mode enforces", ModeEnforce, false},
		{"warn mode downgrades the report", ModeWarn, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			p := writePolicy(t, t.TempDir(), "p.toml", "policy_version = 1\n[lint.severity_floor]\nAR008 = \"error\"\n")
			e := NewEnforcer(func() DiscoverOptions { return DiscoverOptions{Flag: p, Env: ambient.MapEnv{}, Mode: tt.mode} })
			cfg := &config.Config{Lint: &config.LintConfig{Severity: map[string]string{"AR008": "warning"}}}
			// Act
			out, err := e.Enforce(context.Background(), cfg)
			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.wantWarn, out.Warn)
			assert.Len(t, out.Violations, 1, "the attempt is still reported")
			assert.Equal(t, "error", cfg.Lint.Severity["AR008"], "the policy value is still enforced")
		})
	}
}

func TestEnforcerRejectsAnUnknownMode(t *testing.T) {
	// Arrange
	p := writePolicy(t, t.TempDir(), "p.toml", minimalPolicy)
	e := NewEnforcer(func() DiscoverOptions { return DiscoverOptions{Flag: p, Env: ambient.MapEnv{}, Mode: "loose"} })
	// Act
	_, err := e.Enforce(context.Background(), &config.Config{})
	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AR743")
	assert.Contains(t, err.Error(), "not a policy mode")
	assert.True(t, e.Locks("telemetry"), "an unusable setup locks the network features")
}

func TestWarnModeDoesNotRelaxUnavailablePolicy(t *testing.T) {
	// Arrange
	e := NewEnforcer(func() DiscoverOptions {
		return DiscoverOptions{Flag: t.TempDir() + "/gone.toml", Env: ambient.MapEnv{}, Mode: ModeWarn}
	})
	// Act
	_, err := e.Enforce(context.Background(), &config.Config{})
	// Assert
	require.Error(t, err, "fail closed stays")
	assert.Contains(t, err.Error(), "AR742")
}

func TestReportShowsTheMode(t *testing.T) {
	// Arrange
	res := Resolve([]Layer{layer("managed", Policy{Lock: Lock{Enforce: true}})})
	res.Warn = true
	var text bytes.Buffer
	// Act
	rep := BuildReport(res, nil)
	rep.WriteText(&text)
	// Assert
	assert.Equal(t, ModeWarn, rep.Mode)
	assert.Contains(t, text.String(), "mode: warn")
	assert.Empty(t, BuildReport(Resolve([]Layer{layer("managed", Policy{})}), nil).Mode, "enforce mode is not written")
}
