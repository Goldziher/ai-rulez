package commands

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
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
		{"a loosening attempt in warn mode is not fatal", &config.Config{PolicyOutcome: &config.PolicyOutcome{Warn: true, Violations: []config.PolicyViolation{
			{Code: "AR740", File: ".ai-rulez/config.toml", Line: 4, Message: "below the floor"},
		}}}, false},
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
	for _, name := range []string{"policy-mode", "policy-digest", "policy-offline", "policy-max-stale", "policy-trust-tofu", "discover-org", "policy-require-signed", "policy-signer-key", "policy-signer-identity", "policy-signer-issuer", "policy-trusted-root"} {
		assert.NotNil(t, RootCmd.PersistentFlags().Lookup(name), name)
	}
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

func TestBudgetsForRefusesPolicyProtectedCodes(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		Lint:          &config.LintConfig{Tolerate: map[string]int{"AR001": 5, "AR201": 2}},
		PolicyOutcome: &config.PolicyOutcome{RequiredCodes: []string{"AR001"}},
	}
	rep := &lint.Report{Root: "r", ConfigFile: ".ai-rulez/config.toml"}

	// Act
	budgets := ratchetFor(cfg)
	reportRefusedRatchet([]*lint.Report{rep}, []*config.Config{cfg})

	// Assert
	if _, ok := budgets["AR001"]; ok || budgets["AR201"] != 2 {
		t.Fatalf("budgets = %v, want only AR201", budgets)
	}
	if len(rep.Findings) != 1 || rep.Findings[0].Code != lint.CodePolicyLoosened {
		t.Fatalf("want exactly one AR740, got %v", rep.Findings)
	}
}
