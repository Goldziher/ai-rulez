package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/samber/oops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/sbom"
	"github.com/Goldziher/ai-rulez/v5/internal/signing"
)

func TestSBOMVerifyRecordsTheLockAttestation(t *testing.T) {
	tests := []struct {
		name       string
		sign       bool
		tamper     bool
		wantStatus string
		wantSigner bool
		wantCode   string
	}{
		{"signed", true, false, sbom.SignatureVerified, true, ""},
		{"no attestation", false, false, sbom.SignatureAbsent, false, ""},
		{"edited bundle", true, true, sbom.SignatureInvalid, false, "AR72"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			f := newSignFixture(t, signingKeyTable)
			if tt.sign {
				require.Equal(t, 0, f.sign(t))
			}
			if tt.tamper {
				require.NoError(t, os.WriteFile(f.bundle(), []byte(`{"mediaType":"x"}`), 0o600))
			}

			// Act
			var out, errOut bytes.Buffer
			code := codeOf(runSBOM(&out, &errOut, sbomFlags{docType: "cyclonedx", verify: true}, false))

			// Assert
			require.Equal(t, 0, code, errOut.String())
			var doc struct {
				Metadata struct {
					Component struct {
						Properties []sbom.Property `json:"properties"`
					} `json:"component"`
				} `json:"metadata"`
			}
			require.NoError(t, json.Unmarshal(out.Bytes(), &doc))
			props := map[string]string{}
			for _, p := range doc.Metadata.Component.Properties {
				props[p.Name] = p.Value
			}
			assert.Equal(t, tt.wantStatus, props["ai-rulez:signature"])
			assert.Equal(t, tt.wantSigner, props["ai-rulez:signer"] != "")
			if tt.wantCode != "" {
				assert.Contains(t, props["ai-rulez:signature-code"], tt.wantCode)
			}
		})
	}
}

func TestSBOMVerifyWithoutATrustedSignerCouldNotRun(t *testing.T) {
	// Arrange
	f := newSignFixture(t, "")
	require.NoError(t, os.Remove(f.pubKey))

	// Act
	var out, errOut bytes.Buffer
	code := codeOf(runSBOM(&out, &errOut, sbomFlags{docType: "cyclonedx", verify: true}, false))

	// Assert
	assert.Equal(t, 1, code)
	assert.Empty(t, out.String())
}

func TestSignatureFailureMapsTheCode(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus string
		wantCode   string
	}{
		{"missing", signing.Errorf(signing.CodeMissing, "no attestation"), sbom.SignatureAbsent, ""},
		{"untrusted", signing.Errorf(signing.CodeSignerNotTrusted, "who?"), sbom.SignatureInvalid, signing.CodeSignerNotTrusted},
		{"not a signing error", oops.Errorf("boom"), sbom.SignatureInvalid, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := signatureFailure(tt.err)

			// Assert
			assert.Equal(t, tt.wantStatus, got.Status)
			assert.Equal(t, tt.wantCode, got.Code)
		})
	}
}
