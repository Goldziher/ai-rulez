package publish

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/publish/oci"
)

func TestBuild_SignsAReleaseAttestationBindingNameVersionAndDigests(t *testing.T) {
	// Arrange
	signer, trust := keyPair(t)
	in := signedInput(t, signer)
	in.SBOM = []byte(`{"bomFormat":"CycloneDX","specVersion":"1.6"}` + "\n")

	// Act
	d, err := Build(in)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, d.Manifest.Signature)
	assert.Equal(t, "acme-1.4.0.attestation.sigstore.json", d.Manifest.Signature.Attestation)
	assert.Contains(t, string(d.Files[SumsFile]), "acme-1.4.0.attestation.sigstore.json")
	schemaValid(t, "publish-manifest.schema.json", d.Files["acme-1.4.0.manifest.json"])
	files := ReleaseFiles{Archive: d.Files["acme-1.4.0.tar.gz"], Lock: d.Files[LockFile], SBOM: d.Files["acme-1.4.0.sbom.cdx.json"]}
	_, err = VerifyReleaseAttestation(d.Files["acme-1.4.0.attestation.sigstore.json"], d.Manifest, files, trust)
	require.NoError(t, err)
}

func TestVerifyReleaseAttestation_RejectsARelabelledOrSwappedRelease(t *testing.T) {
	signer, trust := keyPair(t)
	_, otherTrust := keyPair(t)
	in := signedInput(t, signer)
	in.SBOM = []byte(`{"bomFormat":"CycloneDX"}` + "\n")
	d, err := Build(in)
	require.NoError(t, err)
	bundle := d.Files["acme-1.4.0.attestation.sigstore.json"]
	good := ReleaseFiles{Archive: d.Files["acme-1.4.0.tar.gz"], Lock: d.Files[LockFile], SBOM: d.Files["acme-1.4.0.sbom.cdx.json"]}

	tests := []struct {
		name   string
		mutate func(m *Manifest, f *ReleaseFiles)
		trust  VerifyOptions
		want   string
	}{
		{"another version", func(m *Manifest, _ *ReleaseFiles) { m.Version = "1.4.1" }, trust, "attestation signs acme 1.4.0"},
		{"another name", func(m *Manifest, _ *ReleaseFiles) { m.Name = "other" }, trust, "attestation signs acme 1.4.0"},
		{"another archive", func(_ *Manifest, f *ReleaseFiles) { f.Archive = []byte("old release") }, trust, "attestation signs"},
		{"another lock", func(_ *Manifest, f *ReleaseFiles) { f.Lock = []byte("version = 1\n") }, trust, "attestation signs"},
		{"another sbom", func(_ *Manifest, f *ReleaseFiles) { f.SBOM = []byte("{}") }, trust, "attestation signs"},
		{"sbom dropped from the manifest", func(m *Manifest, _ *ReleaseFiles) { m.SBOM = nil }, trust, "SBOM the manifest does not name"},
		{"an untrusted signer", func(*Manifest, *ReleaseFiles) {}, otherTrust, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, f := d.Manifest, good
			tt.mutate(&m, &f)

			_, err := VerifyReleaseAttestation(bundle, m, f, tt.trust)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestVerifyWith_ASignedReleaseNeedsItsAttestation(t *testing.T) {
	signer, trust := keyPair(t)
	dir, _ := writeBuilt(t, signedInput(t, signer))
	manifestPath := filepath.Join(dir, "acme-1.4.0.manifest.json")
	replaceIn(t, manifestPath, `"attestation": "acme-1.4.0.attestation.sigstore.json",`, "")

	res, err := VerifyWith(dir, VerifyChecks{Signature: trust})

	require.NoError(t, err)
	require.False(t, res.OK())
	assert.Contains(t, strings.Join(problemPaths(res), "\n"), "no signed attestation")
	assert.Equal(t, "unverified", res.Signature)
}

func TestVerifyWith_ASwappedAttestationFails(t *testing.T) {
	signer, trust := keyPair(t)
	dir, _ := writeBuilt(t, signedInput(t, signer))
	otherIn := signedInput(t, signer)
	otherIn.Version = "1.5.0"
	other, err := Build(otherIn)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "acme-1.4.0.attestation.sigstore.json"), other.Files["acme-1.5.0.attestation.sigstore.json"], 0o600))

	res, err := VerifyWith(dir, VerifyChecks{Signature: trust})

	require.NoError(t, err)
	require.False(t, res.OK())
	assert.Contains(t, strings.Join(problemPaths(res), "\n"), "attestation does not verify")
}

func TestVerify_TheArchiveMustNameTheManifestsPluginAndVersion(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		want string
	}{
		{"matching", `{"name":"acme","version":"1.4.0"}`, ""},
		{"another plugin", `{"name":"other","version":"1.4.0"}`, `names the plugin "other"`},
		{"an older version", `{"name":"acme","version":"1.3.0"}`, `is version "1.3.0"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := committedInput()
			for i := range in.Files {
				if in.Files[i].Path == ".claude-plugin/plugin.json" {
					in.Files[i].Data = []byte(tt.doc)
				}
			}
			dir, _ := writeBuilt(t, in)

			res, err := Verify(dir)

			require.NoError(t, err)
			if tt.want == "" {
				assert.True(t, res.OK(), "%v", res.Problems)
				return
			}
			require.False(t, res.OK())
			assert.Contains(t, strings.Join(problemPaths(res), "\n"), tt.want)
		})
	}
}

func TestOCI_ASignedReleaseVerifiesAfterAPullThroughTheRegistry(t *testing.T) {
	// Arrange: push a signed artifact, then pull it by digest into a fresh directory.
	host := newRegistry(t)
	signer, trust := keyPair(t)
	in := ociInput(host + "/acme/skills/acme")
	in.Sign = signedInput(t, signer).Sign
	dir, d := writeBuilt(t, in)
	_, err := ExecuteOCI(context.Background(), d.Plan, OCIExecuteOptions{Dir: dir})
	require.NoError(t, err)
	pulled := t.TempDir()

	// Act
	digest, err := PullOCI(context.Background(), oci.Target{Ref: host + "/acme/skills/acme@" + d.Plan.OCIDigest}, pulled)
	require.NoError(t, err)
	res, err := VerifyWith(pulled, VerifyChecks{Signature: trust})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, d.Plan.OCIDigest, digest)
	assert.True(t, res.OK(), "%v", res.Problems)
	assert.Equal(t, "verified", res.Signature)
}
