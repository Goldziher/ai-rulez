package policy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/approval"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestParseGovernance(t *testing.T) {
	// Arrange
	body := "policy_version = 1\n[governance]\nenforce = true\nrequire_approval = [\"remote\", \"kind:hook\", \"remote\"]\n" +
		"min_approvers = 2\napprovers = [\" Alice@Example.org \", \"bob@example.org\"]\n"

	// Act
	_, p, err := Parse("p.toml", []byte(body))

	// Assert
	require.NoError(t, err)
	assert.True(t, p.Governance.Enforce)
	assert.Equal(t, []string{"kind:hook", "remote"}, p.Governance.RequireApproval)
	assert.Equal(t, 2, p.Governance.MinApprovers)
	assert.Equal(t, List{Set: true, Items: []string{"alice@example.org", "bob@example.org"}}, p.Governance.Approvers)
}

func TestParseGovernanceRejects(t *testing.T) {
	tests := []struct{ name, body, want string }{
		{"bad selector", "policy_version = 1\n[governance]\nrequire_approval = [\"everything\"]\n", "invalid selector"},
		{"negative min", "policy_version = 1\n[governance]\nmin_approvers = -1\n", "must not be negative"},
		{"unknown key", "policy_version = 1\n[governance]\nexempt = [\"rule:*\"]\n", "unknown key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := Parse("p.toml", []byte(tt.body))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "AR743")
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestMergeGovernance(t *testing.T) {
	a := Policy{Governance: Governance{Enforce: true, RequireApproval: []string{"remote"}, MinApprovers: 1, Approvers: List{Set: true, Items: []string{"a", "b"}}}}
	b := Policy{Governance: Governance{RequireApproval: []string{"kind:hook"}, MinApprovers: 3, Approvers: List{Set: true, Items: []string{"b", "c"}}}}

	got := Merge(a, b)

	assert.Equal(t, Governance{Enforce: true, RequireApproval: []string{"kind:hook", "remote"}, MinApprovers: 3, Approvers: List{Set: true, Items: []string{"b"}}}, got.Governance)
	assert.Equal(t, got, Merge(b, a))
	assert.Equal(t, a.Governance.Approvers, Merge(a, Policy{}).Governance.Approvers, "an unset list constrains nothing")
}

func TestApplyGovernance(t *testing.T) {
	pol := Policy{Governance: Governance{
		Enforce: true, RequireApproval: []string{"remote"}, MinApprovers: 2,
		Approvers: List{Set: true, Items: []string{"alice@example.org", "bob@example.org"}},
	}}
	tests := []struct {
		name      string
		repo      *config.GovernanceConfig
		wantKeys  []string
		approvers []string
		minAppr   int
		enforce   bool
	}{
		{"repo without [governance] is forced silently", nil, nil, []string{"alice@example.org", "bob@example.org"}, 2, true},
		{
			"repo loosening everything is clamped and reported",
			&config.GovernanceConfig{MinApprovers: 1, Approvers: []string{"mallory@example.org", "Alice@Example.org"}},
			[]string{"AR740 governance.approvers", "AR740 governance.enforce", "AR740 governance.min_approvers"},
			[]string{"alice@example.org"}, 2, true,
		},
		{
			"repo that only raises is accepted",
			&config.GovernanceConfig{Enforce: true, MinApprovers: 3, Approvers: []string{"bob@example.org"}},
			nil, []string{"bob@example.org"}, 3, true,
		},
		{
			"repo approvers all outside the policy list fall back to the policy list",
			&config.GovernanceConfig{Enforce: true, Approvers: []string{"mallory@example.org"}},
			[]string{"AR740 governance.approvers"}, []string{"alice@example.org", "bob@example.org"}, 2, true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := testConfig(t, "[governance]\nenforce = false\n")
			cfg.Governance = tt.repo

			// Act
			res := Resolve([]Layer{layer("managed", pol)}).Apply(cfg)

			// Assert
			assert.ElementsMatch(t, tt.wantKeys, codes(res.Outcome))
			require.NotNil(t, cfg.Governance)
			assert.Equal(t, tt.enforce, cfg.Governance.Enforce)
			assert.Equal(t, tt.minAppr, cfg.Governance.MinApprovers)
			assert.Equal(t, tt.approvers, cfg.Governance.Approvers)
			assert.Equal(t, []string{"remote"}, cfg.Governance.PolicyFloor)
		})
	}
}

func TestApplyGovernance_ExemptCannotWidenThePolicyFloor(t *testing.T) {
	// Arrange: the policy requires approval of remote content; the repository exempts it
	cfg := testConfig(t, "")
	cfg.Governance = &config.GovernanceConfig{Exempt: []string{"include:*"}}
	Resolve([]Layer{layer("managed", Policy{Governance: Governance{RequireApproval: []string{"remote"}}})}).Apply(cfg)
	pol := approval.PolicyOf(cfg)
	include := approval.Subject{Kind: approval.KindInclude, ID: "shared", Class: approval.ClassRemote}
	hook := approval.Subject{Kind: "hook", ID: "Stop:*:0", Class: approval.ClassLocal}

	// Act and Assert
	assert.True(t, pol.Active())
	assert.True(t, pol.Requires(include), "the exempt glob does not reach the policy's selectors")
	assert.False(t, pol.Requires(hook))
}

func TestApplyGovernance_RepoSelectorsStayExemptable(t *testing.T) {
	cfg := testConfig(t, "")
	cfg.Governance = &config.GovernanceConfig{RequireApproval: []string{"local"}, Exempt: []string{"rule:*"}}
	Resolve([]Layer{layer("managed", Policy{Governance: Governance{RequireApproval: []string{"remote"}}})}).Apply(cfg)
	pol := approval.PolicyOf(cfg)

	assert.False(t, pol.Requires(approval.Subject{Kind: "rule", ID: "x", Class: approval.ClassLocal}), "the repository's own selection can still be narrowed")
	assert.True(t, pol.Requires(approval.Subject{Kind: "skill", ID: "x", Class: approval.ClassLocal}))
}

// TestApplyGovernance_RepoTeamsCannotWidenAPinnedTeam is RV-GOV-2: the
// repository adds itself to a team the policy's approvers list pins.
func TestApplyGovernance_RepoTeamsCannotWidenAPinnedTeam(t *testing.T) {
	include := approval.Subject{Kind: approval.KindInclude, ID: "shared", Class: approval.ClassRemote}
	tests := []struct {
		name         string
		teams        map[string][]string
		resolved     map[string][]string
		wantCodes    []string
		wantTeams    map[string][]string
		authorized   []string
		unauthorized []string
	}{
		{
			name:         "a pinned team loses the repository's members",
			teams:        map[string][]string{"@Acme/Security": {"mallory"}, "@acme/web": {"bob"}},
			wantCodes:    []string{"AR740 governance.approvers"},
			wantTeams:    map[string][]string{"@acme/web": {"bob"}},
			unauthorized: []string{"github:mallory", "mallory"},
		},
		{
			name:         "forge-resolved members count, the repository's never",
			teams:        map[string][]string{"@acme/security": {"mallory"}},
			resolved:     map[string][]string{"@acme/security": {"github:alice"}},
			wantCodes:    []string{"AR740 governance.approvers"},
			wantTeams:    map[string][]string{},
			authorized:   []string{"github:alice"},
			unauthorized: []string{"github:mallory"},
		},
		{
			name:      "teams the policy does not pin are left alone",
			teams:     map[string][]string{"@acme/web": {"bob"}},
			wantTeams: map[string][]string{"@acme/web": {"bob"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := testConfig(t, "")
			cfg.Governance = &config.GovernanceConfig{Teams: tt.teams}
			pol := Policy{Governance: Governance{RequireApproval: []string{"remote"}, Approvers: List{Set: true, Items: []string{"@acme/security"}}}}

			// Act
			res := Resolve([]Layer{layer("managed", pol)}).Apply(cfg)

			// Assert
			assert.Equal(t, tt.wantCodes, codes(res.Outcome))
			assert.Equal(t, tt.wantTeams, cfg.Governance.Teams)
			p := approval.PolicyOf(cfg)
			if tt.resolved != nil {
				p = p.WithResolvedTeams(tt.resolved)
			}
			for _, who := range tt.authorized {
				assert.True(t, p.Authorized(who), who)
			}
			for _, who := range tt.unauthorized {
				assert.False(t, p.Authorized(who), who)
				assert.False(t, p.AuthorizedFor(who, include), who)
				assert.False(t, p.Names(who, include), who)
			}
		})
	}
}

func TestApplyGovernance_NoPolicyKeyLeavesTheRepoAlone(t *testing.T) {
	cfg := testConfig(t, "")
	cfg.Governance = &config.GovernanceConfig{Approvers: []string{"x"}}

	res := Resolve([]Layer{layer("managed", Policy{Lock: Lock{Enforce: true}})}).Apply(cfg)

	assert.Empty(t, res.Outcome.Violations)
	assert.Equal(t, []string{"x"}, cfg.Governance.Approvers)
	assert.Empty(t, cfg.Governance.PolicyFloor)
}

func TestShowPolicyListsGovernance(t *testing.T) {
	p := Policy{Governance: Governance{Enforce: true, RequireApproval: []string{"remote"}, MinApprovers: 2, Approvers: List{Set: true, Items: []string{"a"}}}}
	tree := p.Tree()
	gov, ok := tree["governance"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, gov["enforce"])
	assert.Equal(t, 2, gov["min_approvers"])
	assert.Equal(t, []string{"remote"}, gov["require_approval"])
	assert.Equal(t, []string{"a"}, gov["approvers"])
}

func TestParseGovernanceAssuranceFloors(t *testing.T) {
	// Arrange
	body := "policy_version = 1\n[governance]\nmin_assurance = \"Review-Linked\"\nforbid_self_approval = true\napprovers_from = \"CODEOWNERS\"\n"
	// Act
	_, p, err := Parse("p.toml", []byte(body))
	// Assert
	require.NoError(t, err)
	assert.Equal(t, "review-linked", p.Governance.MinAssurance)
	assert.True(t, p.Governance.ForbidSelfApproval)
	assert.Equal(t, "CODEOWNERS", p.Governance.ApproversFrom)
	for _, bad := range []string{
		"min_assurance = \"strong\"",
		"approvers_from = \"docs/OWNERS\"",
	} {
		_, _, err := Parse("p.toml", []byte("policy_version = 1\n[governance]\n"+bad+"\n"))
		require.Error(t, err, bad)
	}
}

func TestMergeGovernanceAssuranceFloors(t *testing.T) {
	a := Policy{Governance: Governance{MinAssurance: "review-linked", ApproversFrom: "CODEOWNERS"}}
	b := Policy{Governance: Governance{MinAssurance: "signed", ForbidSelfApproval: true}}
	got := Merge(a, b).Governance
	assert.Equal(t, "signed", got.MinAssurance, "the stronger level")
	assert.True(t, got.ForbidSelfApproval)
	assert.Equal(t, "CODEOWNERS", got.ApproversFrom)
	assert.Equal(t, got, Merge(b, a).Governance)
}

func TestApplyGovernanceAssuranceFloors(t *testing.T) {
	pol := Governance{MinAssurance: "review-linked", ForbidSelfApproval: true, ApproversFrom: "CODEOWNERS"}
	tests := []struct {
		name     string
		repo     *config.GovernanceConfig
		want     config.GovernanceConfig
		wantViol []string
	}{
		{"a repository without [governance] gets the floor silently", nil,
			config.GovernanceConfig{MinAssurance: "review-linked", ForbidSelfApproval: true, ApproversFrom: "CODEOWNERS"}, nil},
		{"weaker explicit values are raised and reported",
			&config.GovernanceConfig{Enforce: true, MinAssurance: "asserted"},
			config.GovernanceConfig{Enforce: true, MinAssurance: "review-linked", ForbidSelfApproval: true, ApproversFrom: "CODEOWNERS"},
			[]string{"AR740 governance.approvers_from", "AR740 governance.forbid_self_approval", "AR740 governance.min_assurance"}},
		{"stricter values and another CODEOWNERS path are accepted",
			&config.GovernanceConfig{Enforce: true, MinAssurance: "signed", ForbidSelfApproval: true, ApproversFrom: ".github/OWNERS"},
			config.GovernanceConfig{Enforce: true, MinAssurance: "signed", ForbidSelfApproval: true, ApproversFrom: ".github/OWNERS"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := testConfig(t, "name = \"x\"\n")
			cfg.Governance = tt.repo
			res := Resolve([]Layer{layer("managed", Policy{Governance: pol})})
			// Act
			out := res.Apply(cfg).Outcome
			// Assert
			require.NotNil(t, cfg.Governance)
			got := *cfg.Governance
			got.PolicyFloor = nil
			assert.Equal(t, tt.want, got)
			assert.ElementsMatch(t, tt.wantViol, codes(out))
		})
	}
}
