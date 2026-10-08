package signing_test

import (
	"context"
	. "github.com/Goldziher/ai-rulez/v5/internal/signing"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// releaseBinary writes a fake binary and the provenance bundle a release would
// ship for it, signed by signer. It returns the binary path and the public key file.
func releaseBinary(t *testing.T, signer testSigner, content string) (exe, pubFile string) {
	t.Helper()
	dir := t.TempDir()
	exe = filepath.Join(dir, "ai-rulez")
	require.NoError(t, os.WriteFile(exe, []byte(content), 0o755))
	fs, err := ReadFileSubject(exe)
	require.NoError(t, err)
	st, err := NewStatement(PredicateSLSA, []Subject{{Name: "ai-rulez", Digest: map[string]string{"sha256": fs.DigestHex}}}, SLSAProvenance{
		BuildDefinition: SLSABuildDefinition{BuildType: "https://actions.github.io/buildtypes/workflow/v1", ExternalParameters: map[string]any{}},
		RunDetails:      SLSARunDetails{Builder: SLSABuilder{ID: "https://github.com/actions/runner"}, Metadata: SLSAMetadata{FinishedOn: testNow.Format(time.RFC3339)}},
	})
	require.NoError(t, err)
	bundle, err := SignStatement(context.Background(), signer.signer, st)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(exe+".sigstore.json", bundle, 0o644))
	pubFile = filepath.Join(dir, "release.pub")
	require.NoError(t, os.WriteFile(pubFile, signer.pubPEM, 0o644))
	return exe, pubFile
}

func TestVerifySelf(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(t *testing.T, exe string)
		other    bool
		wantCode string
	}{
		{name: "the released binary verifies"},
		{name: "an edited binary is AR724", mutate: func(t *testing.T, exe string) {
			require.NoError(t, os.WriteFile(exe, []byte("patched"), 0o755))
		}, wantCode: CodeSubjectMismatch},
		{name: "an untrusted signer is AR722", other: true, wantCode: CodeSignerNotTrusted},
		{name: "no attestation next to the binary is AR720", mutate: func(t *testing.T, exe string) {
			require.NoError(t, os.Remove(exe+".sigstore.json"))
		}, wantCode: CodeMissing},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			releaseSigner := newTestSigner(t, "", "")
			exe, pubFile := releaseBinary(t, releaseSigner, "released-binary")
			if tt.other {
				pubFile = filepath.Join(t.TempDir(), "other.pub")
				require.NoError(t, os.WriteFile(pubFile, newTestSigner(t, "", "").pubPEM, 0o644))
			}
			if tt.mutate != nil {
				tt.mutate(t, exe)
			}

			// Act
			rep, err := VerifySelf(SelfOptions{Exe: exe, PublicKeys: []string{pubFile}, Now: testNow.Add(time.Hour)})

			// Assert
			if tt.wantCode == "" {
				require.NoError(t, err)
				assert.Equal(t, KindKey, rep.Result.Signer.Kind)
				assert.Equal(t, exe+".sigstore.json", rep.Attestation)
				return
			}
			require.Error(t, err)
			assert.Equal(t, tt.wantCode, CodeOf(err))
		})
	}
}

func TestVerifySelfDefaultsToTheReleaseWorkflowIdentity(t *testing.T) {
	// Arrange: no flags, so the pinned release workflow is the only trusted signer.
	signer := newTestSigner(t, "", "")
	exe, _ := releaseBinary(t, signer, "released-binary")
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	// Act: the pin is a certificate identity, so a log proof is required and a key signature has none.
	_, err := VerifySelf(SelfOptions{Exe: exe, Now: testNow.Add(time.Hour)})

	// Assert
	require.Error(t, err)
	assert.Equal(t, CodeTLogMissing, CodeOf(err), err.Error())
}
