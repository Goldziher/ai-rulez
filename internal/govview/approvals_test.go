package govview

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/approval"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

var approvalTestNow = time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)

func approvalLock() *lockfile.File {
	return &lockfile.File{
		Include: []lockfile.Entry{{Name: "shared", Digest: "sha256:new"}},
		Approval: []lockfile.Approval{
			{Kind: "include", ID: "shared", Digest: "sha256:old", Reviewer: "alice", Assurance: lockfile.AssuranceAsserted, ApprovedAt: "2026-10-01T00:00:00Z"},
			{Kind: "rule", ID: "style", Digest: "sha256:style", Reviewer: "bob", Assurance: lockfile.AssuranceAsserted, ApprovedAt: "2026-10-01T00:00:00Z"},
		},
	}
}

func TestApprovalChanges(t *testing.T) {
	items := []lockfile.Item{{Kind: "rule", ID: "style", Digest: "sha256:style"}, {Kind: "hook", ID: "Stop:*:0", Digest: "sha256:hook"}}
	tests := []struct {
		name       string
		gov        *config.GovernanceConfig
		wantRefs   []string
		wantChange []string
		wantNote   bool
	}{
		{"no governance", nil, nil, nil, false},
		{"not enforced is only a note", &config.GovernanceConfig{RequireApproval: []string{"remote"}}, nil, nil, true},
		{"enforced remote: stale include", &config.GovernanceConfig{RequireApproval: []string{"remote"}, Enforce: true},
			[]string{"include:shared"}, []string{contentlock.Changed}, false},
		{"enforced all: stale include and missing hook", &config.GovernanceConfig{RequireApproval: []string{"all"}, Enforce: true},
			[]string{"hook:Stop:*:0", "include:shared"}, []string{contentlock.Added, contentlock.Changed}, false},
		{"exempt", &config.GovernanceConfig{RequireApproval: []string{"all"}, Exempt: []string{"include:*", "hook:*"}, Enforce: true}, nil, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := &config.Config{Governance: tt.gov}

			// Act
			changes, notes := ApprovalChanges(cfg, approvalLock(), items, approvalTestNow)

			// Assert
			var refs, kinds []string
			for _, c := range changes {
				assert.Equal(t, contentlock.ScopeApproval, c.Scope)
				refs = append(refs, approval.Subject{Kind: c.Kind, Domain: c.Domain, ID: c.ID}.Ref())
				kinds = append(kinds, c.Change)
			}
			assert.Equal(t, tt.wantRefs, refs)
			assert.Equal(t, tt.wantChange, kinds)
			assert.Equal(t, tt.wantNote, len(notes) > 0)
		})
	}
}

func TestApprovalChanges_StaleCarriesBothDigests(t *testing.T) {
	cfg := &config.Config{Governance: &config.GovernanceConfig{RequireApproval: []string{"remote"}, Enforce: true}}
	changes, _ := ApprovalChanges(cfg, approvalLock(), nil, approvalTestNow)
	require.Len(t, changes, 1)
	assert.Equal(t, "sha256:old", changes[0].Old)
	assert.Equal(t, "sha256:new", changes[0].New)
	assert.Contains(t, changes[0].Detail, "AR711")
	assert.Contains(t, changes[0].Line(), "approval:")
}

func TestApprovalIndex_ForItem(t *testing.T) {
	idx := &approvalIndex{
		policy: approval.PolicyOf(&config.Config{Governance: &config.GovernanceConfig{RequireApproval: []string{"kind:rule"}}}),
		recs:   approvalLock().Approval, now: approvalTestNow,
	}

	approved := idx.forItem("rule", "", "style", "sha256:style")
	require.NotNil(t, approved)
	assert.Equal(t, approval.StatusOK, approved.Status)
	assert.Equal(t, []string{"bob"}, approved.Reviewers)
	assert.Equal(t, lockfile.AssuranceAsserted, approved.Assurance)

	stale := idx.forItem("rule", "", "style", "sha256:other")
	require.NotNil(t, stale)
	assert.Equal(t, approval.StatusStale, stale.Status)
	assert.Equal(t, []string{}, stale.Reviewers)

	assert.Nil(t, idx.forItem("agent", "", "x", "sha256:x"), "not required and never approved: null")
	assert.Nil(t, idx.forItem("rule", "", "style", ""), "no digest, no state")
}
