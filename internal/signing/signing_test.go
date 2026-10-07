package signing

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

func TestKeyedLockRoundTrip(t *testing.T) {
	lock := testLock(t, nil)
	data, pub := signKeyed(t, lock, LockMeta{Repository: "https://github.com/example/ai-config", Ref: "refs/heads/main", EmbedItems: true})
	trust := keyTrust(t, pub)

	rep, err := VerifyLock(data, lock, LockPolicy{
		Verifier: Verifier{Keys: trust.Keys(SubjectLock), TLog: TLogOff}, Trust: trust, MaxAge: 24 * time.Hour, Now: testNow.Add(time.Hour),
	})

	require.NoError(t, err)
	assert.Equal(t, KindKey, rep.Result.Signer.Kind)
	assert.True(t, rep.Result.Weak, "a key bundle without a log has no trustworthy signing time")
	assert.Equal(t, 1, rep.Predicate.HashVersion)
	assert.Equal(t, lock.Tree, rep.Predicate.Tree)
	assert.Len(t, rep.Predicate.Items, 2)
	assert.Equal(t, "https://github.com/example/ai-config", rep.Predicate.Repository)
	assert.Equal(t, time.Hour, rep.Age)
}

func TestVerifyLockFailures(t *testing.T) {
	lock := testLock(t, nil)
	data, pub := signKeyed(t, lock, LockMeta{Repository: "r"})
	_, otherPub := signKeyed(t, lock, LockMeta{})
	changed := testLock(t, func(f *lockfile.File) { f.Item[0].Digest = "sha256:" + hexRepeat("ee") })
	forged := testLock(t, nil)
	forged.Tree = "sha256:" + hexRepeat("00")
	older, _ := signKeyed(t, lock, LockMeta{Repository: "r", Now: testNow.Add(-48 * time.Hour)})
	_ = older

	tests := []struct {
		name     string
		bundle   []byte
		lock     *lockfile.File
		trust    TrustSet
		keys     [][]byte
		tlog     TLogMode
		maxAge   time.Duration
		minHash  int
		now      time.Time
		wantCode string
	}{
		{"valid", data, lock, keyTrust(t, pub), [][]byte{pub}, TLogOff, 0, 0, testNow, ""},
		{"signer not trusted", data, lock, keyTrust(t, otherPub), [][]byte{otherPub}, TLogOff, 0, 0, testNow, CodeSignerNotTrusted},
		{"tlog required but absent", data, lock, keyTrust(t, pub), [][]byte{pub}, TLogRequired, 0, 0, testNow, CodeTLogMissing},
		{"lock changed after signing", data, changed, keyTrust(t, pub), [][]byte{pub}, TLogOff, 0, 0, testNow, CodeSubjectMismatch},
		{"lock tree edited by hand", data, forged, keyTrust(t, pub), [][]byte{pub}, TLogOff, 0, 0, testNow, CodeSubjectMismatch},
		{"stale", data, lock, keyTrust(t, pub), [][]byte{pub}, TLogOff, 24 * time.Hour, 0, testNow.Add(72 * time.Hour), CodeStale},
		{"hash version below floor", data, lock, keyTrust(t, pub), [][]byte{pub}, TLogOff, 0, 2, testNow, CodeSubjectMismatch},
		{"not a bundle", []byte(`{"a":1}`), lock, keyTrust(t, pub), [][]byte{pub}, TLogOff, 0, 0, testNow, CodeInvalid},
		{"oversized", []byte(strings.Repeat(" ", MaxBundleBytes+1)), lock, keyTrust(t, pub), [][]byte{pub}, TLogOff, 0, 0, testNow, CodeInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var keys = tt.trust.Keys(SubjectLock)

			_, err := VerifyLock(tt.bundle, tt.lock, LockPolicy{
				Verifier: Verifier{Keys: keys, TLog: tt.tlog}, Trust: tt.trust, MaxAge: tt.maxAge, MinHashVersion: tt.minHash, Now: tt.now,
			})

			if tt.wantCode == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Equal(t, tt.wantCode, CodeOf(err), err.Error())
		})
	}
}

func TestTamperedPayloadFailsVerification(t *testing.T) {
	lock := testLock(t, nil)
	data, pub := signKeyed(t, lock, LockMeta{})
	var doc map[string]any
	require.NoError(t, json.Unmarshal(data, &doc))
	env := doc["dsseEnvelope"].(map[string]any)
	env["payload"] = "eyJ0YW1wZXJlZCI6dHJ1ZX0=" // {"tampered":true}
	tampered, err := json.Marshal(doc)
	require.NoError(t, err)

	_, err = (&Verifier{Keys: keyTrust(t, pub).Keys(SubjectLock), TLog: TLogOff}).Verify(tampered)

	require.Error(t, err)
	assert.Equal(t, CodeInvalid, CodeOf(err))
}

func TestRollbackAgainstHighWaterMark(t *testing.T) {
	setUserDirs(t)
	lock := testLock(t, nil)
	var newer []byte
	var trust TrustSet
	priv, pub2, err := GenerateKeyPair(nil)
	require.NoError(t, err)
	signer, err := LoadKeySigner(priv, nil)
	require.NoError(t, err)
	// The mark is per signer, so the older bundle comes from the same key as the newer one.
	older, err := SignLock(context.Background(), signer, lock, LockMeta{Repository: "repo", Now: testNow.Add(-24 * time.Hour), Version: "5"})
	require.NoError(t, err)
	newer, err = SignLock(context.Background(), signer, lock, LockMeta{Repository: "repo", Now: testNow, Version: "5"})
	require.NoError(t, err)
	trust = keyTrust(t, pub2)
	statePath, secretPath := StatePaths(nil)
	st, err := OpenState(statePath, secretPath)
	require.NoError(t, err)
	policy := LockPolicy{Verifier: Verifier{Keys: trust.Keys(SubjectLock), TLog: TLogOff}, Trust: trust, State: st, Now: testNow}

	first, err := VerifyLock(newer, lock, policy)
	require.NoError(t, err)
	require.NoError(t, first.Commit(st))
	reopened, err := OpenState(statePath, secretPath)
	require.NoError(t, err)
	policy.State = reopened
	_, err = VerifyLock(older, lock, policy)

	require.Error(t, err)
	assert.Equal(t, CodeRollback, CodeOf(err))
	_, err = VerifyLock(newer, lock, policy)
	assert.NoError(t, err, "re-presenting the newest attestation is not a rollback")
}

func setUserDirs(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))
}

func TestStateRejectsTamperedFile(t *testing.T) {
	setUserDirs(t)
	statePath, secretPath := StatePaths(nil)
	st, err := OpenState(statePath, secretPath)
	require.NoError(t, err)
	require.NoError(t, st.Advance("repo", testNow))
	data, err := os.ReadFile(statePath)
	require.NoError(t, err)
	forged := strings.Replace(string(data), "2026-10-06", "2020-01-01", 1)
	require.NotEqual(t, string(data), forged)
	require.NoError(t, os.WriteFile(statePath, []byte(forged), 0o600))

	reopened, err := OpenState(statePath, secretPath)

	require.NoError(t, err)
	assert.True(t, reopened.Reset, "a state that fails its MAC is discarded and reported")
	_, ok := reopened.Highwater("repo")
	assert.False(t, ok)
}

func TestKeyFormats(t *testing.T) {
	tests := []struct {
		name     string
		password []byte
	}{
		{"plain PKCS8 ECDSA", nil},
		{"cosign encrypted", []byte("correct horse")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			priv, pub, err := GenerateKeyPair(tt.password)
			require.NoError(t, err)

			kp, err := ParsePrivateKey(priv, tt.password)

			require.NoError(t, err)
			pk, err := ParsePublicKey(pub)
			require.NoError(t, err)
			fp, err := Fingerprint(pk)
			require.NoError(t, err)
			assert.Equal(t, fp, kp.Fingerprint())
		})
	}
	priv, _, err := GenerateKeyPair([]byte("pw"))
	require.NoError(t, err)
	_, err = ParsePrivateKey(priv, nil)
	assert.Error(t, err, "an encrypted key without a password is refused")
	_, err = ParsePrivateKey(priv, []byte("wrong"))
	assert.Error(t, err)
}

func TestIdentityRegexpMustBeAnchored(t *testing.T) {
	tests := []struct {
		expr string
		ok   bool
	}{
		{`^https://github\.com/org/[^/]+/\.github/workflows/release\.yml@refs/heads/main$`, true},
		{`https://github.com/org/.*`, false},
		{`^https://github.com/org/.*`, false},
		{`.*release@example\.org$`, false},
		{`^a\$`, false},
		{`^(`, false},
	}
	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			err := ValidateIdentityRegexp(tt.expr)
			assert.Equal(t, tt.ok, err == nil, "%v", err)
		})
	}
}

func TestKeylessLockWithVirtualSigstore(t *testing.T) {
	const (
		identity = "https://github.com/example/ai-config/.github/workflows/release.yml@refs/heads/main"
		issuer   = "https://token.actions.githubusercontent.com"
	)
	fs := newFakeSigstore(t, identity, issuer)
	trusted := fs.trustedRoot()
	lock := testLock(t, nil)
	st, err := LockStatement(lock, LockMeta{Version: "5", Repository: "repo", Now: time.Now()})
	require.NoError(t, err)
	payload, err := st.Marshal()
	require.NoError(t, err)
	data := fs.bundle(payload)
	trustFor := func(e TrustEntry) TrustSet { e.Subject = SubjectLock; return TrustSet{Entries: []TrustEntry{e}} }

	tests := []struct {
		name     string
		trusted  root.TrustedMaterial
		trust    TrustSet
		tlog     TLogMode
		wantCode string
	}{
		{"exact identity", trusted, trustFor(TrustEntry{Identity: identity, Issuer: issuer}), TLogRequired, ""},
		{"anchored regexp", trusted, trustFor(TrustEntry{IdentityRegexp: `^https://github\.com/example/[^/]+/\.github/workflows/release\.yml@refs/heads/main$`, Issuer: issuer}), TLogRequired, ""},
		{"alternation cannot match a substring", trusted, trustFor(TrustEntry{IdentityRegexp: `^nomatch|workflows/release\.yml@refs/heads/main$`, Issuer: issuer}), TLogRequired, CodeSignerNotTrusted},
		{"wrong issuer", trusted, trustFor(TrustEntry{Identity: identity, Issuer: "https://accounts.example.com"}), TLogRequired, CodeSignerNotTrusted},
		{"look-alike identity", trusted, trustFor(TrustEntry{Identity: identity + "x", Issuer: issuer}), TLogRequired, CodeSignerNotTrusted},
		{"entry not yet valid", trusted, trustFor(TrustEntry{Identity: identity, Issuer: issuer, ValidFrom: time.Now().Add(24 * time.Hour)}), TLogRequired, CodeSignerNotTrusted},
		{"entry expired", trusted, trustFor(TrustEntry{Identity: identity, Issuer: issuer, ValidUntil: time.Now().Add(-24 * time.Hour)}), TLogRequired, CodeSignerNotTrusted},
		{"no trusted root", nil, trustFor(TrustEntry{Identity: identity, Issuer: issuer}), TLogRequired, CodeRootUnavailable},
		{"tlog off needs keys", trusted, trustFor(TrustEntry{Identity: identity, Issuer: issuer}), TLogOff, CodeInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := Verifier{TLog: tt.tlog}
			if tt.trusted != nil {
				v.TrustedRoot = tt.trusted
			}

			rep, err := VerifyLock(data, lock, LockPolicy{Verifier: v, Trust: tt.trust, Now: time.Now()})

			if tt.wantCode == "" {
				require.NoError(t, err)
				assert.Equal(t, KindKeyless, rep.Result.Signer.Kind)
				assert.Equal(t, identity, rep.Result.Signer.Identity)
				assert.Equal(t, issuer, rep.Result.Signer.Issuer)
				assert.False(t, rep.Result.Weak)
				assert.True(t, rep.Result.Logged)
				return
			}
			require.Error(t, err)
			assert.Equal(t, tt.wantCode, CodeOf(err), err.Error())
		})
	}
}

func TestKeylessSignatureForOtherRootFails(t *testing.T) {
	fs := newFakeSigstore(t, "id", "iss")
	other := newFakeSigstore(t, "id", "iss")
	lock := testLock(t, nil)
	st, err := LockStatement(lock, LockMeta{Now: time.Now()})
	require.NoError(t, err)
	payload, err := st.Marshal()
	require.NoError(t, err)
	data := fs.bundle(payload)

	_, err = (&Verifier{TrustedRoot: other.trustedRoot(), TLog: TLogRequired}).Verify(data)

	require.Error(t, err)
	assert.Equal(t, CodeInvalid, CodeOf(err))
}

func TestStatementHelpers(t *testing.T) {
	st, err := NewStatement("https://example.com/p/v1", []Subject{{Name: "a", Digest: map[string]string{"sha256": "abc"}}}, map[string]int{"n": 1})
	require.NoError(t, err)
	payload, err := st.Marshal()
	require.NoError(t, err)

	parsed, err := ParseStatement(payload)

	require.NoError(t, err)
	assert.NoError(t, parsed.RequireSubject("sha256", "ABC"))
	assert.Equal(t, CodeSubjectMismatch, CodeOf(parsed.RequireSubject("sha256", "abe")))
	assert.Equal(t, CodeSubjectMismatch, CodeOf(parsed.RequireSubject("sha256", "")))
	_, err = ParseStatement([]byte(`{"_type":"other","subject":[{"name":"a","digest":{"sha256":"a"}}]}`))
	assert.Equal(t, CodeInvalid, CodeOf(err))
	_, err = NewStatement("", nil, nil)
	assert.Error(t, err)
}

func TestSignStatementCustomPredicate(t *testing.T) {
	priv, pub, err := GenerateKeyPair(nil)
	require.NoError(t, err)
	signer, err := LoadKeySigner(priv, nil)
	require.NoError(t, err)
	st, err := NewStatement("https://example.com/predicate/v1", []Subject{{Name: "x", Digest: map[string]string{"sha256": "abc"}}}, map[string]int{"n": 1})
	require.NoError(t, err)

	data, err := SignStatement(context.Background(), signer, st)

	require.NoError(t, err)
	res, err := (&Verifier{Keys: keyTrust(t, pub).Keys(SubjectLock), TLog: TLogOff}).Verify(data)
	require.NoError(t, err)
	assert.Equal(t, PayloadTypeInToto, res.PayloadType)
	assert.Equal(t, "https://example.com/predicate/v1", res.Statement.PredicateType)
	assert.JSONEq(t, `{"n":1}`, string(res.Statement.Predicate))
	assert.NoError(t, res.Statement.RequireSubject("sha256", "abc"))
	info, err := Inspect(data)
	require.NoError(t, err)
	assert.Equal(t, res.Signer.KeyID, info.KeyID)
}

func TestSignLockRefusesEditedLock(t *testing.T) {
	lock := testLock(t, nil)
	lock.Tree = "sha256:" + hexRepeat("11")
	priv, _, err := GenerateKeyPair(nil)
	require.NoError(t, err)
	signer, err := LoadKeySigner(priv, nil)
	require.NoError(t, err)

	_, err = SignLock(context.Background(), signer, lock, LockMeta{Now: testNow})

	require.Error(t, err)
	assert.Equal(t, CodeSubjectMismatch, CodeOf(err))
}

// blobBundle signs the lock-subject statement file like `cosign sign-blob --bundle`.
func blobBundle(t *testing.T, privPEM []byte, lock *lockfile.File) []byte {
	t.Helper()
	kp, err := ParsePrivateKey(privPEM, nil)
	require.NoError(t, err)
	subject, err := lockSubjectOf(lock)
	require.NoError(t, err)
	statement, err := subject.Statement().JSON()
	require.NoError(t, err)
	pb, err := buildBundle(context.Background(), &PlainData{Data: statement}, kp, nil, "", nil)
	require.NoError(t, err)
	data, err := protojson.Marshal(pb)
	require.NoError(t, err)
	return data
}

func TestVerifyLockAcceptsCosignBlobBundle(t *testing.T) {
	lock := testLock(t, nil)
	priv, pub, err := GenerateKeyPair(nil)
	require.NoError(t, err)
	trust := keyTrust(t, pub)
	policy := LockPolicy{Verifier: Verifier{Keys: trust.Keys(SubjectLock), TLog: TLogOff}, Trust: trust, Now: testNow}
	data := blobBundle(t, priv, lock)

	rep, err := VerifyLock(data, lock, policy)

	require.NoError(t, err)
	assert.True(t, IsBlobBundle(data))
	assert.Equal(t, 1, rep.Predicate.HashVersion)
	assert.Equal(t, KindKey, rep.Result.Signer.Kind)

	changed := testLock(t, func(f *lockfile.File) { f.Item[0].Digest = "sha256:" + hexRepeat("ee") })
	_, err = VerifyLock(data, changed, policy)
	require.Error(t, err)
	assert.Equal(t, CodeSubjectMismatch, CodeOf(err), "a lock edited after signing is told apart from a forged signature")

	policy.MaxAge = time.Hour
	_, err = VerifyLock(data, lock, policy)
	require.Error(t, err)
	assert.Equal(t, CodeStale, CodeOf(err), "a log-less blob bundle has no time, so max_age cannot be met")
}
