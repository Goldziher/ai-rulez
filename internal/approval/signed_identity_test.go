package approval

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/signing"
)

var signedDigest = "sha256:" + strings.Repeat("ab", 32)

type approvalKey struct {
	fp     string
	signer signing.Signer
}

// signedProject writes one key pair per reviewer into a project whose [signing]
// trust table maps each key to that reviewer ("" maps it to nobody).
func signedProject(t *testing.T, reviewers ...string) (*config.Config, []approvalKey) {
	t.Helper()
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	cfg := &config.Config{BaseDir: dir, ConfigDir: dir, Signing: &config.SigningConfig{}}
	keys := make([]approvalKey, 0, len(reviewers))
	for i, reviewer := range reviewers {
		priv, pub, err := signing.GenerateKeyPair(nil)
		require.NoError(t, err)
		s, err := signing.LoadKeySigner(priv, nil)
		require.NoError(t, err)
		name := fmt.Sprintf("k%d.pub", i)
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), pub, 0o600))
		pk, err := signing.ParsePublicKey(pub)
		require.NoError(t, err)
		fp, err := signing.Fingerprint(pk)
		require.NoError(t, err)
		cfg.Signing.Trust = append(cfg.Signing.Trust, config.SigningTrust{Subject: "approval", KeyFile: name, Reviewer: reviewer})
		keys = append(keys, approvalKey{fp: fp, signer: s})
	}
	return cfg, keys
}

// signedRecord signs an approval of s with k, stores its attestation and returns
// the lock record.
func signedRecord(t *testing.T, cfg *config.Config, k approvalKey, s Subject, approvedAt string) lockfile.Approval {
	t.Helper()
	st, err := signing.ApprovalStatement(signing.ApprovalSubject{Kind: s.Kind, Domain: s.Domain, ID: s.ID, Digest: s.Digest},
		signing.ApprovalPredicate{ApprovedAt: approvedAt})
	require.NoError(t, err)
	b, err := signing.SignStatement(context.Background(), k.signer, st)
	require.NoError(t, err)
	d := BundleDigest(b)
	path, err := AttestationFile(cfg.ConfigDir, d)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, b, 0o600))
	return lockfile.Approval{Kind: s.Kind, ID: s.ID, Domain: s.Domain, Digest: s.Digest, Reviewer: "key:" + k.fp,
		Assurance: lockfile.AssuranceSigned, ApprovedAt: approvedAt, Attestation: d}
}

// TestEvaluate_CountsPeopleNotKeys is RV-GOV-3: min_approvers counts the people
// the trust table maps keys to, not the keys.
func TestEvaluate_CountsPeopleNotKeys(t *testing.T) {
	s := Subject{Kind: KindInclude, ID: "shared", Digest: signedDigest, Class: ClassRemote}
	now := time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)
	const at = "2026-10-01T00:00:00Z"
	tests := []struct {
		name          string
		reviewers     []string
		wantStatus    string
		wantReviewers int
	}{
		{"one person with two keys is one approver", []string{"alice@example.org", "Alice@Example.org"}, StatusInsufficient, 1},
		{"two people are two approvers", []string{"alice@example.org", "bob@example.org"}, StatusOK, 2},
		{"keys mapped to nobody count as themselves", []string{"", ""}, StatusOK, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg, keys := signedProject(t, tt.reviewers...)
			recs := []lockfile.Approval{signedRecord(t, cfg, keys[0], s, at), signedRecord(t, cfg, keys[1], s, at)}
			p := Policy{Selectors: []string{"remote"}, MinApprovers: 2, MinAssurance: lockfile.AssuranceSigned, Signed: newConfigVerifier(cfg)}

			// Act
			res := p.Evaluate(recs, s, now)

			// Assert
			assert.Equal(t, tt.wantStatus, res.Status, res.Detail)
			assert.Len(t, res.Reviewers, tt.wantReviewers)
		})
	}
}
