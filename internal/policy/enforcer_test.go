package policy

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestEnforcerWithoutPolicyChangesNothing(t *testing.T) {
	// Arrange
	e := NewEnforcer(func() DiscoverOptions {
		return DiscoverOptions{Env: ambient.MapEnv{}, ManagedPaths: []string{filepath.Join(t.TempDir(), "none.toml")}}
	})
	cfg := &config.Config{Lint: &config.LintConfig{Severity: map[string]string{"AR001": "off"}}}
	// Act
	out, err := e.Enforce(context.Background(), cfg)
	// Assert
	require.NoError(t, err)
	assert.Nil(t, out)
	assert.Equal(t, "off", cfg.Lint.Severity["AR001"])
	assert.False(t, e.Locks("telemetry"))
	assert.False(t, e.Locks("llm"))
}

func TestEnforcerReloadsWhenTheAnchorsChange(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	locked := writePolicy(t, dir, "locked.toml", "policy_version = 1\n[telemetry]\nallow_network = false\n")
	flag := ""
	e := NewEnforcer(func() DiscoverOptions {
		return DiscoverOptions{Flag: flag, Env: ambient.MapEnv{}, ManagedPaths: []string{filepath.Join(dir, "none.toml")}}
	})
	// Act and Assert
	assert.False(t, e.Locks("telemetry"))
	flag = locked
	assert.True(t, e.Locks("telemetry"))
	assert.False(t, e.Locks("llm"))
	assert.False(t, e.Locks("unknown"))
	flag = ""
	assert.False(t, e.Locks("telemetry"))
}

func TestEnforcerFailsClosed(t *testing.T) {
	// Arrange
	e := NewEnforcer(func() DiscoverOptions {
		return DiscoverOptions{Flag: filepath.Join(t.TempDir(), "gone.toml"), Env: ambient.MapEnv{}}
	})
	// Act
	_, err := e.Enforce(context.Background(), &config.Config{})
	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AR742")
	assert.True(t, e.Locks("telemetry"), "an unusable policy locks the network features")
	assert.True(t, e.Locks("llm"))
}

func TestEnforcerAppliesAndRecordsTheOutcome(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	p := writePolicy(t, dir, "p.toml", "policy_version = 1\n[lint.severity_floor]\nAR008 = \"error\"\n")
	e := NewEnforcer(func() DiscoverOptions { return DiscoverOptions{Flag: p, Env: ambient.MapEnv{}} })
	cfg := &config.Config{Lint: &config.LintConfig{Severity: map[string]string{"AR008": "warning"}}}
	// Act
	out, err := e.Enforce(context.Background(), cfg)
	// Assert
	require.NoError(t, err)
	require.Len(t, out.Violations, 1)
	assert.Equal(t, "error", cfg.Lint.Severity["AR008"])
	assert.Equal(t, "error", out.SeverityFloor["AR008"])
	_, statErr := os.Stat(p)
	require.NoError(t, statErr)
}
