package lockfile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApprovalsDigest(t *testing.T) {
	base := func() *File {
		return &File{Approval: approvalFixture(), Deny: []Deny{{Digest: "sha256:bad", Reason: "malware"}}}
	}
	tests := []struct {
		name   string
		mutate func(*File)
	}{
		{"a reviewer", func(f *File) { f.Approval[0].Reviewer = "mallory" }},
		{"a digest", func(f *File) { f.Approval[0].Digest = "sha256:ff" }},
		{"an expiry", func(f *File) { f.Approval[1].Expires = "2099-01-01" }},
		{"a note", func(f *File) { f.Approval[1].Note = "edited" }},
		{"an accepted finding", func(f *File) { f.Approval[1].AcceptedFindings = nil }},
		{"an assurance", func(f *File) { f.Approval[0].Assurance = AssuranceSigned }},
		{"a ref", func(f *File) { f.Approval[0].Ref = "https://example.org/pull/1" }},
		{"an attestation", func(f *File) { f.Approval[0].Attestation = "sha256:ab" }},
		{"a removed approval", func(f *File) { f.Approval = f.Approval[1:] }},
		{"a deny reason", func(f *File) { f.Deny[0].Reason = "other" }},
		{"a removed deny entry", func(f *File) { f.Deny = nil }},
	}
	want := base().ApprovalsDigest()
	require.NotEmpty(t, want)
	assert.Regexp(t, `^sha256:[0-9a-f]{64}$`, want)
	for _, tt := range tests {
		t.Run("changes with "+tt.name, func(t *testing.T) {
			// Arrange
			f := base()
			tt.mutate(f)

			// Act / Assert
			assert.NotEqual(t, want, f.ApprovalsDigest())
		})
	}

	t.Run("does not depend on record order", func(t *testing.T) {
		f := base()
		f.Approval[0], f.Approval[2] = f.Approval[2], f.Approval[0]
		assert.Equal(t, want, f.ApprovalsDigest())
	})
	t.Run("is empty without approvals and deny entries", func(t *testing.T) {
		assert.Empty(t, (&File{}).ApprovalsDigest())
		assert.Empty(t, (*File)(nil).ApprovalsDigest())
	})
}

func TestSave_DenyIsSortedAndRoundTrips(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	f := &File{Deny: []Deny{{Digest: "sha256:b", Reason: "two"}, {Digest: "sha256:a", Reason: "one"}}}

	// Act
	require.NoError(t, Save(dir, f))
	got, err := Load(dir)
	require.NoError(t, err)

	// Assert
	assert.Equal(t, []Deny{{Digest: "sha256:a", Reason: "one"}, {Digest: "sha256:b", Reason: "two"}}, got.Deny)
	assert.Equal(t, map[string]string{"sha256:a": "one", "sha256:b": "two"}, got.DenySet())
	got.SetDeny(Deny{Digest: "sha256:a", Reason: "changed"})
	assert.Len(t, got.Deny, 2)
	assert.Equal(t, "changed", got.DenySet()["sha256:a"])
}

func TestAssuranceRank(t *testing.T) {
	assert.Less(t, AssuranceRank("bogus"), AssuranceRank(AssuranceAsserted))
	assert.Less(t, AssuranceRank(AssuranceAsserted), AssuranceRank(AssuranceReviewLinked))
	assert.Less(t, AssuranceRank(AssuranceReviewLinked), AssuranceRank(AssuranceSigned))
}
