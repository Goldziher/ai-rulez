package commands

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestPolicyGate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *config.Config
		wantErr bool
	}{
		{"nil config", nil, false},
		{"no policy in force", &config.Config{}, false},
		{"policy with nothing to report", &config.Config{PolicyOutcome: &config.PolicyOutcome{}}, false},
		{"a loosening attempt", &config.Config{PolicyOutcome: &config.PolicyOutcome{Violations: []config.PolicyViolation{
			{Code: "AR740", File: ".ai-rulez/config.toml", Line: 4, Message: "below the floor"},
		}}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			err := policyGate(tt.cfg)
			// Assert
			if !tt.wantErr {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), "AR740 .ai-rulez/config.toml:4  below the floor")
			assert.Contains(t, err.Error(), "loosens the organization policy")
		})
	}
}

func TestPolicyFlagIsGlobalAndShowPolicyIsOnValidate(t *testing.T) {
	assert.NotNil(t, RootCmd.PersistentFlags().Lookup("policy"))
	assert.NotNil(t, ValidateCmd.Flags().Lookup("show-policy"))
}

func TestShowPolicyWithoutPolicyPrintsNone(t *testing.T) {
	// Arrange
	t.Setenv("AI_RULEZ_POLICY", "")
	old := policyFlag
	policyFlag = ""
	t.Cleanup(func() { policyFlag = old })
	var out bytes.Buffer
	// Act
	code := runShowPolicy(t.Context(), nil, &out)
	// Assert
	assert.Equal(t, 0, code)
	assert.Contains(t, out.String(), "policy: none")
}
