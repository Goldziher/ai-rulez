package signing

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sigstore/sigstore/pkg/cryptoutils"
	"github.com/sigstore/sigstore/pkg/signature/kms/fake"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsKMSRef(t *testing.T) {
	file := filepath.Join(t.TempDir(), "cosign.key")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))
	tests := []struct {
		key  string
		want bool
	}{
		{"awskms:///alias/release", true},
		{"gcpkms://projects/p/locations/global/keyRings/r/cryptoKeys/k", true},
		{"azurekms://vault.vault.azure.net/key", true},
		{"hashivault://release", true},
		{"cosign.key", false},
		{"/etc/keys/cosign.key", false},
		{file, false},
		{"", false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, IsKMSRef(tt.key), tt.key)
	}
}

func TestKMSProvidersAreRegistered(t *testing.T) {
	providers := KMSProviders()

	for _, scheme := range []string{"awskms://", "gcpkms://", "azurekms://", "hashivault://"} {
		assert.Contains(t, providers, scheme)
	}
}

// fakeKMSContext makes the fake KMS provider sign with priv, as a stand-in for a
// cloud key: the signer sees a digest and returns a signature, never the key.
func fakeKMSContext(t *testing.T) (context.Context, []byte) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	pubPEM, err := cryptoutils.MarshalPublicKeyToPEM(priv.Public())
	require.NoError(t, err)
	return context.WithValue(context.Background(), fake.KmsCtxKey{}, priv), pubPEM
}

func TestKMSSignerSignsAndVerifiesOffline(t *testing.T) {
	ctx, pubPEM := fakeKMSContext(t)
	signer, err := LoadKMSSigner(ctx, "fakekms://release")
	require.NoError(t, err)
	dir := writeTree(t, bundleFiles)
	st, _, err := TreeStatement(SubjectBundle, dir, ArtifactMeta{Version: "5.0.0", Now: testNow})
	require.NoError(t, err)

	data, err := SignStatement(ctx, signer, st)
	require.NoError(t, err)

	pub, err := ParsePublicKey(pubPEM)
	require.NoError(t, err)
	ts, err := ReadTreeSubject(KindBundleTree, dir)
	require.NoError(t, err)
	policy := policyFor(SubjectBundle, 1, testSigner{pub: TrustEntry{Subject: SubjectBundle, Key: pub}})
	rep, err := VerifyArtifact([][]byte{data}, TreeExpectation(SubjectBundle, ts), policy)
	require.NoError(t, err)
	fp, err := Fingerprint(pub)
	require.NoError(t, err)
	assert.Equal(t, fp, rep.Result.Signer.KeyID, "the bundle names the KMS key by fingerprint, like any key")

	other := newTestSigner(t, SubjectBundle, "")
	_, err = VerifyArtifact([][]byte{data}, TreeExpectation(SubjectBundle, ts), policyFor(SubjectBundle, 1, other))
	assert.Equal(t, CodeSignerNotTrusted, CodeOf(err))
}

func TestLoadKMSSignerRejectsAnUnknownScheme(t *testing.T) {
	_, err := LoadKMSSigner(context.Background(), "nokms://some/key?token=secret")

	require.Error(t, err)
	assert.NotContains(t, err.Error(), "secret", "a query string never reaches an error message")
}

// TestLiveKMSRoundTrip signs with a real cloud KMS key. It needs the provider's
// credentials in the environment, so it runs only when AI_RULEZ_LIVE_KMS=1 and
// AI_RULEZ_LIVE_KMS_KEY names the key URI. Never part of the default run.
func TestLiveKMSRoundTrip(t *testing.T) {
	if os.Getenv("AI_RULEZ_LIVE_KMS") != "1" {
		t.Skip("set AI_RULEZ_LIVE_KMS=1 and AI_RULEZ_LIVE_KMS_KEY=<key URI> to sign with a cloud KMS key")
	}
	ref := os.Getenv("AI_RULEZ_LIVE_KMS_KEY")
	if ref == "" {
		t.Skip("AI_RULEZ_LIVE_KMS_KEY is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	signer, err := LoadKMSSigner(ctx, ref)
	require.NoError(t, err)
	dir := writeTree(t, bundleFiles)
	st, _, err := TreeStatement(SubjectBundle, dir, ArtifactMeta{Version: "live", Now: time.Now()})
	require.NoError(t, err)

	data, err := SignStatement(ctx, signer, st)
	require.NoError(t, err)

	ts, err := ReadTreeSubject(KindBundleTree, dir)
	require.NoError(t, err)
	policy := policyFor(SubjectBundle, 1, testSigner{pub: TrustEntry{Subject: SubjectBundle, Key: signer.Key.GetPublicKey()}})
	policy.Now = time.Now()
	_, err = VerifyArtifact([][]byte{data}, TreeExpectation(SubjectBundle, ts), policy)
	assert.NoError(t, err)
}
