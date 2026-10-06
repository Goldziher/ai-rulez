package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/signing"
)

// selfFixture is a fake release binary with the provenance bundle its release ships.
type selfFixture struct{ exe, pub string }

func newSelfFixture(t *testing.T) selfFixture {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	resetSigningFlags(t)
	t.Cleanup(func() { verifySelf = false })
	dir := t.TempDir()
	exe := filepath.Join(dir, "ai-rulez")
	require.NoError(t, os.WriteFile(exe, []byte("release-binary"), 0o755))
	fs, err := signing.ReadFileSubject(exe)
	require.NoError(t, err)
	priv, pubPEM, err := signing.GenerateKeyPair(nil)
	require.NoError(t, err)
	ks, err := signing.LoadKeySigner(priv, nil)
	require.NoError(t, err)
	st, err := signing.NewStatement(signing.PredicateSLSA, []signing.Subject{{Name: "ai-rulez", Digest: map[string]string{"sha256": fs.DigestHex}}}, signing.SLSAProvenance{
		BuildDefinition: signing.SLSABuildDefinition{BuildType: "https://actions.github.io/buildtypes/workflow/v1", ExternalParameters: map[string]any{}},
		RunDetails:      signing.SLSARunDetails{Metadata: signing.SLSAMetadata{FinishedOn: time.Now().UTC().Format(time.RFC3339)}},
	})
	require.NoError(t, err)
	bundle, err := signing.SignStatement(context.Background(), ks, st)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(exe+".sigstore.json", bundle, 0o644))
	pub := filepath.Join(dir, "release.pub")
	require.NoError(t, os.WriteFile(pub, pubPEM, 0o644))
	return selfFixture{exe: exe, pub: pub}
}

func TestRunVerifySelf(t *testing.T) {
	t.Run("an official build verifies and reports the signer", func(t *testing.T) {
		// Arrange
		f := newSelfFixture(t)
		verifyPublicKeys, verifyFormat = []string{f.pub}, formatJSON
		var out bytes.Buffer

		// Act
		code := runVerifySelf(f.exe, nil, &out)

		// Assert
		require.Equal(t, 0, code)
		var rep attestationReport
		require.NoError(t, json.Unmarshal(out.Bytes(), &rep))
		require.Len(t, rep.Results, 1)
		assert.Equal(t, "release", rep.Results[0].Subject)
		assert.Equal(t, attestationValid, rep.Results[0].Status)
	})
	t.Run("an edited binary fails with drift", func(t *testing.T) {
		// Arrange
		f := newSelfFixture(t)
		verifyPublicKeys, verifyFormat = []string{f.pub}, formatJSON
		require.NoError(t, os.WriteFile(f.exe, []byte("patched"), 0o755))
		var out bytes.Buffer

		// Act
		code := runVerifySelf(f.exe, nil, &out)

		// Assert
		assert.Equal(t, exitDrift, code)
		assert.Contains(t, out.String(), "AR724")
	})
	t.Run("a missing binary could not be checked", func(t *testing.T) {
		// Arrange
		newSelfFixture(t)

		// Act
		code := runVerifySelf(filepath.Join(t.TempDir(), "nope"), nil, &bytes.Buffer{})

		// Assert
		assert.Equal(t, 1, code)
	})
}
