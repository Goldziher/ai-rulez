package approval

import (
	"context"
	"errors"
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

type fakeVerifier struct {
	who string
	err error
}

func (f fakeVerifier) Verify(lockfile.Approval, Subject, time.Time) (string, error) {
	return f.who, f.err
}

func withAssurance(level string, mods ...func(*lockfile.Approval)) func(*lockfile.Approval) {
	return func(a *lockfile.Approval) {
		a.Assurance = level
		for _, m := range mods {
			m(a)
		}
	}
}

func TestEvaluate_Assurance(t *testing.T) {
	include := Subject{Kind: KindInclude, ID: "shared", Digest: digestB, Class: ClassRemote}
	reviewLinked := withAssurance(lockfile.AssuranceReviewLinked, func(a *lockfile.Approval) {
		a.Ref = "https://github.com/o/r/pull/7#pullrequestreview-1"
	})
	signed := withAssurance(lockfile.AssuranceSigned, func(a *lockfile.Approval) { a.Attestation = "sha256:" + digestHex() })
	tests := []struct {
		name          string
		policy        Policy
		recs          []lockfile.Approval
		want          string
		wantAssurance string
		wantReviewers []string
	}{
		{"asserted meets the default", Policy{Selectors: []string{"remote"}},
			[]lockfile.Approval{rec("include", "shared", digestB, "alice")}, StatusOK, lockfile.AssuranceAsserted, []string{"alice"}},
		{"asserted is below review-linked", Policy{Selectors: []string{"remote"}, MinAssurance: "review-linked"},
			[]lockfile.Approval{rec("include", "shared", digestB, "alice")}, StatusInsufficient, "", nil},
		{"review-linked meets review-linked", Policy{Selectors: []string{"remote"}, MinAssurance: "review-linked"},
			[]lockfile.Approval{rec("include", "shared", digestB, "github:alice", reviewLinked)}, StatusOK, lockfile.AssuranceReviewLinked, []string{"github:alice"}},
		{"review-linked without a ref is unverified", Policy{Selectors: []string{"remote"}},
			[]lockfile.Approval{rec("include", "shared", digestB, "github:alice", withAssurance(lockfile.AssuranceReviewLinked))}, StatusUnverified, "", nil},
		{"signed counts through the verifier", Policy{Selectors: []string{"remote"}, MinAssurance: "signed", Signed: fakeVerifier{who: "alice@example.org"}},
			[]lockfile.Approval{rec("include", "shared", digestB, "alice@example.org", signed)}, StatusOK, lockfile.AssuranceSigned, []string{"alice@example.org"}},
		{"signed that fails verification is unverified", Policy{Selectors: []string{"remote"}, Signed: fakeVerifier{err: errors.New("bad signature")}},
			[]lockfile.Approval{rec("include", "shared", digestB, "alice@example.org", signed)}, StatusUnverified, "", nil},
		{"the signer, not the record, is the reviewer", Policy{Selectors: []string{"remote"}, Approvers: []string{"carol@example.org"}, Signed: fakeVerifier{who: "mallory@example.org"}},
			[]lockfile.Approval{rec("include", "shared", digestB, "carol@example.org", signed)}, StatusUnauthorized, "", nil},
		{"the weakest of several approvals is reported", Policy{Selectors: []string{"remote"}, MinApprovers: 2, Signed: fakeVerifier{who: "bob@example.org"}},
			[]lockfile.Approval{rec("include", "shared", digestB, "alice@example.org"), rec("include", "shared", digestB, "bob@example.org", signed)},
			StatusOK, lockfile.AssuranceAsserted, []string{"alice@example.org", "bob@example.org"}},
		{"alice and github:alice are one reviewer", Policy{Selectors: []string{"remote"}, MinApprovers: 2},
			[]lockfile.Approval{rec("include", "shared", digestB, "alice"), rec("include", "shared", digestB, "github:alice", reviewLinked)},
			StatusInsufficient, lockfile.AssuranceAsserted, []string{"alice"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := tt.policy.Evaluate(tt.recs, include, testNow)

			// Assert
			assert.Equal(t, tt.want, got.Status, got.Detail)
			assert.Equal(t, tt.wantAssurance, got.Assurance)
			if tt.wantReviewers != nil {
				assert.Equal(t, tt.wantReviewers, got.Reviewers)
			}
		})
	}
}

func digestHex() string {
	out := ""
	for range 32 {
		out += "ab"
	}
	return out
}

func TestEvaluate_Deny(t *testing.T) {
	hook := Subject{Kind: "hook", ID: "Stop:*:0", Digest: digestB, Class: ClassLocal}
	denied := map[string]string{digestB: "exfiltrates ~/.ssh"}
	tests := []struct {
		name   string
		policy Policy
		recs   []lockfile.Approval
	}{
		{"not selected by any policy", Policy{Deny: denied}, nil},
		{"selected and approved", Policy{Deny: denied, Selectors: []string{"all"}}, []lockfile.Approval{rec("hook", "Stop:*:0", digestB, "alice")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := tt.policy.Evaluate(tt.recs, hook, testNow)

			// Assert
			assert.Equal(t, StatusDenied, got.Status)
			assert.True(t, got.Failing(), "a denied digest fails whether or not it was required")
			assert.Equal(t, "AR717", CodeOf(got.Status))
			assert.Contains(t, got.Message(), "exfiltrates ~/.ssh")
		})
	}
	other := Subject{Kind: "hook", ID: "x", Digest: digestA, Class: ClassLocal}
	assert.Equal(t, StatusNotRequired, Policy{Deny: denied}.Evaluate(nil, other, testNow).Status)
}

func TestAuthorizedFor_Codeowners(t *testing.T) {
	owners := &OwnerSet{
		Codeowners: ParseCodeowners([]byte("/.ai-rulez/skills/ @acme/security @dave\n")),
		Prefix:     ".ai-rulez", LockPath: ".ai-rulez/ai-rulez.lock",
	}
	skill := Subject{Kind: "skill", ID: "deploy", Path: "skills/deploy"}
	rule := Subject{Kind: "rule", ID: "style", Path: "rules/style.md"}
	teams := NewTeams(map[string][]string{"@acme/security": {"alice@example.org"}})
	p := Policy{Owners: owners, Teams: teams}

	tests := []struct {
		name     string
		policy   Policy
		reviewer string
		subject  Subject
		want     bool
	}{
		{"team member owns the path", p, "alice@example.org", skill, true},
		{"direct owner", p, "github:dave", skill, true},
		{"not an owner", p, "mallory", skill, false},
		{"an unowned path authorizes nobody", p, "alice@example.org", rule, false},
		{"allowlist and owners must both hold", Policy{Owners: owners, Teams: teams, Approvers: []string{"dave"}}, "alice@example.org", skill, false},
		{"allowlist and owners both hold", Policy{Owners: owners, Teams: teams, Approvers: []string{"dave"}}, "dave", skill, true},
		{"a CODEOWNERS file that is missing authorizes nobody", Policy{Owners: &OwnerSet{Problem: "none found"}}, "dave", skill, false},
		{"no approvers_from allows anyone", Policy{}, "anyone", skill, true},
		{"an allowlist team expands through the map", Policy{Approvers: []string{"@acme/security"}, Teams: teams}, "alice@example.org", rule, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.policy.AuthorizedFor(tt.reviewer, tt.subject))
		})
	}
	assert.Equal(t, "none found", Policy{Owners: &OwnerSet{Problem: "none found"}}.OwnersProblem())
	assert.Empty(t, Policy{}.OwnersProblem())
	assert.Equal(t, []string{"@acme/infra"}, Policy{Approvers: []string{"@acme/infra", "@acme/security"}, Teams: teams}.UnresolvedTeams(nil))
}

func TestSubjectsOf_RoleOutputsAreSubjectsOnlyForTheirKind(t *testing.T) {
	// Arrange
	lock := &lockfile.File{Output: []lockfile.OutputPin{
		{Path: "CLAUDE.md", Digest: "sha256:default"},
		{Role: "reviewer", Digest: "sha256:role"},
	}}

	// Act
	subs := SubjectsOf(lock, nil)

	// Assert
	require.Len(t, subs, 1)
	assert.Equal(t, "role-output:reviewer", subs[0].Ref())
	for _, sel := range []string{"all", "local", "remote"} {
		assert.False(t, Policy{Selectors: []string{sel}}.Requires(subs[0]), sel+" must not widen to role outputs")
	}
	assert.True(t, Policy{Selectors: []string{"kind:role-output"}}.Requires(subs[0]))
	assert.Equal(t, StatusMissing, Policy{Selectors: []string{"kind:role-output"}}.Evaluate(nil, subs[0], testNow).Status)
	assert.Equal(t, StatusOK, Policy{Selectors: []string{"kind:role-output"}}.Evaluate(
		[]lockfile.Approval{rec("role-output", "reviewer", "sha256:role", "alice")}, subs[0], testNow).Status)
}

// signedFixture signs an approval of sub with a fresh key and lays out a project
// the configVerifier can read: the public key, the bundle and the trust entry.
func signedFixture(t *testing.T, sub Subject, mutate func(*lockfile.Approval)) (*config.Config, lockfile.Approval) {
	t.Helper()
	base := t.TempDir()
	configDir := filepath.Join(base, ".ai-rulez")
	require.NoError(t, os.MkdirAll(filepath.Join(configDir, lockfile.AttestationDir), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(base, "keys"), 0o755))
	priv, pub, err := signing.GenerateKeyPair(nil)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(base, "keys", "approver.pub"), pub, 0o644))
	signer, err := signing.LoadKeySigner(priv, nil)
	require.NoError(t, err)
	st, err := signing.ApprovalStatement(signing.ApprovalSubject{Kind: sub.Kind, Domain: sub.Domain, ID: sub.ID, Digest: sub.Digest},
		signing.ApprovalPredicate{ApprovedAt: "2026-10-05T00:00:00Z", Expires: "2027-01-01"})
	require.NoError(t, err)
	bundle, err := signing.SignStatement(context.Background(), signer, st)
	require.NoError(t, err)
	info, err := signing.Inspect(bundle)
	require.NoError(t, err)
	digest := BundleDigest(bundle)
	path, err := AttestationFile(configDir, digest)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, bundle, 0o644))
	rec := lockfile.Approval{
		Kind: sub.Kind, ID: sub.ID, Domain: sub.Domain, Digest: sub.Digest, Reviewer: signing.ApprovalReviewer(info),
		Assurance: lockfile.AssuranceSigned, ApprovedAt: "2026-10-05T00:00:00Z", Expires: "2027-01-01", Attestation: digest,
	}
	if mutate != nil {
		mutate(&rec)
	}
	cfg := &config.Config{
		BaseDir: base, ConfigDir: configDir,
		Governance: &config.GovernanceConfig{RequireApproval: []string{"remote"}, MinAssurance: "signed"},
		Signing:    &config.SigningConfig{Trust: []config.SigningTrust{{Subject: "approval", KeyFile: "keys/approver.pub"}}},
	}
	return cfg, rec
}

func TestPolicyOf_VerifiesSignedApprovals(t *testing.T) {
	sub := Subject{Kind: KindInclude, ID: "shared", Digest: "sha256:" + digestHex(), Class: ClassRemote}
	tests := []struct {
		name   string
		mutate func(*lockfile.Approval)
		want   string
	}{
		{"a valid attestation", nil, StatusOK},
		{"a record naming another reviewer", func(a *lockfile.Approval) { a.Reviewer = "mallory@example.org" }, StatusUnverified},
		{"an expiry edited after signing", func(a *lockfile.Approval) { a.Expires = "2099-01-01" }, StatusUnverified},
		{"findings accepted after signing", func(a *lockfile.Approval) { a.AcceptedFindings = []string{"AR005"} }, StatusUnverified},
		{"an attestation digest that points elsewhere", func(a *lockfile.Approval) { a.Attestation = "sha256:" + digestHexOf("cd") }, StatusUnverified},
		{"an attestation field that tries to leave the directory", func(a *lockfile.Approval) { a.Attestation = "../../etc/passwd" }, StatusUnverified},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg, record := signedFixture(t, sub, tt.mutate)
			policy := PolicyOf(cfg)

			// Act
			got := policy.Evaluate([]lockfile.Approval{record}, sub, testNow)

			// Assert
			assert.Equal(t, tt.want, got.Status, got.Detail)
			if tt.want == StatusOK {
				assert.Equal(t, lockfile.AssuranceSigned, got.Assurance)
				assert.Equal(t, []string{NormalizeReviewer(record.Reviewer)}, got.Reviewers)
			}
		})
	}
}

func digestHexOf(pair string) string {
	out := ""
	for range 32 {
		out += pair
	}
	return out
}

func TestPolicyOf_SignedNeedsAnApprovalTrustEntry(t *testing.T) {
	// Arrange: the only trust entry is for the lock
	sub := Subject{Kind: KindInclude, ID: "shared", Digest: "sha256:" + digestHex(), Class: ClassRemote}
	cfg, record := signedFixture(t, sub, nil)
	cfg.Signing.Trust[0].Subject = "lock"

	// Act
	got := PolicyOf(cfg).Evaluate([]lockfile.Approval{record}, sub, testNow)

	// Assert
	assert.Equal(t, StatusUnverified, got.Status)
	assert.Contains(t, got.Detail, "approval signer")
}

func TestPolicyOf_ReadsTheDenyListFromTheLock(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	require.NoError(t, lockfile.Save(dir, &lockfile.File{Deny: []lockfile.Deny{{Digest: digestB, Reason: "malware"}}}))
	cfg := &config.Config{ConfigDir: dir}

	// Act
	p := PolicyOf(cfg)

	// Assert
	assert.Equal(t, map[string]string{digestB: "malware"}, p.Deny)
	assert.Equal(t, StatusDenied, p.Evaluate(nil, Subject{Kind: "rule", ID: "x", Digest: digestB}, testNow).Status)
}
