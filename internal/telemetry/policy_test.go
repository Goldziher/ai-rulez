package telemetry

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

type lockTelemetry struct{}

func (lockTelemetry) Enforce(context.Context, *config.Config) (*config.PolicyOutcome, error) {
	return nil, nil
}
func (lockTelemetry) Locks(feature string) bool { return feature == "telemetry" }

func TestPolicySwitchesExportOffWhateverTheUserScopeSays(t *testing.T) {
	// Arrange
	user := &config.TelemetryConfig{Enabled: true, AllowNetwork: true, OTLPEndpoint: "https://collector.example.org"}
	before := Resolve(Layers{User: user, Getenv: env()})
	require.True(t, before.AllowNetwork, "without a policy the user scope enables export")
	config.SetPolicyEnforcer(lockTelemetry{})
	t.Cleanup(func() { config.SetPolicyEnforcer(nil) })
	// Act
	s := Resolve(Layers{User: user, Getenv: env(EnvAllowNetwork, "true")})
	// Assert
	assert.False(t, s.AllowNetwork)
	assert.False(t, s.ExportActive())
	assert.Equal(t, ScopePolicy, s.Sources["allow_network"])
	assert.True(t, s.RecordActive(), "local recording is not network use")
}
