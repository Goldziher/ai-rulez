package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeEnforcer struct {
	out    *PolicyOutcome
	err    error
	locked map[string]bool
	seen   *Config
}

func (f *fakeEnforcer) Enforce(_ context.Context, cfg *Config) (*PolicyOutcome, error) {
	f.seen = cfg
	return f.out, f.err
}

func (f *fakeEnforcer) Locks(feature string) bool { return f.locked[feature] }

func policyProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(cfgDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte("version = \"5.0\"\nname = \"t\"\npresets = [\"claude\"]\n"), 0o644))
	return dir
}

func TestLoadWithoutEnforcerHasNoPolicyOutcome(t *testing.T) {
	// Act
	cfg, err := LoadConfig(context.Background(), policyProject(t))
	// Assert
	require.NoError(t, err)
	assert.Nil(t, cfg.PolicyOutcome)
	assert.False(t, cfg.PolicyLocks("telemetry"))
}

func TestLoadAppliesTheEnforcerBeforeIncludesResolve(t *testing.T) {
	// Arrange
	fake := &fakeEnforcer{out: &PolicyOutcome{RequiredCodes: []string{"AR001"}}}
	// Act
	cfg, err := LoadConfig(context.Background(), policyProject(t), WithPolicy(fake))
	// Assert
	require.NoError(t, err)
	assert.Same(t, cfg, fake.seen)
	assert.Equal(t, []string{"AR001"}, cfg.PolicyOutcome.RequiredCodes)
	assert.NotEmpty(t, fake.seen.ConfigDir, "the enforcer sees a configuration that knows where it lives")
}

func TestLoadFailsClosedWhenThePolicyIsUnusable(t *testing.T) {
	// Arrange
	unusable := &fakeEnforcer{err: errors.New("AR742: policy gone")}
	// Act
	cfg, err := LoadConfig(context.Background(), policyProject(t), WithPolicy(unusable))
	// Assert
	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "AR742")
}

func TestPolicyLocksAndLLMResolution(t *testing.T) {
	// Arrange
	t.Setenv("AI_RULEZ_LLM_ALLOW_NETWORK", "true")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	before, err := (&Config{}).ResolveLLM(nil)
	require.NoError(t, err)
	require.True(t, before.Config.AllowNetwork, "the environment enables network use without a policy")
	locked := &fakeEnforcer{locked: map[string]bool{"llm": true}}
	// Act
	res, err := (&Config{enforcer: locked}).ResolveLLM(nil)
	// Assert
	require.NoError(t, err)
	assert.False(t, res.Config.AllowNetwork, "the policy beats the environment")
	assert.True(t, PolicyLocks(locked, "llm"))
	assert.False(t, PolicyLocks(locked, "telemetry"))
}

func TestPolicyBelongsToTheLoadItWasGivenTo(t *testing.T) {
	// Arrange: two loads in one process, one under a policy, one under none, and a
	// third that takes its policy from the context.
	fake := &fakeEnforcer{out: &PolicyOutcome{RequiredCodes: []string{"AR001"}}}
	dir := policyProject(t)

	// Act
	under, errUnder := LoadConfig(context.Background(), dir, WithPolicy(fake))
	free, errFree := LoadConfig(context.Background(), dir)
	viaCtx, errCtx := LoadConfig(WithPolicyContext(context.Background(), fake), dir)

	// Assert
	require.NoError(t, errors.Join(errUnder, errFree, errCtx))
	assert.NotNil(t, under.PolicyOutcome)
	assert.Nil(t, free.PolicyOutcome, "a load given no policy is not bound by another load's")
	assert.NotNil(t, viaCtx.PolicyOutcome)
	assert.Same(t, fake, viaCtx.Policy())
}
