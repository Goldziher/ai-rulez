package mcp

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type loosening struct{}

func (loosening) Enforce(context.Context, *config.Config) (*config.PolicyOutcome, error) {
	return &config.PolicyOutcome{Violations: []config.PolicyViolation{{Code: "AR740", Key: "lock.enforce", Message: "loosened"}}}, nil
}

func (loosening) Locks(string) bool { return false }

func TestServeSetup_RefusesAConfigurationThatLoosensThePolicy(t *testing.T) {
	// Arrange
	config.SetPolicyEnforcer(loosening{})
	t.Cleanup(func() { config.SetPolicyEnforcer(nil) })
	setup := &ServeSetup{WorkDir: project(t, baseConfig, nil), NoWatch: true, CacheDir: filepath.Join(t.TempDir(), "cache")}

	// Act
	_, err := setup.NewServer(context.Background())

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loosens the organization policy")
}
