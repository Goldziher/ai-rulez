package signing

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// TestBlobLockRollbackIsDetectedByDigest is RV-GOV-5: a key-signed blob bundle
// (cosign sign-blob) carries no signing time, so the lock bodies already
// replaced on this machine are its rollback record.
func TestBlobLockRollbackIsDetectedByDigest(t *testing.T) {
	// Arrange
	setUserDirs(t)
	statePath, secretPath := StatePaths(nil)
	st, err := OpenState(statePath, secretPath)
	require.NoError(t, err)
	priv, pub, err := GenerateKeyPair(nil)
	require.NoError(t, err)
	trust := keyTrust(t, pub)
	oldLock := testLock(t, nil)
	newLock := testLock(t, func(f *lockfile.File) { f.Item[0].Digest = "sha256:" + hexRepeat("ee") })
	oldB, newB := blobBundle(t, priv, oldLock), blobBundle(t, priv, newLock)
	p := LockPolicy{Verifier: Verifier{Keys: trust.Keys(SubjectLock), TLog: TLogOff}, Trust: trust, State: st, ScopeAbs: "/p", Now: testNow}
	first, err := VerifyLock(oldB, oldLock, p)
	require.NoError(t, err)
	require.NoError(t, first.Commit(st))
	second, err := VerifyLock(newB, newLock, p)
	require.NoError(t, err)
	require.NoError(t, second.Commit(st))

	// Act
	_, rollback := VerifyLock(oldB, oldLock, p)
	again, current := VerifyLock(newB, newLock, p)
	p.ScopeAbs = "/elsewhere"
	_, otherProject := VerifyLock(oldB, oldLock, p)

	// Assert
	assert.Equal(t, CodeRollback, CodeOf(rollback), "%v", rollback)
	require.NoError(t, current, "the current lock still verifies")
	assert.NotEmpty(t, again.SubjectDigest)
	assert.NoError(t, otherProject, "another project has its own record")
}
