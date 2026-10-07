package handlers

import (
	"context"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// loosening reports a violation for every configuration.
type loosening struct{}

func (loosening) Enforce(context.Context, *config.Config) (*config.PolicyOutcome, error) {
	return &config.PolicyOutcome{Violations: []config.PolicyViolation{{Code: "AR740", Key: "lock.enforce", Message: "loosened"}}}, nil
}

func (loosening) Locks(string) bool { return false }

func TestMCPGenerateAndValidateRefusePolicyViolations(t *testing.T) {
	// Arrange
	dir := driftProject(t)
	ctx := config.WithPolicyContext(context.Background(), loosening{})
	args := map[string]any{"working_directory": dir, "no_local": true}

	// Act
	gen, err := GenerateOutputsHandler(ctx, newRequestWithArgs(args))
	require.NoError(t, err)
	val, err := ValidateConfigHandler(ctx, newRequestWithArgs(args))
	require.NoError(t, err)

	// Assert
	assert.True(t, gen.IsError, "generate must refuse a configuration that loosens the policy")
	payload := resultPayload(t, val)
	assert.Equal(t, false, payload["valid"])
	assert.Contains(t, payload["error"], "loosens the organization policy")
}
