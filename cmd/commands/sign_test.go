package commands

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/signing"
)

// signFixture is a locked project that trusts one key, with the private key
// kept outside the project.
type signFixture struct {
	root    string
	privKey string
	pubKey  string
}

func resetSigningFlags(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		signLock, signKey, signKeyPassEnv, signKeyless, signTokenEnv, signFulcioURL, signRekorURL = false, "", "", false, "", "", ""
		signTLog, signEmbedItems, signOutput, signInteractive = false, false, "", false
		verifyAttestation, verifyAttLock, verifyAttFile, verifyTrustedRoot, verifyPublicKeys = false, false, "", "", nil
		verifyIdentity, verifyIssuer, verifyNoState, verifyFormat = "", "", false, ""
	})
}

func newSignFixture(t *testing.T, signingTable string) *signFixture {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	resetSigningFlags(t)
	resetLockViewFlags(t)
	root := lockProject(t, signingTable)
	priv, pub, err := signing.GenerateKeyPair(nil)
	require.NoError(t, err)
	f := &signFixture{root: root, privKey: filepath.Join(t.TempDir(), "release.key"), pubKey: filepath.Join(root, "keys", "release.pub")}
	require.NoError(t, os.WriteFile(f.privKey, priv, 0o600))
	require.NoError(t, os.MkdirAll(filepath.Dir(f.pubKey), 0o755))
	require.NoError(t, os.WriteFile(f.pubKey, pub, 0o644))
	require.Equal(t, 0, writeLockAt("", "", nil))
	return f
}

func (f *signFixture) bundle() string {
	return filepath.Join(f.root, ".ai-rulez", config.SigningAttestationFile)
}

func (f *signFixture) sign(t *testing.T) int {
	t.Helper()
	signLock, signKey = true, f.privKey
	defer func() { signLock, signKey = false, "" }()
	var code int
	capture(t, func() { code = runSign(context.Background(), nil, nil) })
	return code
}

func (f *signFixture) verify(t *testing.T, format string) (code int, stdout, stderr string) {
	t.Helper()
	verifyAttestation, verifyFormat = true, format
	defer func() { verifyAttestation, verifyFormat = false, "" }()
	stdout, stderr = capture(t, func() { code = runVerifyAttestation(nil, nil, os.Stdout) })
	return code, stdout, stderr
}

const signingKeyTable = "\n[signing]\nkey_file = \"keys/release.pub\"\nmax_age = \"30d\"\n"

func TestSignAndVerifyLock(t *testing.T) {
	f := newSignFixture(t, signingKeyTable)

	require.Equal(t, 0, f.sign(t))
	code, stdout, _ := f.verify(t, "")

	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "OK  lock  signer=sha256:")
	assert.Contains(t, stdout, "hash_version=1")
	assert.Contains(t, stdout, "weak freshness", "a key bundle without a log has no trustworthy time")
	info, err := os.Stat(f.bundle())
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), info.Mode().Perm(), "the attestation is public")
}

func TestVerifyAttestationJSON(t *testing.T) {
	f := newSignFixture(t, signingKeyTable)
	require.Equal(t, 0, f.sign(t))

	code, stdout, _ := f.verify(t, formatJSON)

	assert.Equal(t, 0, code)
	validateAgainst(t, "../../schema/verify-attestation.schema.json", []byte(stdout))
	var doc attestationReport
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc))
	require.Len(t, doc.Results, 1)
	assert.Equal(t, attestationValid, doc.Results[0].Status)
	assert.Equal(t, "lock", doc.Results[0].Subject)
	assert.Equal(t, signing.KindKey, doc.Results[0].Signer.Kind)
	assert.Equal(t, 1, doc.Results[0].HashVersion)
}

func TestVerifyAttestationFailures(t *testing.T) {
	tests := []struct {
		name       string
		table      string
		prepare    func(t *testing.T, f *signFixture)
		wantCode   int
		wantAR     string
		wantStatus string
	}{
		{"no attestation file", signingKeyTable, func(*testing.T, *signFixture) {}, exitDrift, "AR720", attestationMissing},
		{"lock changed after signing", signingKeyTable, func(t *testing.T, f *signFixture) {
			require.Equal(t, 0, f.sign(t))
			require.NoError(t, os.WriteFile(filepath.Join(f.root, ".ai-rulez", "rules", "style.md"), []byte("# Style\nUse spaces.\n"), 0o644))
			require.Equal(t, 0, writeLockAt("", "", nil))
		}, exitDrift, "AR724", attestationInvalid},
		{"untrusted key", signingKeyTable, func(t *testing.T, f *signFixture) {
			require.Equal(t, 0, f.sign(t))
			_, other, err := signing.GenerateKeyPair(nil)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(f.pubKey, other, 0o644))
		}, exitDrift, "AR722", attestationInvalid},
		{"corrupt bundle", signingKeyTable, func(t *testing.T, f *signFixture) {
			require.NoError(t, os.WriteFile(f.bundle(), []byte("not json"), 0o644))
		}, exitDrift, "AR721", attestationInvalid},
		{"tlog required for a key bundle", "\n[signing]\nkey_file = \"keys/release.pub\"\ntlog = \"required\"\n", func(t *testing.T, f *signFixture) {
			require.Equal(t, 0, f.sign(t))
		}, exitDrift, "AR726", attestationInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newSignFixture(t, tt.table)
			tt.prepare(t, f)

			code, stdout, stderr := f.verify(t, formatJSON)

			assert.Equal(t, tt.wantCode, code, stderr)
			var doc attestationReport
			require.NoError(t, json.Unmarshal([]byte(stdout), &doc), stdout)
			require.Len(t, doc.Results, 1)
			assert.Equal(t, tt.wantAR, doc.Results[0].Code)
			assert.Equal(t, tt.wantStatus, doc.Results[0].Status)
			validateAgainst(t, "../../schema/verify-attestation.schema.json", []byte(stdout))
		})
	}
}

func TestVerifyAttestationNeedsATrustedSigner(t *testing.T) {
	f := newSignFixture(t, "")
	require.Equal(t, 0, f.sign(t))

	code, _, stderr := f.verify(t, "")

	assert.Equal(t, 1, code, "no trust set is a tool error, never an accept-anyone verification")
	assert.Contains(t, stderr, "no trusted signer")

	verifyPublicKeys = []string{f.pubKey}
	code, stdout, _ := f.verify(t, "")
	assert.Equal(t, 0, code, "--public-key supplies the signer")
	assert.Contains(t, stdout, "OK  lock")
}

func TestVerifyAttestationRollback(t *testing.T) {
	f := newSignFixture(t, "\n[signing]\nkey_file = \"keys/release.pub\"\n")
	require.Equal(t, 0, f.sign(t))
	first, err := os.ReadFile(f.bundle())
	require.NoError(t, err)
	code, _, _ := f.verify(t, "")
	require.Equal(t, 0, code)
	// A weak (log-less) bundle falls back to its claimed time: a later signature
	// raises the mark, and the earlier bundle is then a rollback.
	require.NoError(t, os.WriteFile(f.bundle(), signAt(t, f, 2*time.Minute), 0o644))
	code, _, _ = f.verify(t, "")
	require.Equal(t, 0, code)

	require.NoError(t, os.WriteFile(f.bundle(), first, 0o644))
	code, _, stderr := f.verify(t, "")

	assert.Equal(t, exitDrift, code)
	assert.Contains(t, stderr, "AR727")
	verifyNoState = true
	code, _, _ = f.verify(t, "")
	assert.Equal(t, 0, code, "--no-state skips the rollback check")
}

func signAt(t *testing.T, f *signFixture, ahead time.Duration) []byte {
	t.Helper()
	priv, err := os.ReadFile(f.privKey)
	require.NoError(t, err)
	signer, err := signing.LoadKeySigner(priv, nil)
	require.NoError(t, err)
	lock, err := lockfile.Load(filepath.Join(f.root, ".ai-rulez"))
	require.NoError(t, err)
	data, err := signing.SignLock(context.Background(), signer, lock, signing.LockMeta{Version: "5", Now: time.Now().Add(ahead)})
	require.NoError(t, err)
	return data
}

func TestRequireLockSignatureGates(t *testing.T) {
	f := newSignFixture(t, "\n[signing]\nrequire = [\"lock\"]\nkey_file = \"keys/release.pub\"\n")

	t.Run("lock --check fails without an attestation", func(t *testing.T) {
		var code int
		_, stderr := capture(t, func() { code = checkLockAt("") })

		assert.Equal(t, exitDrift, code)
		assert.Contains(t, stderr, "AR720")
	})

	t.Run("generate --locked fails without an attestation", func(t *testing.T) {
		cfg, _, err := loadForLockCheck("")
		require.NoError(t, err)
		generateLocked = true
		defer func() { generateLocked = false }()

		err = enforceLockedContent(cfg)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "AR720")
	})

	t.Run("validate --strict reports it", func(t *testing.T) {
		cfg, _, err := loadForLockCheck("")
		require.NoError(t, err)

		findings := signingFindingsFor(cfg)

		require.Len(t, findings, 1)
		assert.Equal(t, "AR720", findings[0].Code)
		assert.Equal(t, ".ai-rulez/ai-rulez.lock", findings[0].Path)
	})

	t.Run("a valid attestation passes every gate", func(t *testing.T) {
		require.Equal(t, 0, f.sign(t))
		cfg, _, err := loadForLockCheck("")
		require.NoError(t, err)
		generateLocked = true
		defer func() { generateLocked = false }()

		var code int
		capture(t, func() { code = checkLockAt("") })

		assert.Equal(t, 0, code)
		assert.NoError(t, enforceLockedContent(cfg))
		assert.Empty(t, signingFindingsFor(cfg))
	})

	t.Run("an edited lock invalidates it", func(t *testing.T) {
		require.NoError(t, os.WriteFile(filepath.Join(f.root, ".ai-rulez", "rules", "style.md"), []byte("# Style\nUse spaces.\n"), 0o644))
		require.Equal(t, 0, writeLockAt("", "", nil))

		var code int
		_, stderr := capture(t, func() { code = checkLockAt("") })

		assert.Equal(t, exitDrift, code)
		assert.Contains(t, stderr, "AR724")
	})
}

func TestSignFlagValidation(t *testing.T) {
	tests := []struct {
		name string
		set  func()
		want string
	}{
		{"nothing to sign", func() {}, "nothing to sign"},
		{"no mode", func() { signLock = true }, "choose how to sign"},
		{"both modes", func() { signLock, signKey, signKeyless = true, "k", true }, "mutually exclusive"},
		{"tlog with keyless", func() { signLock, signKeyless, signTLog = true, true, true }, "always logged"},
		{"token env with a key", func() { signLock, signKey, signTokenEnv = true, "k", "T" }, "apply to --keyless"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetSigningFlags(t)
			tt.set()

			err := validateSignFlags()

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestSignRefusesEditedLock(t *testing.T) {
	f := newSignFixture(t, signingKeyTable)
	lockPath := lockfile.Path(filepath.Join(f.root, ".ai-rulez"))
	lock, err := lockfile.Load(filepath.Join(f.root, ".ai-rulez"))
	require.NoError(t, err)
	lock.Item[0].Digest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	require.NoError(t, lockfile.Save(filepath.Join(f.root, ".ai-rulez"), lock))
	_, statErr := os.Stat(lockPath)
	require.NoError(t, statErr)

	assert.Equal(t, exitDrift, f.sign(t))
	_, err = os.Stat(f.bundle())
	assert.True(t, os.IsNotExist(err), "nothing is written for a lock that does not match its entries")
}

func TestNormalizeRemote(t *testing.T) {
	tests := []struct{ in, want string }{
		{"https://github.com/example/ai-config.git", "https://github.com/example/ai-config"},
		{"https://user:secret@github.com/example/ai-config", "https://github.com/example/ai-config"},
		{"git@github.com:example/ai-config.git", "https://github.com/example/ai-config"},
		{"ssh://git@git.example.com:2222/team/repo.git", "https://git.example.com/team/repo"},
		{"", ""},
		{"not-a-remote", ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, tt.want, normalizeRemote(tt.in))
		})
	}
}
