package contentlock

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

func TestSubjectOfCommitsToTheApprovalSet(t *testing.T) {
	// Arrange
	plain := &lockfile.File{Version: 1, Item: []lockfile.Item{{Kind: "rule", ID: "style", Digest: "sha256:aa"}}}
	approved := &lockfile.File{Version: 1, Item: plain.Item, Approval: []lockfile.Approval{
		{Kind: "rule", ID: "style", Digest: "sha256:aa", Reviewer: "alice", Assurance: lockfile.AssuranceAsserted, ApprovedAt: "2026-10-05T00:00:00Z"},
	}}

	// Act
	without, with := SubjectOf(plain), SubjectOf(approved)

	// Assert
	assert.Empty(t, without.ApprovalsDigest, "a lock without approvals keeps the subject it always had")
	assert.Equal(t, approved.ApprovalsDigest(), with.ApprovalsDigest)
	assert.NotEmpty(t, with.ApprovalsDigest)
	assert.Equal(t, without.Tree, with.Tree, "approvals sit outside the tree digest")
	assert.NotEqual(t, without.Digest(), with.Digest(), "but inside the signed subject")
}
