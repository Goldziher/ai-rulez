package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateGovernance_AssuranceOwnersAndTeams(t *testing.T) {
	tests := []struct {
		name    string
		gov     *GovernanceConfig
		wantErr string
	}{
		{"defaults", &GovernanceConfig{}, ""},
		{"every assurance level", &GovernanceConfig{MinAssurance: AssuranceSigned}, ""},
		{"unknown assurance", &GovernanceConfig{MinAssurance: "notarized"}, "min_assurance"},
		{"CODEOWNERS", &GovernanceConfig{ApproversFrom: "CODEOWNERS"}, ""},
		{"a path inside the project", &GovernanceConfig{ApproversFrom: "ci/OWNERS"}, ""},
		{"an absolute path", &GovernanceConfig{ApproversFrom: "/etc/CODEOWNERS"}, "approvers_from"},
		{"a path that leaves the project", &GovernanceConfig{ApproversFrom: "../CODEOWNERS"}, "approvers_from"},
		{"a team map", &GovernanceConfig{Teams: map[string][]string{"@acme/security": {"alice@example.org"}}}, ""},
		{"a team without the org", &GovernanceConfig{Teams: map[string][]string{"security": {"alice"}}}, "invalid team"},
		{"an empty member", &GovernanceConfig{Teams: map[string][]string{"@acme/security": {""}}}, "empty member"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := &Config{Governance: tt.gov}

			// Act
			err := cfg.validateGovernance()

			// Assert
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestParseApprovalSelector_RoleOutput(t *testing.T) {
	kind, err := ParseApprovalSelector("kind:role-output")
	require.NoError(t, err)
	assert.Equal(t, "role-output", kind)
}

func TestValidateSigning_ApprovalTrustSubject(t *testing.T) {
	cfg := &Config{Signing: &SigningConfig{Trust: []SigningTrust{{Subject: "approval", KeyFile: "keys/approver.pub"}}}}
	require.NoError(t, cfg.validateSigning())

	cfg.Signing.Trust[0].Subject = "notarized"
	assert.Error(t, cfg.validateSigning())

	cfg.Signing.Trust[0].Subject = "lock"
	cfg.Signing.Require = []string{"approval"}
	assert.Error(t, cfg.validateSigning(), "require cannot name approvals")
}
