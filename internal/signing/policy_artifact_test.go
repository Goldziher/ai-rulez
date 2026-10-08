package signing_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/Goldziher/ai-rulez/v5/internal/signing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func writeKey(t *testing.T, root, rel string, pubPEM []byte) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, pubPEM, 0o644))
}

func TestPrepareArtifactCheckBuildsTheTrustOfOneSubject(t *testing.T) {
	skillKey, bundleKey := newTestSigner(t, "", ""), newTestSigner(t, "", "")
	cfg := policyProject(t, &config.SigningConfig{
		Thresholds: map[string]int{"skill": 2, "bundle": 1},
		Trust: []config.SigningTrust{
			{Subject: "skill", Source: "shared", KeyFile: "keys/skill-a.pub"},
			{Subject: "skill", Source: "shared", KeyFile: "keys/skill-b.pub"},
			{Subject: "bundle", KeyFile: "keys/bundle.pub"},
		},
		Builders: []string{"builder-1"}, RequireProvenance: true,
	})
	writeKey(t, cfg.BaseDir, "keys/skill-a.pub", skillKey.pubPEM)
	writeKey(t, cfg.BaseDir, "keys/skill-b.pub", skillKey.pubPEM)
	writeKey(t, cfg.BaseDir, "keys/bundle.pub", bundleKey.pubPEM)

	skill, err := PrepareArtifactCheck(cfg, SubjectSkill, VerifyOptions{NoState: true, Now: testNow})
	require.NoError(t, err)
	bundle, err := PrepareArtifactCheck(cfg, SubjectBundle, VerifyOptions{NoState: true, Now: testNow})
	require.NoError(t, err)

	assert.Equal(t, 2, skill.Policy.Threshold)
	assert.Len(t, skill.Policy.Verifier.Keys, 2, "only the skill entries are trusted keys for a skill")
	assert.Equal(t, 1, bundle.Policy.Threshold)
	assert.Len(t, bundle.Policy.Verifier.Keys, 1)
	assert.Equal(t, TLogOff, skill.Policy.Verifier.TLog, "keys only: no log needed")
	assert.True(t, bundle.RequireProvenance)
	assert.Equal(t, []string{"builder-1"}, bundle.Builders)
	assert.NotEmpty(t, skill.Warnings, "the trusted signers come from the repository's own configuration")
}

func TestPrepareArtifactCheckNeedsASignerForTheSubject(t *testing.T) {
	cfg := policyProject(t, &config.SigningConfig{Trust: []config.SigningTrust{{Subject: "bundle", KeyFile: "keys/b.pub"}}})
	key := newTestSigner(t, "", "")
	writeKey(t, cfg.BaseDir, "keys/b.pub", key.pubPEM)

	_, err := PrepareArtifactCheck(cfg, SubjectSBOM, VerifyOptions{NoState: true, Now: testNow})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no trusted signer is configured for sbom")
}

func TestPrepareArtifactCheckTakesFlagSignersForItsSubject(t *testing.T) {
	cfg := policyProject(t, nil)
	key := newTestSigner(t, "", "")
	keyPath := filepath.Join(t.TempDir(), "pub.pem")
	require.NoError(t, os.WriteFile(keyPath, key.pubPEM, 0o644))

	check, err := PrepareArtifactCheck(cfg, SubjectSBOM, VerifyOptions{NoState: true, Now: testNow, PublicKeys: []string{keyPath}})

	require.NoError(t, err)
	assert.Len(t, check.Policy.Verifier.Keys, 1)
	assert.Empty(t, check.Warnings, "signers named outside the repository carry no warning")
	assert.Equal(t, 1, check.Policy.Trust.CountFor(SubjectSBOM))
	assert.Zero(t, check.Policy.Trust.CountFor(SubjectBundle), "a flag key trusts only the subject being verified")
}

func TestCheckLockAttestationRunsWithoutRequire(t *testing.T) {
	setUserDirs(t)
	cfg := policyProject(t, &config.SigningConfig{KeyFile: "keys/release.pub"})
	key := newTestSigner(t, "", "")
	writeKey(t, cfg.BaseDir, "keys/release.pub", key.pubPEM)

	findings, err := CheckLockAttestation(cfg, nil, time.Now())

	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Equal(t, CodeMissing, findings[0].Code, "the lock is unsigned")
	required, err := RequiredLockCheck(cfg, nil, time.Now())
	require.NoError(t, err)
	assert.Empty(t, required, "RequiredLockCheck still asks only when require names the lock")
}
