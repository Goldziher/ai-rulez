package sigstore_test

import (
	"context"
	"crypto"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	. "github.com/Goldziher/ai-rulez/v5/internal/signing"
	. "github.com/Goldziher/ai-rulez/v5/internal/signing/sigstore"
	"github.com/Goldziher/ai-rulez/v5/internal/signing/sigstore/fakesigstore"
)

func TestMain(m *testing.M) {
	UseBackend(New())
	os.Exit(m.Run())
}

func TestKeylessSignerAgainstFakeSigstore(t *testing.T) {
	const identity, issuer = "https://github.com/acme/repo/.github/workflows/release.yml@refs/heads/main", "https://token.actions.githubusercontent.com"
	payload := []byte(`{"_type":"https://in-toto.io/Statement/v1","subject":[{"name":"x","digest":{"sha256":"` + strings.Repeat("ab", 32) + `"}}],"predicateType":"https://example.com/p","predicate":{}}`)

	tests := []struct {
		name       string
		setup      func(f *fakesigstore.Fake)
		wantErr    string
		wantRekor  int
		wantFulcio int
	}{
		{name: "signs, certifies and logs", wantRekor: 1, wantFulcio: 1},
		{name: "retries a transient Rekor failure", setup: func(f *fakesigstore.Fake) { f.RekorFailures = 2 }, wantRekor: 3, wantFulcio: 1},
		{name: "gives up after the retries", setup: func(f *fakesigstore.Fake) { f.RekorFailures = 5 }, wantErr: "Rekor answered HTTP 503", wantRekor: 3, wantFulcio: 1},
		{name: "fetches the existing entry on a conflict", setup: func(f *fakesigstore.Fake) { f.Conflict = true }, wantRekor: 2, wantFulcio: 1},
		{name: "refuses a log entry of another kind", setup: func(f *fakesigstore.Fake) { f.WrongKind = true }, wantErr: "not the \"dsse\" entry", wantRekor: 1, wantFulcio: 1},
		{name: "refuses a certificate for another key", setup: func(f *fakesigstore.Fake) { f.WrongCertKey = true }, wantErr: "certificate for a different key", wantFulcio: 1},
		{name: "a token Fulcio refuses stops before the log", setup: func(f *fakesigstore.Fake) { f.Token += "x" }, wantErr: "Fulcio answered HTTP 401", wantFulcio: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			f := fakesigstore.New(t, identity, issuer)
			signer := f.Signer()
			if tt.setup != nil {
				tt.setup(f)
			}

			// Act
			pb, err := signer.Bundle(context.Background(), &DSSEData{Data: payload, PayloadType: PayloadTypeInToto})

			// Assert
			assert.Equal(t, tt.wantRekor, f.RekorCalls, "rekor calls")
			assert.Equal(t, tt.wantFulcio, f.FulcioCalls, "fulcio calls")
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.True(t, f.LastProofSubjectVerified)
			data, err := protojson.Marshal(pb)
			require.NoError(t, err)
			res, err := (&Verifier{TrustedRoot: f.TrustedRoot(), TLog: TLogRequired}).Verify(data)
			require.NoError(t, err)
			assert.Equal(t, KindKeyless, res.Signer.Kind)
			assert.Equal(t, identity, res.Signer.Identity)
			assert.Equal(t, issuer, res.Signer.Issuer)
			assert.True(t, res.Logged)
			assert.Equal(t, payload, res.Payload)
		})
	}
}

func TestKeylessSignerLogsAMessageSignature(t *testing.T) {
	// Arrange
	f := fakesigstore.New(t, "dev@example.org", "https://accounts.example.org")
	artifact := []byte("a release archive")

	// Act
	pb, err := f.Signer().Bundle(context.Background(), &PlainData{Data: artifact})

	// Assert
	require.NoError(t, err)
	assert.Contains(t, f.LastProposed, `"kind":"hashedrekord"`)
	assert.Equal(t, "hashedrekord", pb.GetVerificationMaterial().GetTlogEntries()[0].GetKindVersion().GetKind())
	data, err := protojson.Marshal(pb)
	require.NoError(t, err)
	res, err := (&Verifier{TrustedRoot: f.TrustedRoot(), TLog: TLogRequired}).VerifyBlob(data, artifact)
	require.NoError(t, err)
	assert.Equal(t, "dev@example.org", res.Signer.Identity)
	_, err = (&Verifier{TrustedRoot: f.TrustedRoot(), TLog: TLogRequired}).VerifyBlob(data, []byte("another archive"))
	assert.Error(t, err)
}

func TestKeySignerWithTransparencyLog(t *testing.T) {
	// Arrange
	f := fakesigstore.New(t, "unused", "unused")
	priv, pub, err := GenerateKeyPair(nil)
	require.NoError(t, err)
	ks, err := LoadKeySigner(priv, nil)
	require.NoError(t, err)
	ks.TLog, ks.RekorURL = true, f.Rekor.URL
	statement, err := NewStatement("https://example.com/p/v1", []Subject{{Name: "a", Digest: map[string]string{"sha256": strings.Repeat("cd", 32)}}}, map[string]int{"n": 1})
	require.NoError(t, err)

	// Act
	data, err := SignStatement(context.Background(), ks, statement)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 1, f.RekorCalls)
	assert.Equal(t, 0, f.FulcioCalls)
	key, err := ParsePublicKey(pub)
	require.NoError(t, err)
	res, err := (&Verifier{TrustedRoot: f.TrustedRoot(), Keys: []crypto.PublicKey{key}, TLog: TLogRequired}).Verify(data)
	require.NoError(t, err)
	assert.Equal(t, KindKey, res.Signer.Kind)
	assert.True(t, res.Logged)
}
