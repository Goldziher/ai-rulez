package lockfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func approvalFixture() []Approval {
	return []Approval{
		{Kind: "skill", ID: "deploy", Domain: "backend", Digest: "sha256:02", Reviewer: "bob", Assurance: AssuranceAsserted, ApprovedAt: "2026-10-05T12:00:00Z"},
		{Kind: "include", ID: "shared", Digest: "sha256:01", Reviewer: "alice", Assurance: AssuranceAsserted, ApprovedAt: "2026-10-04T08:30:00Z",
			Expires: "2027-01-05", Note: "read scripts/", AcceptedFindings: []string{"AR005"}},
		{Kind: "include", ID: "shared", Digest: "sha256:01", Reviewer: "aaron", Assurance: AssuranceAsserted, ApprovedAt: "2026-10-04T09:00:00Z"},
	}
}

func TestSave_ApprovalsAreSortedAndByteStable(t *testing.T) {
	// Arrange: the same approvals in two input orders.
	dir := t.TempDir()
	forward := &File{Approval: approvalFixture()}
	reversed := &File{Approval: []Approval{approvalFixture()[2], approvalFixture()[1], approvalFixture()[0]}}

	// Act
	require.NoError(t, Save(dir, forward))
	first, err := os.ReadFile(filepath.Join(dir, FileName))
	require.NoError(t, err)
	require.NoError(t, Save(dir, reversed))
	second, err := os.ReadFile(filepath.Join(dir, FileName))
	require.NoError(t, err)

	// Assert
	assert.Equal(t, string(first), string(second), "input order must not change the bytes")
	loaded, err := Load(dir)
	require.NoError(t, err)
	require.Len(t, loaded.Approval, 3)
	assert.Equal(t, []string{"aaron", "alice", "bob"}, []string{loaded.Approval[0].Reviewer, loaded.Approval[1].Reviewer, loaded.Approval[2].Reviewer})
	assert.Equal(t, "2026-10-04T08:30:00Z", loaded.Approval[1].ApprovedAt, "the timestamp is stored once and kept verbatim")
	assert.Equal(t, []string{"AR005"}, loaded.Approval[1].AcceptedFindings)
}

func TestSave_ApprovalsSurviveARewrite(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Save(dir, &File{Approval: approvalFixture()}))
	before, err := os.ReadFile(filepath.Join(dir, FileName))
	require.NoError(t, err)

	loaded, err := Load(dir)
	require.NoError(t, err)
	require.NoError(t, Save(dir, loaded))

	after, err := os.ReadFile(filepath.Join(dir, FileName))
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after))
}

func TestSave_ApprovalsDoNotChangeThePins(t *testing.T) {
	dir := t.TempDir()
	base := &File{Tree: "sha256:tree", Item: []Item{{Kind: "rule", ID: "style", Digest: "sha256:01"}}}
	require.NoError(t, Save(dir, base))
	plain, err := Load(dir)
	require.NoError(t, err)

	withApprovals := *base
	withApprovals.Approval = approvalFixture()
	require.NoError(t, Save(dir, &withApprovals))
	got, err := Load(dir)
	require.NoError(t, err)

	assert.Equal(t, plain.Tree, got.Tree)
	assert.Equal(t, plain.Item, got.Item)
	assert.False(t, got.HasContentPins() != plain.HasContentPins())
}

func TestSetApproval_ReplacesTheSameReviewerAndDigest(t *testing.T) {
	f := &File{Approval: approvalFixture()}
	f.SetApproval(Approval{Kind: "include", ID: "shared", Digest: "sha256:01", Reviewer: "alice", Note: "second look"})
	assert.Len(t, f.Approval, 3)
	f.SetApproval(Approval{Kind: "include", ID: "shared", Digest: "sha256:03", Reviewer: "alice"})
	assert.Len(t, f.Approval, 4, "another digest is another record")
	var notes []string
	for _, a := range f.Approval {
		if a.Reviewer == "alice" && a.Digest == "sha256:01" {
			notes = append(notes, a.Note)
		}
	}
	assert.Equal(t, []string{"second look"}, notes)
}
