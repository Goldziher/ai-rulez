package signing_test

import (
	"context"
	. "github.com/Goldziher/ai-rulez/v5/internal/signing"
	. "github.com/Goldziher/ai-rulez/v5/internal/signing/sigstore"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

func approvalBundle(t *testing.T, sub ApprovalSubject, pred ApprovalPredicate) (bundle, pubPEM []byte) {
	t.Helper()
	priv, pub, err := GenerateKeyPair(nil)
	require.NoError(t, err)
	signer, err := LoadKeySigner(priv, nil)
	require.NoError(t, err)
	st, err := ApprovalStatement(sub, pred)
	require.NoError(t, err)
	data, err := SignStatement(context.Background(), signer, st)
	require.NoError(t, err)
	return data, pub
}

func approvalCheck(t *testing.T, pubPEM []byte, subject string) *ApprovalCheck {
	t.Helper()
	pub, err := ParsePublicKey(pubPEM)
	require.NoError(t, err)
	trust := TrustSet{Entries: []TrustEntry{{Subject: subject, Key: pub}}}
	return &ApprovalCheck{Trust: trust, Verifier: Verifier{Keys: trust.Keys(SubjectApproval), TLog: TLogOff}}
}

func TestApprovalCheck_Verify(t *testing.T) {
	digest := "sha256:" + hexRepeat("ab")
	sub := ApprovalSubject{Kind: "skill", Domain: "backend", ID: "deploy", Digest: digest}
	pred := ApprovalPredicate{ApprovedAt: "2026-10-06T12:00:00Z", Expires: "2027-01-01", AcceptedFindings: []string{"AR005"}}
	bundle, pub := approvalBundle(t, sub, pred)

	tests := []struct {
		name     string
		subject  string
		sub      ApprovalSubject
		wantCode string
	}{
		{"the signed subject", SubjectApproval, sub, ""},
		{"another digest", SubjectApproval, ApprovalSubject{Kind: "skill", Domain: "backend", ID: "deploy", Digest: "sha256:" + hexRepeat("cd")}, CodeSubjectMismatch},
		{"another item with the same digest", SubjectApproval, ApprovalSubject{Kind: "skill", Domain: "backend", ID: "other", Digest: digest}, CodeSubjectMismatch},
		{"signer trusted only for the lock", SubjectLock, sub, CodeSignerNotTrusted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			check := approvalCheck(t, pub, tt.subject)

			// Act
			res, err := check.Verify(bundle, tt.sub, testNow)

			// Assert
			if tt.wantCode != "" {
				require.Error(t, err)
				assert.Equal(t, tt.wantCode, CodeOf(err))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, KindKey, res.Signer.Kind)
			assert.Equal(t, "key:"+res.Signer.KeyID, res.Reviewer)
			assert.Equal(t, []string{"AR005"}, res.Predicate.AcceptedFindings)
			assert.Equal(t, "2027-01-01", res.Predicate.Expires)
		})
	}
}

func TestApprovalCheck_RejectsALockAttestation(t *testing.T) {
	// Arrange: a validly signed lock statement is not an approval
	lock := testLock(t, nil)
	bundle, pub := signKeyed(t, lock, LockMeta{})
	check := approvalCheck(t, pub, SubjectApproval)

	// Act
	_, err := check.Verify(bundle, ApprovalSubject{Kind: "rule", ID: "style", Digest: "sha256:" + hexRepeat("ab")}, testNow)

	// Assert
	require.Error(t, err)
	assert.Equal(t, CodeSubjectMismatch, CodeOf(err))
}

func TestVerifyLock_ApprovalsAreInsideTheSignedSubject(t *testing.T) {
	// Arrange: sign a lock, then add an approval to it
	lock := testLock(t, nil)
	data, pub := signKeyed(t, lock, LockMeta{})
	trust := keyTrust(t, pub)
	policy := LockPolicy{Verifier: Verifier{Keys: trust.Keys(SubjectLock), TLog: TLogOff}, Trust: trust, Now: testNow}
	approved := testLock(t, nil)
	approved.Approval = []lockfile.Approval{{Kind: "rule", ID: "style", Digest: approved.Item[0].Digest, Reviewer: "alice", Assurance: lockfile.AssuranceAsserted, ApprovedAt: "2026-10-05T00:00:00Z"}}

	// Act
	_, errAdded := VerifyLock(data, approved, policy)
	resigned, pub2 := signKeyed(t, approved, LockMeta{})
	trust2 := keyTrust(t, pub2)
	policy2 := LockPolicy{Verifier: Verifier{Keys: trust2.Keys(SubjectLock), TLog: TLogOff}, Trust: trust2, Now: testNow}
	rep, errResigned := VerifyLock(resigned, approved, policy2)
	tampered := testLock(t, nil)
	tampered.Approval = []lockfile.Approval{approved.Approval[0]}
	tampered.Approval[0].Reviewer = "mallory"
	_, errTampered := VerifyLock(resigned, tampered, policy2)

	// Assert
	require.Error(t, errAdded, "an approval added after signing changes the subject")
	assert.Equal(t, CodeSubjectMismatch, CodeOf(errAdded))
	require.NoError(t, errResigned)
	assert.Equal(t, approved.ApprovalsDigest(), rep.Predicate.ApprovalsDigest)
	assert.NotEmpty(t, rep.Predicate.ApprovalsDigest)
	require.Error(t, errTampered, "editing a signed approval breaks the signature")
	assert.Equal(t, CodeSubjectMismatch, CodeOf(errTampered))
}

func TestApprovalStatement_RejectsABadDigest(t *testing.T) {
	_, err := ApprovalStatement(ApprovalSubject{Kind: "rule", ID: "x", Digest: "md5:zz"}, ApprovalPredicate{})
	require.Error(t, err)
}
