package signing_test

import (
	"context"
	. "github.com/Goldziher/ai-rulez/v5/internal/signing"
	. "github.com/Goldziher/ai-rulez/v5/internal/signing/sigstore"
	"github.com/Goldziher/ai-rulez/v5/internal/signing/sigstore/fakesigstore"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// testSigner is a key pair for one signer of a test.
type testSigner struct {
	signer *KeySigner
	pubPEM []byte
	pub    TrustEntry
}

func newTestSigner(t *testing.T, subject, source string) testSigner {
	t.Helper()
	priv, pubPEM, err := GenerateKeyPair(nil)
	require.NoError(t, err)
	ks, err := LoadKeySigner(priv, nil)
	require.NoError(t, err)
	pub, err := ParsePublicKey(pubPEM)
	require.NoError(t, err)
	return testSigner{signer: ks, pubPEM: pubPEM, pub: TrustEntry{Subject: subject, Source: source, Key: pub}}
}

func policyFor(subject string, threshold int, signers ...testSigner) ArtifactPolicy {
	trust := TrustSet{}
	for _, s := range signers {
		trust.Entries = append(trust.Entries, s.pub)
	}
	return ArtifactPolicy{
		Subject: subject, Verifier: Verifier{Keys: trust.Keys(subject), TLog: TLogOff}, Trust: trust,
		Threshold: threshold, Now: testNow.Add(time.Hour),
	}
}

// writeTree writes files (relative path to content) under a new directory and returns it.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "acme-plugin")
	for rel, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	}
	return dir
}

func signTree(t *testing.T, s testSigner, subject, dir string) []byte {
	t.Helper()
	st, _, err := TreeStatement(subject, dir, ArtifactMeta{Version: "5.0.0", Repository: "https://github.com/example/plugin", Now: testNow})
	require.NoError(t, err)
	data, err := SignStatement(context.Background(), s.signer, st)
	require.NoError(t, err)
	return data
}

var bundleFiles = map[string]string{
	"plugin.json":            `{"name":"acme"}`,
	"skills/deploy/SKILL.md": "---\nname: deploy\n---\nShip it.\n",
	"scripts/run.sh":         "#!/bin/sh\necho hi\n",
}

func TestTreeSubjectIgnoresSignatureFilesAndIsStable(t *testing.T) {
	dir := writeTree(t, bundleFiles)
	before, err := ReadTreeSubject(KindBundleTree, dir)
	require.NoError(t, err)
	for _, name := range []string{SidecarName, ".ai-rulez.2.sigstore.json", ProvenanceSidecarName} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0o644))
	}

	after, err := ReadTreeSubject(KindBundleTree, dir)

	require.NoError(t, err)
	assert.Equal(t, before.Digest, after.Digest, "signature files are not part of what they sign")
	assert.Len(t, after.Tree.Leaves, len(bundleFiles))
}

func TestTreeSubjectCoversContentModeAndKind(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the executable bit is not a Windows file mode")
	}
	tests := []struct {
		name   string
		mutate func(t *testing.T, dir string)
	}{
		{"a byte changes", func(t *testing.T, dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(`{"name":"evil"}`), 0o644))
		}},
		{"the executable bit flips", func(t *testing.T, dir string) {
			require.NoError(t, os.Chmod(filepath.Join(dir, "scripts", "run.sh"), 0o755))
		}},
		{"a file is added", func(t *testing.T, dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "extra.txt"), []byte("x"), 0o644))
		}},
		{"a file is removed", func(t *testing.T, dir string) {
			require.NoError(t, os.Remove(filepath.Join(dir, "plugin.json")))
		}},
		{"a deeper file is named like a signature", func(t *testing.T, dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "scripts", SidecarName), []byte("x"), 0o644))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeTree(t, bundleFiles)
			before, err := ReadTreeSubject(KindBundleTree, dir)
			require.NoError(t, err)

			tt.mutate(t, dir)
			after, err := ReadTreeSubject(KindBundleTree, dir)

			require.NoError(t, err)
			assert.NotEqual(t, before.Digest, after.Digest)
		})
	}
	t.Run("a bundle and a skill with the same files differ", func(t *testing.T) {
		dir := writeTree(t, bundleFiles)
		b, err := ReadTreeSubject(KindBundleTree, dir)
		require.NoError(t, err)
		s, err := ReadTreeSubject(KindSkillTree, dir)
		require.NoError(t, err)
		assert.NotEqual(t, b.Digest, s.Digest)
	})
}

func TestReadDirTreeRefusesWhatAnAgentCouldReadElsewhere(t *testing.T) {
	dir := writeTree(t, bundleFiles)
	testutil.SymlinkOrSkip(t, "/etc/hosts", filepath.Join(dir, "link"))

	_, err := ReadDirTree(dir)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlink")
	_, err = ReadDirTree(t.TempDir())
	assert.ErrorContains(t, err, "no files")
}

func TestReadDirTreeHandlesVersionControlMetadata(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(t *testing.T, dir string)
		wantErr string
	}{
		{"a root .git directory is skipped", func(t *testing.T, dir string) {
			require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("ref"), 0o644))
		}, ""},
		{"a nested .git directory is refused", func(t *testing.T, dir string) {
			require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub", ".git"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", ".git", "HEAD"), []byte("ref"), 0o644))
		}, "nested version-control"},
		{"a .git pointer file is refused", func(t *testing.T, dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", ".git"), []byte("gitdir: ../x"), 0o644))
		}, ".git file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := writeTree(t, bundleFiles)
			require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "f.txt"), []byte("x"), 0o644))
			tt.mutate(t, dir)
			// Act
			tree, err := ReadDirTree(dir)
			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			for _, l := range tree.Leaves {
				assert.NotContains(t, l.Path, ".git")
			}
		})
	}
}

func TestIsSignatureFile(t *testing.T) {
	tests := map[string]bool{
		".ai-rulez.sigstore.json":            true,
		".ai-rulez.2.sigstore.json":          true,
		".ai-rulez.provenance.sigstore.json": true,
		".ai-rulez.x.sigstore.json":          false,
		"ai-rulez.sigstore.json":             false,
		"sub/.ai-rulez.sigstore.json":        false,
		".ai-rulez.sigstore.json.bak":        false,
		"SKILL.md":                           false,
	}
	for name, want := range tests {
		assert.Equal(t, want, IsSignatureFile(name), name)
	}
}

func TestBundleAttestationRoundTripAndFailures(t *testing.T) {
	publisher := newTestSigner(t, SubjectBundle, "")
	other := newTestSigner(t, SubjectBundle, "")
	dir := writeTree(t, bundleFiles)
	data := signTree(t, publisher, SubjectBundle, dir)
	ts, err := ReadTreeSubject(KindBundleTree, dir)
	require.NoError(t, err)

	tests := []struct {
		name     string
		policy   ArtifactPolicy
		mutate   func()
		wantCode string
	}{
		{"trusted publisher", policyFor(SubjectBundle, 1, publisher), func() {}, ""},
		{"untrusted signer", policyFor(SubjectBundle, 1, other), func() {}, CodeSignerNotTrusted},
		{"entry for another subject", policyFor(SubjectBundle, 1, testSigner{pub: TrustEntry{Subject: SubjectSBOM, Key: publisher.pub.Key}}), func() {}, CodeSignerNotTrusted},
		{"file changed after signing", policyFor(SubjectBundle, 1, publisher), func() {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(`{"name":"evil"}`), 0o644))
		}, CodeSubjectMismatch},
		{"stale", func() ArtifactPolicy {
			p := policyFor(SubjectBundle, 1, publisher)
			p.MaxAge = time.Minute
			p.Now = testNow.Add(24 * time.Hour)
			return p
		}(), func() {}, CodeStale},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(bundleFiles["plugin.json"]), 0o644))
			tt.mutate()
			now, err := ReadTreeSubject(KindBundleTree, dir)
			require.NoError(t, err)

			rep, err := VerifyArtifact([][]byte{data}, TreeExpectation(SubjectBundle, now), tt.policy)

			if tt.wantCode == "" {
				require.NoError(t, err)
				assert.Equal(t, KindKey, rep.Result.Signer.Kind)
				assert.Equal(t, ts.HexDigest(), rep.Statement.Subject[0].Digest["sha256"])
				return
			}
			assert.Equal(t, tt.wantCode, CodeOf(err), "%v", err)
		})
	}
}

func TestVerifyArtifactRejectsAnAttestationOfAnotherKind(t *testing.T) {
	publisher := newTestSigner(t, SubjectBundle, "")
	dir := writeTree(t, bundleFiles)
	skillData := signTree(t, publisher, SubjectSkill, dir)
	ts, err := ReadTreeSubject(KindBundleTree, dir)
	require.NoError(t, err)

	_, err = VerifyArtifact([][]byte{skillData}, TreeExpectation(SubjectBundle, ts), policyFor(SubjectBundle, 1, publisher))

	assert.Equal(t, CodeSubjectMismatch, CodeOf(err), "a skill attestation must not pass for a bundle: %v", err)
}

func TestSourceScopedTrust(t *testing.T) {
	publisher := newTestSigner(t, SubjectSkill, "shared")
	dir := writeTree(t, map[string]string{"SKILL.md": "---\nname: deploy\n---\nShip it.\n"})
	data := signTree(t, publisher, SubjectSkill, dir)
	ts, err := ReadTreeSubject(KindSkillTree, dir)
	require.NoError(t, err)
	p := policyFor(SubjectSkill, 1, publisher)

	tests := []struct {
		name     string
		source   string
		wantCode string
	}{
		{"the scoped source", "shared", ""},
		{"another source", "other", CodeSignerNotTrusted},
		{"no source", "", CodeSignerNotTrusted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p.Source = tt.source

			_, err := VerifyArtifact([][]byte{data}, TreeExpectation(SubjectSkill, ts), p)

			assert.Equal(t, tt.wantCode, CodeOf(err), "%v", err)
		})
	}
}

func TestSBOMAttestation(t *testing.T) {
	publisher := newTestSigner(t, SubjectSBOM, "")
	path := filepath.Join(t.TempDir(), "sbom.cdx.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"bomFormat":"CycloneDX"}`), 0o644))
	st, fs, err := SBOMStatement(path, ArtifactMeta{Version: "5.0.0", Now: testNow})
	require.NoError(t, err)
	data, err := SignStatement(context.Background(), publisher.signer, st)
	require.NoError(t, err)
	p := policyFor(SubjectSBOM, 1, publisher)

	_, err = VerifyArtifact([][]byte{data}, SBOMExpectation(fs), p)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(path, []byte(`{"bomFormat":"CycloneDX","extra":1}`), 0o644))
	changed, err := ReadFileSubject(path)
	require.NoError(t, err)
	_, err = VerifyArtifact([][]byte{data}, SBOMExpectation(changed), p)
	assert.Equal(t, CodeSubjectMismatch, CodeOf(err))

	_, err = ReadFileSubject(t.TempDir())
	assert.Error(t, err, "a directory is not an SBOM")
}

func TestThresholdNeedsDistinctTrustedSigners(t *testing.T) {
	a, b, c := newTestSigner(t, SubjectBundle, ""), newTestSigner(t, SubjectBundle, ""), newTestSigner(t, SubjectBundle, "")
	dir := writeTree(t, bundleFiles)
	byA, byB, byC := signTree(t, a, SubjectBundle, dir), signTree(t, b, SubjectBundle, dir), signTree(t, c, SubjectBundle, dir)
	ts, err := ReadTreeSubject(KindBundleTree, dir)
	require.NoError(t, err)
	exp := TreeExpectation(SubjectBundle, ts)
	twoOfTwo := policyFor(SubjectBundle, 2, a, b)
	garbage := []byte("not a bundle")

	tests := []struct {
		name     string
		bundles  [][]byte
		policy   ArtifactPolicy
		wantCode string
		wantN    int
	}{
		{"two signers meet two", [][]byte{byA, byB}, twoOfTwo, "", 2},
		{"one signer is short", [][]byte{byA}, twoOfTwo, CodeThreshold, 0},
		{"the same signer twice counts once", [][]byte{byA, byA}, twoOfTwo, CodeThreshold, 0},
		{"an untrusted second signer does not count", [][]byte{byA, byC}, twoOfTwo, CodeThreshold, 0},
		{"a corrupt extra file does not block enough valid ones", [][]byte{byA, garbage, byB}, twoOfTwo, "", 2},
		{"two of three", [][]byte{byA, byC}, policyFor(SubjectBundle, 2, a, b, c), "", 2},
		{"a threshold of one needs only one", [][]byte{byA}, policyFor(SubjectBundle, 1, a, b), "", 1},
		{"nothing valid reports the first failure", [][]byte{garbage}, twoOfTwo, CodeInvalid, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rep, err := VerifyArtifact(tt.bundles, exp, tt.policy)

			if tt.wantCode != "" {
				assert.Equal(t, tt.wantCode, CodeOf(err), "%v", err)
				return
			}
			require.NoError(t, err)
			assert.Len(t, rep.Signers(), tt.wantN)
		})
	}
}

func TestVerifyLockSetThreshold(t *testing.T) {
	setUserDirs(t)
	lock := testLock(t, nil)
	a, b := newTestSigner(t, SubjectLock, ""), newTestSigner(t, SubjectLock, "")
	sign := func(s testSigner) []byte {
		data, err := SignLock(context.Background(), s.signer, lock, LockMeta{Version: "5", Now: testNow})
		require.NoError(t, err)
		return data
	}
	trust := TrustSet{Entries: []TrustEntry{a.pub, b.pub}}
	policy := LockPolicy{Verifier: Verifier{Keys: trust.Keys(SubjectLock), TLog: TLogOff}, Trust: trust, Threshold: 2, Now: testNow.Add(time.Hour)}

	rep, err := VerifyLockSet([][]byte{sign(a), sign(b)}, lock, policy)
	require.NoError(t, err)
	require.Len(t, rep.Cosigners, 1)

	_, err = VerifyLockSet([][]byte{sign(a)}, lock, policy)
	assert.Equal(t, CodeThreshold, CodeOf(err))

	policy.Threshold = 1
	rep, err = VerifyLockSet([][]byte{sign(b)}, lock, policy)
	require.NoError(t, err)
	assert.Empty(t, rep.Cosigners)
}

// TestThresholdCountsPeopleNotKeys is RV-GOV-3 on the signing side: the trust
// table maps keys to the person who holds them, and a threshold counts people.
func TestThresholdCountsPeopleNotKeys(t *testing.T) {
	setUserDirs(t)
	lock := testLock(t, nil)
	const issuer = "https://accounts.example.org"
	fs := fakesigstore.New(t, "alice@example.org", issuer)
	payload := func() []byte {
		st, err := LockStatement(lock, LockMeta{Version: "5", Now: time.Now()})
		require.NoError(t, err)
		data, err := st.Marshal()
		require.NoError(t, err)
		return data
	}
	keyed := func(reviewer string) (TrustEntry, []byte) {
		data, pub := signKeyed(t, lock, LockMeta{Now: time.Now()})
		pk, err := ParsePublicKey(pub)
		require.NoError(t, err)
		return TrustEntry{Subject: SubjectLock, Key: pk, Reviewer: reviewer}, data
	}
	alice1, byAlice1 := keyed("alice@example.org")
	alice2, byAlice2 := keyed("Alice@Example.org ")
	bob, byBob := keyed("bob@example.org")
	anon1, byAnon1 := keyed("")
	anon2, byAnon2 := keyed("")
	keyless := TrustEntry{Subject: SubjectLock, Identity: "alice@example.org", Issuer: issuer}
	byKeyless := fs.Bundle(payload())

	tests := []struct {
		name     string
		trust    []TrustEntry
		bundles  [][]byte
		wantCode string
	}{
		{"one person with two keys is one signer", []TrustEntry{alice1, alice2}, [][]byte{byAlice1, byAlice2}, CodeThreshold},
		{"a key and the keyless identity of one person are one signer", []TrustEntry{alice1, keyless}, [][]byte{byAlice1, byKeyless}, CodeThreshold},
		{"two people meet two", []TrustEntry{alice1, bob}, [][]byte{byAlice1, byBob}, ""},
		{"keys no entry names count as themselves", []TrustEntry{anon1, anon2}, [][]byte{byAnon1, byAnon2}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			trust := TrustSet{Entries: tt.trust}
			policy := LockPolicy{Verifier: Verifier{TrustedRoot: fs.TrustedRoot(), Keys: trust.Keys(SubjectLock), TLog: TLogOptional}, Trust: trust, Threshold: 2, Now: time.Now()}

			// Act
			_, err := VerifyLockSet(tt.bundles, lock, policy)

			// Assert
			if tt.wantCode != "" {
				assert.Equal(t, tt.wantCode, CodeOf(err), "%v", err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestBundleFilesAreNumericallyOrdered(t *testing.T) {
	dir := t.TempDir()
	primary := filepath.Join(dir, "ai-rulez.lock.sigstore.json")
	for _, name := range []string{"ai-rulez.lock.sigstore.json", "ai-rulez.lock.10.sigstore.json", "ai-rulez.lock.2.sigstore.json",
		"ai-rulez.lock.provenance.sigstore.json", "ai-rulez.lock.sigstore.json.bak", "other.2.sigstore.json"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0o644))
	}

	files, err := BundleFiles(primary)

	require.NoError(t, err)
	assert.Equal(t, []string{primary, filepath.Join(dir, "ai-rulez.lock.2.sigstore.json"), filepath.Join(dir, "ai-rulez.lock.10.sigstore.json")}, files)
}

func TestBundleFilesReportsMissingAsAR720(t *testing.T) {
	_, err := BundleFiles(filepath.Join(t.TempDir(), "nope.sigstore.json"))

	assert.Equal(t, CodeMissing, CodeOf(err))
}

func TestNextCosignaturePathSkipsUsedNumbers(t *testing.T) {
	dir := t.TempDir()
	primary := filepath.Join(dir, ".ai-rulez.sigstore.json")
	assert.Equal(t, filepath.Join(dir, ".ai-rulez.2.sigstore.json"), NextCosignaturePath(primary))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez.2.sigstore.json"), []byte("{}"), 0o644))
	assert.Equal(t, filepath.Join(dir, ".ai-rulez.3.sigstore.json"), NextCosignaturePath(primary))
}

func TestProvenanceRoundTrip(t *testing.T) {
	builder := newTestSigner(t, SubjectBundle, "")
	dir := writeTree(t, bundleFiles)
	ts, err := ReadTreeSubject(KindBundleTree, dir)
	require.NoError(t, err)
	st, err := ProvenanceStatement(ts, ProvenanceInput{
		BuilderID: "https://github.com/example/plugin/.github/workflows/release.yml@refs/heads/main", Version: "5.0.0",
		Repository: "https://github.com/example/plugin", Ref: "refs/heads/main", Commit: "abc123", InvocationID: "run-1", Now: testNow,
	})
	require.NoError(t, err)
	data, err := SignStatement(context.Background(), builder.signer, st)
	require.NoError(t, err)
	check := &ArtifactCheck{Subject: SubjectBundle, Policy: policyFor(SubjectBundle, 2, builder)}

	rep, prov, err := check.VerifyProvenance([][]byte{data}, ts, nil)

	require.NoError(t, err)
	assert.Equal(t, "https://github.com/example/plugin/.github/workflows/release.yml@refs/heads/main", prov.RunDetails.Builder.ID)
	assert.Equal(t, BuildTypeBundle, prov.BuildDefinition.BuildType)
	require.Len(t, prov.BuildDefinition.ResolvedDependencies, 1)
	assert.Equal(t, "abc123", prov.BuildDefinition.ResolvedDependencies[0].Digest["gitCommit"])
	assert.Equal(t, testNow, rep.SigningTime, "the run's finishedOn stands in for a signing time")
	assert.Equal(t, PredicateSLSA, rep.Statement.PredicateType)

	t.Run("a builder outside the allowlist", func(t *testing.T) {
		_, _, err := check.VerifyProvenance([][]byte{data}, ts, []string{"https://example.org/other-builder"})
		assert.Equal(t, CodeProvenance, CodeOf(err))
	})
	t.Run("the allowlisted builder", func(t *testing.T) {
		_, _, err := check.VerifyProvenance([][]byte{data}, ts, []string{prov.RunDetails.Builder.ID})
		assert.NoError(t, err)
	})
	t.Run("no provenance", func(t *testing.T) {
		_, _, err := check.VerifyProvenance(nil, ts, nil)
		assert.Equal(t, CodeProvenance, CodeOf(err))
	})
	t.Run("provenance of another build", func(t *testing.T) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(`{"name":"evil"}`), 0o644))
		changed, err := ReadTreeSubject(KindBundleTree, dir)
		require.NoError(t, err)
		_, _, err = check.VerifyProvenance([][]byte{data}, changed, nil)
		assert.Equal(t, CodeSubjectMismatch, CodeOf(err))
	})
	t.Run("a bundle attestation is not provenance", func(t *testing.T) {
		attestation := signTree(t, builder, SubjectBundle, dir)
		cur, err := ReadTreeSubject(KindBundleTree, dir)
		require.NoError(t, err)
		_, _, err = check.VerifyProvenance([][]byte{attestation}, cur, nil)
		assert.Equal(t, CodeSubjectMismatch, CodeOf(err))
	})
}

func TestProvenanceSidecarFor(t *testing.T) {
	assert.Equal(t, "d/.ai-rulez.provenance.sigstore.json", ProvenanceSidecarFor("d/.ai-rulez.sigstore.json"))
	assert.Equal(t, "x.provenance.sigstore.json", ProvenanceSidecarFor("x.sigstore.json"))
	assert.Equal(t, "custom.provenance.sigstore.json", ProvenanceSidecarFor("custom"))
}
