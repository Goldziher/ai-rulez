package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseApprovalSelector(t *testing.T) {
	tests := []struct {
		sel      string
		wantKind string
		wantErr  bool
	}{
		{"remote", "", false},
		{"local", "", false},
		{"all", "", false},
		{"kind:hook", "hook", false},
		{"kind:mcp_server", "mcp_server", false},
		{"kind:installed-skill", "installed-skill", false},
		{"kind:nope", "", true},
		{"hook", "", true},
		{"", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.sel, func(t *testing.T) {
			kind, err := ParseApprovalSelector(tt.sel)
			assert.Equal(t, tt.wantErr, err != nil)
			assert.Equal(t, tt.wantKind, kind)
		})
	}
}

func TestParseApprovalMaxAge(t *testing.T) {
	tests := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{"365d", 365 * 24 * time.Hour, false},
		{"720h", 720 * time.Hour, false},
		{"0d", 0, true},
		{"-1d", 0, true},
		{"soon", 0, true},
		{"", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseApprovalMaxAge(tt.in)
			assert.Equal(t, tt.wantErr, err != nil)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestValidateGovernance(t *testing.T) {
	tests := []struct {
		name    string
		gov     *GovernanceConfig
		wantErr string
	}{
		{"absent", nil, ""},
		{"valid", &GovernanceConfig{RequireApproval: []string{"remote", "kind:hook"}, MinApprovers: 2, MaxAge: "90d"}, ""},
		{"bad selector", &GovernanceConfig{RequireApproval: []string{"remotes"}}, "invalid selector"},
		{"negative approvers", &GovernanceConfig{MinApprovers: -1}, "min_approvers"},
		{"bad max_age", &GovernanceConfig{MaxAge: "forever"}, "max_age"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := (&Config{Governance: tt.gov}).validateGovernance()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestMergeConfigDocs_LocalOverlayCannotRelaxGovernance(t *testing.T) {
	shared := map[string]any{"name": "p", "governance": map[string]any{"require_approval": []any{"remote"}, "enforce": true}}
	local := map[string]any{"governance": map[string]any{"enforce": false, "require_approval": []any{}}}

	merged, warnings, err := MergeConfigDocs(shared, local)

	require.NoError(t, err)
	assert.Equal(t, shared["governance"], merged["governance"])
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "[governance]")
}
