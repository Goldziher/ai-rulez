package signing_test

import (
	"context"
	. "github.com/Goldziher/ai-rulez/v5/internal/signing"
	. "github.com/Goldziher/ai-rulez/v5/internal/signing/sigstore"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// rollbackFixture signs a lock with one key at several times and verifies it
// against a shared on-disk rollback state.
type rollbackFixture struct {
	t      *testing.T
	lock   *lockfile.File
	signer Signer
	trust  TrustSet
	state  *State
}

func newRollbackFixture(t *testing.T) *rollbackFixture {
	t.Helper()
	setUserDirs(t)
	priv, pub, err := GenerateKeyPair(nil)
	require.NoError(t, err)
	signer, err := LoadKeySigner(priv, nil)
	require.NoError(t, err)
	statePath, secretPath := StatePaths(nil)
	st, err := OpenState(statePath, secretPath)
	require.NoError(t, err)
	return &rollbackFixture{t: t, lock: testLock(t, nil), signer: signer, trust: keyTrust(t, pub), state: st}
}

func (f *rollbackFixture) bundle(repo string, at time.Time) []byte {
	f.t.Helper()
	data, err := SignLock(context.Background(), f.signer, f.lock, LockMeta{Repository: repo, Now: at, Version: "5"})
	require.NoError(f.t, err)
	return data
}

func (f *rollbackFixture) policy() LockPolicy {
	return LockPolicy{Verifier: Verifier{Keys: f.trust.Keys(SubjectLock), TLog: TLogOff}, Trust: f.trust, State: f.state, Now: testNow}
}

func TestRollbackStateIsNotSharedAcrossProjectsWithoutARepositoryClaim(t *testing.T) {
	f := newRollbackFixture(t)
	newer, older := f.bundle("", testNow), f.bundle("", testNow.Add(-24*time.Hour))
	p := f.policy()
	p.ScopeAbs = "/work/b"
	rep, err := VerifyLock(newer, f.lock, p)
	require.NoError(t, err)
	require.NoError(t, rep.Commit(f.state))

	p.ScopeAbs = "/work/a"
	_, errOther := VerifyLock(older, f.lock, p)
	p.ScopeAbs = "/work/b"
	_, errSame := VerifyLock(older, f.lock, p)

	assert.NoError(t, errOther, "project A has its own mark")
	assert.Equal(t, CodeRollback, CodeOf(errSame))
}

func TestRollbackStateIsKeyedBySignerAndRepository(t *testing.T) {
	f := newRollbackFixture(t)
	p := f.policy()
	p.ScopeRel, p.ScopeAbs = ".ai-rulez", "/work/repo/.ai-rulez"
	rep, err := VerifyLock(f.bundle("https://example.com/org/repo", testNow), f.lock, p)
	require.NoError(t, err)
	require.NoError(t, rep.Commit(f.state))

	p.ScopeAbs = "/work/other/.ai-rulez"
	_, other := VerifyLock(f.bundle("https://example.com/org/other", testNow.Add(-time.Hour)), f.lock, p)
	p.ScopeAbs = "/work/clone/.ai-rulez"
	_, clone := VerifyLock(f.bundle("https://example.com/org/repo", testNow.Add(-time.Hour)), f.lock, p)

	assert.NoError(t, other, "a signer-claimed repository name cannot reach another repository's mark")
	assert.Equal(t, CodeRollback, CodeOf(clone), "two clones of one repository share its mark")
}

// TestRollbackMarkDoesNotDependOnTheRepositoryClaim is RV-GOV-6: in one
// checkout, an older attestation is a rollback whatever repository it claims.
func TestRollbackMarkDoesNotDependOnTheRepositoryClaim(t *testing.T) {
	const repo = "https://github.com/o/r"
	tests := []struct {
		name         string
		newer, older string
	}{
		{"older without a claim after newer with one", repo, ""},
		{"older with a claim after newer without one", "", repo},
		{"older with another claim", repo, "https://github.com/o/fork"},
		{"same claim on both", repo, repo},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			f := newRollbackFixture(t)
			p := f.policy()
			p.ScopeRel, p.ScopeAbs = "proj", "/w/proj"
			rep, err := VerifyLock(f.bundle(tt.newer, testNow.Add(-time.Hour)), f.lock, p)
			require.NoError(t, err)
			require.NoError(t, rep.Commit(f.state))

			// Act
			_, err = VerifyLock(f.bundle(tt.older, testNow.Add(-48*time.Hour)), f.lock, p)

			// Assert
			require.Error(t, err)
			assert.Equal(t, CodeRollback, CodeOf(err))
		})
	}
}

func TestRollbackStateIsKeyedByTheCheckoutScopeInsideARepository(t *testing.T) {
	f := newRollbackFixture(t)
	const repo = "https://example.com/org/mono"
	newer, older := f.bundle(repo, testNow), f.bundle(repo, testNow.Add(-time.Hour))
	p := f.policy()
	p.ScopeRel, p.ScopeAbs = "services/a/.ai-rulez", "/w/mono/services/a/.ai-rulez"
	rep, err := VerifyLock(newer, f.lock, p)
	require.NoError(t, err)
	require.NoError(t, rep.Commit(f.state))

	p.ScopeRel, p.ScopeAbs = "services/b/.ai-rulez", "/w/mono/services/b/.ai-rulez"
	_, otherRoot := VerifyLock(older, f.lock, p)
	p.ScopeRel, p.ScopeAbs = "services/a/.ai-rulez", "/w/mono/services/a/.ai-rulez"
	_, sameRoot := VerifyLock(older, f.lock, p)

	assert.NoError(t, otherRoot)
	assert.Equal(t, CodeRollback, CodeOf(sameRoot))
}

func TestFutureIssuedAtIsRejected(t *testing.T) {
	tests := []struct {
		name     string
		at       time.Time
		maxAge   time.Duration
		wantCode string
	}{
		{"a day ahead without max_age", testNow.Add(24 * time.Hour), 0, CodeStale},
		{"a day ahead with max_age", testNow.Add(24 * time.Hour), 30 * 24 * time.Hour, CodeStale},
		{"within the clock skew", testNow.Add(time.Minute), 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newRollbackFixture(t)
			p := f.policy()
			p.MaxAge = tt.maxAge

			_, err := VerifyLock(f.bundle("", tt.at), f.lock, p)

			if tt.wantCode == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Equal(t, tt.wantCode, CodeOf(err))
			assert.Contains(t, err.Error(), "future")
		})
	}
}

func TestCheckFreshRejectsASigningTimeInTheFuture(t *testing.T) {
	res := &Result{SignedAt: testNow.Add(48 * time.Hour)}

	err := CheckFresh(res, time.Time{}, 24*time.Hour, testNow)

	require.Error(t, err)
	assert.Equal(t, CodeStale, CodeOf(err))
}

func TestRollbackMessageSaysHowToReset(t *testing.T) {
	f := newRollbackFixture(t)
	p := f.policy()
	rep, err := VerifyLock(f.bundle("", testNow), f.lock, p)
	require.NoError(t, err)
	require.NoError(t, rep.Commit(f.state))

	_, err = VerifyLock(f.bundle("", testNow.Add(-time.Hour)), f.lock, p)

	require.Error(t, err)
	statePath, _ := StatePaths(nil)
	assert.Contains(t, err.Error(), statePath)
	assert.Contains(t, err.Error(), "--no-state")
}

func TestDigestRollbackRetiresReplacedBodies(t *testing.T) {
	// Arrange
	f := newRollbackFixture(t)
	require.NoError(t, f.state.AdvanceDigest("k", "sha256:a"))
	require.NoError(t, f.state.AdvanceDigest("k", "sha256:b"))
	statePath, secretPath := StatePaths(nil)
	reopened, err := OpenState(statePath, secretPath)
	require.NoError(t, err)
	// Act and Assert
	require.Error(t, reopened.CheckDigest("k", "sha256:a"), "a replaced body is a rollback, also after a restart")
	assert.ErrorContains(t, reopened.CheckDigest("k", "sha256:a"), "AR727")
	assert.NoError(t, reopened.CheckDigest("k", "sha256:b"), "the current body is fine")
	assert.NoError(t, reopened.CheckDigest("k", "sha256:c"), "a body never seen is new")
	assert.NoError(t, reopened.CheckDigest("other", "sha256:a"), "keys are independent")
}
