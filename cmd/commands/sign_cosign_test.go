package commands

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/signing"
	"github.com/Goldziher/ai-rulez/v5/internal/signing/sigstore"
)

const cosignTimeout = 60 * time.Second

func runCosign(t *testing.T, env []string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), cosignTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "cosign", args...)
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "cosign %v:\n%s", args, out)
}

// TestCosignInterop checks that a bundle made by `ai-rulez sign` verifies with
// cosign and that the bundles cosign writes verify with `ai-rulez verify
// --attestation`. It needs the cosign binary and is skipped without it.
func TestCosignInterop(t *testing.T) {
	if _, err := exec.LookPath("cosign"); err != nil {
		t.Skip("cosign is not installed")
	}
	f := newSignFixture(t, "\n[signing]\nkey_file = \"keys/release.pub\"\n")
	const password = "interop"
	priv, pub, err := sigstore.GenerateKeyPair([]byte(password))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(f.privKey, priv, 0o600))
	require.NoError(t, os.WriteFile(f.pubKey, pub, 0o644))
	work := t.TempDir()
	keyEnv := []string{"COSIGN_PASSWORD=" + password, "HOME=" + t.TempDir()}
	lock, err := lockfile.Load(filepath.Join(f.root, ".ai-rulez"))
	require.NoError(t, err)
	subject := contentlock.SubjectOf(lock)
	statement, err := subject.Statement().JSON()
	require.NoError(t, err)
	subjectFile := filepath.Join(work, "lock-subject.json")
	require.NoError(t, os.WriteFile(subjectFile, statement, 0o644))
	hexDigest := subject.Digest()[len("sha256:"):]
	offline := []string{"--use-signing-config=false", "--new-bundle-format=true", "--tlog-upload=false", "--yes"}

	t.Run("ai-rulez sign verifies with cosign verify-blob-attestation", func(t *testing.T) {
		signLock, signKey = true, f.privKey
		t.Setenv(signPasswordEnv, password)
		require.Equal(t, 0, func() int {
			var c int
			capture(t, func() { c = codeOf(runSign(context.Background(), nil, nil)) })
			return c
		}())
		signLock, signKey = false, ""

		runCosign(t, nil, "verify-blob-attestation", "--bundle", f.bundle(), "--key", f.pubKey,
			"--type", signing.PredicateLock, "--digest", hexDigest, "--digestAlg", "sha256", "--insecure-ignore-tlog=true")

		code, stdout, _ := f.verify(t, "")
		assert.Equal(t, 0, code)
		assert.Contains(t, stdout, "OK  lock")
	})

	t.Run("cosign sign-blob over lock-subject.json verifies with ai-rulez", func(t *testing.T) {
		require.NoError(t, os.Remove(f.bundle()))
		runCosign(t, keyEnv, append([]string{"sign-blob", "--key", f.privKey, "--bundle", f.bundle(), subjectFile}, offline...)...)

		verifyNoState = true
		defer func() { verifyNoState = false }()
		code, stdout, stderr := f.verify(t, "")

		assert.Equal(t, 0, code, stderr)
		assert.Contains(t, stdout, "OK  lock")
		runCosign(t, nil, "verify-blob", "--key", f.pubKey, "--bundle", f.bundle(), "--insecure-ignore-tlog=true", subjectFile)
	})

	t.Run("cosign attest-blob verifies with ai-rulez", func(t *testing.T) {
		require.NoError(t, os.Remove(f.bundle()))
		pred, err := json.Marshal(signing.LockPredicate{
			HashVersion: subject.HashVersion, Tree: subject.Tree, ApprovalsDigest: subject.ApprovalsDigest, Scope: subject.Scope,
			OutputsPinned: subject.OutputsPinned, AIRulezVersion: "cosign", IssuedAt: time.Now().UTC().Truncate(time.Second),
		})
		require.NoError(t, err)
		predFile := filepath.Join(work, "predicate.json")
		require.NoError(t, os.WriteFile(predFile, pred, 0o644))
		runCosign(t, keyEnv, append([]string{"attest-blob", "--key", f.privKey, "--predicate", predFile, "--type", signing.PredicateLock,
			"--hash", hexDigest, "--bundle", f.bundle(), subjectFile}, offline...)...)

		verifyNoState = true
		defer func() { verifyNoState = false }()
		code, stdout, stderr := f.verify(t, "")

		assert.Equal(t, 0, code, stderr)
		assert.Contains(t, stdout, "OK  lock")
	})
}
