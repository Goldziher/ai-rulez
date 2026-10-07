package signing

import (
	"context"
	"crypto"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestKeylessSignerAgainstFakeSigstore(t *testing.T) {
	const identity, issuer = "https://github.com/acme/repo/.github/workflows/release.yml@refs/heads/main", "https://token.actions.githubusercontent.com"
	payload := []byte(`{"_type":"https://in-toto.io/Statement/v1","subject":[{"name":"x","digest":{"sha256":"` + hexRepeat("ab") + `"}}],"predicateType":"https://example.com/p","predicate":{}}`)

	tests := []struct {
		name       string
		setup      func(f *fakeSigstore)
		wantErr    string
		wantRekor  int
		wantFulcio int
	}{
		{name: "signs, certifies and logs", wantRekor: 1, wantFulcio: 1},
		{name: "retries a transient Rekor failure", setup: func(f *fakeSigstore) { f.rekorFailures = 2 }, wantRekor: 3, wantFulcio: 1},
		{name: "gives up after the retries", setup: func(f *fakeSigstore) { f.rekorFailures = 5 }, wantErr: "Rekor answered HTTP 503", wantRekor: 3, wantFulcio: 1},
		{name: "fetches the existing entry on a conflict", setup: func(f *fakeSigstore) { f.conflict = true }, wantRekor: 2, wantFulcio: 1},
		{name: "refuses a log entry of another kind", setup: func(f *fakeSigstore) { f.wrongKind = true }, wantErr: "not the \"dsse\" entry", wantRekor: 1, wantFulcio: 1},
		{name: "refuses a certificate for another key", setup: func(f *fakeSigstore) { f.wrongCertKey = true }, wantErr: "certificate for a different key", wantFulcio: 1},
		{name: "a token Fulcio refuses stops before the log", setup: func(f *fakeSigstore) { f.token += "x" }, wantErr: "Fulcio answered HTTP 401", wantFulcio: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			f := newFakeSigstore(t, identity, issuer)
			signer := f.signer()
			if tt.setup != nil {
				tt.setup(f)
			}

			// Act
			pb, err := signer.Bundle(context.Background(), &DSSEData{Data: payload, PayloadType: PayloadTypeInToto})

			// Assert
			assert.Equal(t, tt.wantRekor, f.rekorCalls, "rekor calls")
			assert.Equal(t, tt.wantFulcio, f.fulcioCalls, "fulcio calls")
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.True(t, f.lastProofSubjectVerified)
			data, err := protojson.Marshal(pb)
			require.NoError(t, err)
			res, err := (&Verifier{TrustedRoot: f.trustedRoot(), TLog: TLogRequired}).Verify(data)
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
	f := newFakeSigstore(t, "dev@example.org", "https://accounts.example.org")
	artifact := []byte("a release archive")

	// Act
	pb, err := f.signer().Bundle(context.Background(), &PlainData{Data: artifact})

	// Assert
	require.NoError(t, err)
	assert.Contains(t, f.lastProposed, `"kind":"hashedrekord"`)
	assert.Equal(t, "hashedrekord", pb.GetVerificationMaterial().GetTlogEntries()[0].GetKindVersion().GetKind())
	data, err := protojson.Marshal(pb)
	require.NoError(t, err)
	res, err := (&Verifier{TrustedRoot: f.trustedRoot(), TLog: TLogRequired}).VerifyBlob(data, artifact)
	require.NoError(t, err)
	assert.Equal(t, "dev@example.org", res.Signer.Identity)
	_, err = (&Verifier{TrustedRoot: f.trustedRoot(), TLog: TLogRequired}).VerifyBlob(data, []byte("another archive"))
	assert.Error(t, err)
}

func TestKeySignerWithTransparencyLog(t *testing.T) {
	// Arrange
	f := newFakeSigstore(t, "unused", "unused")
	priv, pub, err := GenerateKeyPair(nil)
	require.NoError(t, err)
	ks, err := LoadKeySigner(priv, nil)
	require.NoError(t, err)
	ks.TLog, ks.RekorURL = true, f.rekor.URL
	statement, err := NewStatement("https://example.com/p/v1", []Subject{{Name: "a", Digest: map[string]string{"sha256": hexRepeat("cd")}}}, map[string]int{"n": 1})
	require.NoError(t, err)

	// Act
	data, err := SignStatement(context.Background(), ks, statement)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 1, f.rekorCalls)
	assert.Equal(t, 0, f.fulcioCalls)
	key, err := ParsePublicKey(pub)
	require.NoError(t, err)
	res, err := (&Verifier{TrustedRoot: f.trustedRoot(), Keys: []crypto.PublicKey{key}, TLog: TLogRequired}).Verify(data)
	require.NoError(t, err)
	assert.Equal(t, KindKey, res.Signer.Kind)
	assert.True(t, res.Logged)
}

func TestFulcioRequestNeverFollowsARedirect(t *testing.T) {
	// Arrange: the token must reach only the configured Fulcio host.
	var elsewhere int
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { elsewhere++ }))
	t.Cleanup(other.Close)
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/api/v2/signingCert", http.StatusTemporaryRedirect)
	}))
	t.Cleanup(redirect.Close)
	kp, err := newEphemeralKeyPair()
	require.NoError(t, err)
	claims, _ := json.Marshal(map[string]string{"sub": "me"})
	token := "e30." + base64.RawURLEncoding.EncodeToString(claims) + ".c2ln"

	// Act
	_, err = fulcioClient{newServiceClient(redirect.URL)}.certificate(context.Background(), kp, token)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 307")
	assert.Zero(t, elsewhere)
}

func TestFulcioRejectsMalformedTokensAndAnswers(t *testing.T) {
	kp, err := newEphemeralKeyPair()
	require.NoError(t, err)
	claims, _ := json.Marshal(map[string]string{"sub": "me"})
	good := "e30." + base64.RawURLEncoding.EncodeToString(claims) + ".c2ln"
	tests := []struct {
		name    string
		token   string
		answer  string
		wantErr string
	}{
		{"not a JWT", "opaque", "", "not a JWT"},
		{"claims not base64", "a.%%%.b", "", "not a JWT"},
		{"no subject", "e30.e30.c2ln", "", "subject"},
		{"answer not JSON", good, "nope", "not JSON"},
		{"no certificate", good, `{"signedCertificateEmbeddedSct":{"chain":{"certificates":[]}}}`, "no certificate"},
		{"certificate not PEM", good, `{"signedCertificateDetachedSct":{"chain":{"certificates":["garbage"]}}}`, "not PEM"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(tt.answer)) }))
			t.Cleanup(srv.Close)

			// Act
			_, err := fulcioClient{newServiceClient(srv.URL)}.certificate(context.Background(), kp, tt.token)

			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestParseRekorEntryRejectsIncompleteAnswers(t *testing.T) {
	body := base64.StdEncoding.EncodeToString([]byte(`{"kind":"dsse","apiVersion":"0.0.1"}`))
	proof := `"inclusionProof":{"checkpoint":"c","hashes":["00"],"logIndex":0,"rootHash":"00","treeSize":1}`
	tests := []struct {
		name    string
		answer  string
		wantErr string
	}{
		{"not JSON", `nope`, "unexpected response"},
		{"two entries", `{"a":{},"b":{}}`, "unexpected response"},
		{"no time", `{"u":{"body":"` + body + `","logID":"00","logIndex":1,"verification":{` + proof + `}}}`, "without its time"},
		{"no proof", `{"u":{"body":"` + body + `","integratedTime":1,"logID":"00","logIndex":1,"verification":{"signedEntryTimestamp":"AA=="}}}`, "without an inclusion proof"},
		{"bad log ID", `{"u":{"body":"` + body + `","integratedTime":1,"logID":"zz","logIndex":1,"verification":{` + proof + `}}}`, "malformed entry"},
		{"bad proof hash", `{"u":{"body":"` + body + `","integratedTime":1,"logID":"00","logIndex":1,"verification":{"inclusionProof":{"hashes":["zz"],"logIndex":0,"rootHash":"00","treeSize":1}}}}`, "malformed inclusion proof"},
		{"incomplete proof", `{"u":{"body":"` + body + `","integratedTime":1,"logID":"00","logIndex":1,"verification":{"inclusionProof":{"rootHash":"00"}}}}`, "incomplete inclusion proof"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			_, err := parseRekorEntry([]byte(tt.answer), "dsse")

			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestParseRekorEntryConvertsTheProof(t *testing.T) {
	// Arrange
	body := base64.StdEncoding.EncodeToString([]byte(`{"kind":"dsse","apiVersion":"0.0.1"}`))
	answer := `{"u":{"body":"` + body + `","integratedTime":1700000000,"logID":"0a0b","logIndex":42,"verification":{"signedEntryTimestamp":"AQI=","inclusionProof":{"checkpoint":"cp","hashes":["0c","0d"],"logIndex":7,"rootHash":"0e","treeSize":9}}}}`

	// Act
	tle, err := parseRekorEntry([]byte(answer), "dsse")

	// Assert
	require.NoError(t, err)
	assert.Equal(t, int64(42), tle.GetLogIndex())
	assert.Equal(t, []byte{0x0a, 0x0b}, tle.GetLogId().GetKeyId())
	assert.Equal(t, int64(1700000000), tle.GetIntegratedTime())
	assert.Equal(t, "0.0.1", tle.GetKindVersion().GetVersion())
	assert.Equal(t, []byte{1, 2}, tle.GetInclusionPromise().GetSignedEntryTimestamp())
	assert.Equal(t, int64(7), tle.GetInclusionProof().GetLogIndex())
	assert.Equal(t, int64(9), tle.GetInclusionProof().GetTreeSize())
	assert.Equal(t, [][]byte{{0x0c}, {0x0d}}, tle.GetInclusionProof().GetHashes())
	assert.Equal(t, "cp", tle.GetInclusionProof().GetCheckpoint().GetEnvelope())
}

func TestRekorConflictNeedsAValidLocation(t *testing.T) {
	// Arrange
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "https://elsewhere.example/../../etc/passwd")
		w.WriteHeader(http.StatusConflict)
	}))
	t.Cleanup(srv.Close)
	kp, err := newEphemeralKeyPair()
	require.NoError(t, err)
	r := &rekorClient{newServiceClient(srv.URL)}

	// Act
	_, err = buildBundle(context.Background(), &DSSEData{Data: []byte("{}"), PayloadType: PayloadTypeInToto}, kp, nil, "", r)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "without a valid location")
}

func TestServiceClientHonoursCancellationDuringBackoff(t *testing.T) {
	// Arrange
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTooManyRequests) }))
	t.Cleanup(srv.Close)
	c := newServiceClient(srv.URL)
	c.backoff = time.Hour
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Act
	_, err := c.do(ctx, func() (*http.Request, error) { return http.NewRequest(http.MethodGet, srv.URL, nil) })

	// Assert
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}
