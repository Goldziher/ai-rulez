package signing

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
)

// fakeSigstore is an in-process Fulcio and Rekor v1 that speak the HTTP APIs the
// keyless signer uses, plus the trusted root that verifies what they issue. The
// leaf certificates carry no signed certificate timestamp, so the root lists no
// CT log.
type fakeSigstore struct {
	t                        *testing.T
	identity, issuer, token  string
	caKey, logKey            *ecdsa.PrivateKey
	caCert                   *x509.Certificate
	logID                    []byte
	fulcio, rekor            *httptest.Server
	mu                       sync.Mutex
	entries                  map[string][]byte // uuid -> response entry JSON
	fulcioCalls, rekorCalls  int
	rekorFailures            int  // answer HTTP 503 this many times first
	conflict                 bool // answer HTTP 409 and serve the entry by UUID
	wrongKind, wrongCertKey  bool
	lastAuth, lastProposed   string
	lastProofSubjectVerified bool
}

func newFakeSigstore(t *testing.T, identity, issuer string) *fakeSigstore {
	t.Helper()
	f := &fakeSigstore{t: t, identity: identity, issuer: issuer, entries: map[string][]byte{}}
	var err error
	f.caKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	f.logKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "fake fulcio"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, f.caKey.Public(), f.caKey)
	require.NoError(t, err)
	f.caCert, err = x509.ParseCertificate(der)
	require.NoError(t, err)
	logDER, err := x509.MarshalPKIXPublicKey(f.logKey.Public())
	require.NoError(t, err)
	sum := sha256.Sum256(logDER)
	f.logID = sum[:]
	claims, err := json.Marshal(map[string]string{"sub": identity, "iss": issuer})
	require.NoError(t, err)
	f.token = "e30." + base64.RawURLEncoding.EncodeToString(claims) + ".c2ln"
	f.fulcio = httptest.NewServer(http.HandlerFunc(f.serveFulcio))
	f.rekor = httptest.NewServer(http.HandlerFunc(f.serveRekor))
	t.Cleanup(f.fulcio.Close)
	t.Cleanup(f.rekor.Close)
	return f
}

// signer is a keyless signer pointed at the fake services, with no retry delay.
func (f *fakeSigstore) signer() *KeylessSigner {
	f.t.Helper()
	s, err := NewKeylessSigner(KeylessOptions{IDToken: f.token, FulcioURL: f.fulcio.URL, RekorURL: f.rekor.URL})
	require.NoError(f.t, err)
	s.backoff = time.Millisecond
	return s
}

// bundle signs payload as an in-toto DSSE envelope and returns the bundle JSON.
func (f *fakeSigstore) bundle(payload []byte) []byte {
	f.t.Helper()
	pb, err := f.signer().Bundle(context.Background(), &DSSEData{Data: payload, PayloadType: PayloadTypeInToto})
	require.NoError(f.t, err)
	data, err := protojson.Marshal(pb)
	require.NoError(f.t, err)
	return data
}

// trustedRoot verifies the fake's certificates and log entries.
func (f *fakeSigstore) trustedRoot() root.TrustedMaterial {
	return &fakeRoot{
		ca: &root.FulcioCertificateAuthority{Root: f.caCert, ValidityPeriodStart: f.caCert.NotBefore, ValidityPeriodEnd: f.caCert.NotAfter},
		logs: map[string]*root.TransparencyLog{hex.EncodeToString(f.logID): {
			BaseURL: f.rekor.URL, ID: f.logID, ValidityPeriodStart: time.Now().Add(-time.Hour),
			HashFunc: crypto.SHA256, PublicKey: f.logKey.Public(), SignatureHashFunc: crypto.SHA256,
		}},
	}
}

type fakeRoot struct {
	root.BaseTrustedMaterial
	ca   root.CertificateAuthority
	logs map[string]*root.TransparencyLog
}

func (r *fakeRoot) FulcioCertificateAuthorities() []root.CertificateAuthority {
	return []root.CertificateAuthority{r.ca}
}

func (r *fakeRoot) RekorLogs() map[string]*root.TransparencyLog { return r.logs }

func (f *fakeSigstore) serveFulcio(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fulcioCalls++
	f.lastAuth = r.Header.Get("Authorization")
	if r.Method != http.MethodPost || r.URL.Path != "/api/v2/signingCert" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if f.lastAuth != "Bearer "+f.token {
		http.Error(w, "bad token", http.StatusUnauthorized)
		return
	}
	var req fulcioRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	pub, err := ParsePublicKey([]byte(req.PublicKeyRequest.PublicKey.Content))
	if err != nil {
		http.Error(w, "bad key", http.StatusBadRequest)
		return
	}
	proof, err := base64.StdEncoding.DecodeString(req.PublicKeyRequest.ProofOfPossession)
	digest := sha256.Sum256([]byte(f.identity))
	ecPub, ok := pub.(*ecdsa.PublicKey)
	f.lastProofSubjectVerified = err == nil && ok && ecdsa.VerifyASN1(ecPub, digest[:], proof)
	if !f.lastProofSubjectVerified {
		http.Error(w, "bad proof of possession", http.StatusBadRequest)
		return
	}
	certPub := pub
	if f.wrongCertKey {
		other, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		certPub = other.Public()
	}
	issuerExt, _ := asn1.Marshal(f.issuer)
	leaf := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()), EmailAddresses: []string{f.identity},
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(10 * time.Minute),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning},
		ExtraExtensions: []pkix.Extension{
			{Id: asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 57264, 1, 1}, Value: []byte(f.issuer)},
			{Id: asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 57264, 1, 8}, Value: issuerExt},
		},
	}
	der, err := x509.CreateCertificate(rand.Reader, leaf, f.caCert, certPub, f.caKey)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	chain := []string{
		string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: f.caCert.Raw})),
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"signedCertificateEmbeddedSct": map[string]any{"chain": map[string]any{"certificates": chain}}})
}

func (f *fakeSigstore) serveRekor(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rekorCalls++
	const prefix = "/api/v1/log/entries"
	switch {
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, prefix+"/"):
		uuid := strings.TrimPrefix(r.URL.Path, prefix+"/")
		entry, ok := f.entries[uuid]
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, map[string]json.RawMessage{uuid: entry})
		return
	case r.Method != http.MethodPost || r.URL.Path != prefix:
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if f.rekorFailures > 0 {
		f.rekorFailures--
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	raw, _ := io.ReadAll(r.Body)
	f.lastProposed = string(raw)
	body, err := canonicalEntry(raw, f.wrongKind)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	index := int64(len(f.entries))
	integrated := time.Now().Unix()
	b64 := base64.StdEncoding.EncodeToString(body)
	logID := hex.EncodeToString(f.logID)
	set, err := f.signSET(b64, integrated, logID, index)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Each entry is the only leaf of its own tree: the root hash is the leaf hash
	// and the proof has no hashes.
	leaf := sha256.Sum256(append([]byte{0}, body...))
	checkpoint, err := f.signCheckpoint(leaf[:])
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	entry, _ := json.Marshal(map[string]any{
		"body": b64, "integratedTime": integrated, "logID": logID, "logIndex": index,
		"verification": map[string]any{
			"signedEntryTimestamp": set,
			"inclusionProof": map[string]any{
				"checkpoint": checkpoint, "hashes": []string{}, "logIndex": 0,
				"rootHash": hex.EncodeToString(leaf[:]), "treeSize": 1,
			},
		},
	})
	uuid := hex.EncodeToString(leaf[:])
	f.entries[uuid] = entry
	if f.conflict {
		w.Header().Set("Location", prefix+"/"+uuid)
		http.Error(w, "an equivalent entry already exists", http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]json.RawMessage{uuid: entry})
}

// signCheckpoint returns a Rekor v1 signed tree head for a one-leaf tree.
func (f *fakeSigstore) signCheckpoint(rootHash []byte) (string, error) {
	const origin = "rekor.test - 1"
	body := fmt.Sprintf("%s\n1\n%s\n", origin, base64.StdEncoding.EncodeToString(rootHash))
	sum := sha256.Sum256([]byte(body))
	sig, err := ecdsa.SignASN1(rand.Reader, f.logKey, sum[:])
	if err != nil {
		return "", err
	}
	der, err := x509.MarshalPKIXPublicKey(f.logKey.Public())
	if err != nil {
		return "", err
	}
	keyHash := sha256.Sum256(der)
	stamped := append(keyHash[:4:4], sig...)
	return fmt.Sprintf("%s\n\u2014 rekor.test %s\n", body, base64.StdEncoding.EncodeToString(stamped)), nil
}

// signSET signs the inclusion promise: the canonical JSON (keys sorted) of the
// body, integrated time, log ID and log index.
func (f *fakeSigstore) signSET(body string, integrated int64, logID string, index int64) ([]byte, error) {
	payload, err := json.Marshal(struct {
		Body           string `json:"body"`
		IntegratedTime int64  `json:"integratedTime"`
		LogID          string `json:"logID"`
		LogIndex       int64  `json:"logIndex"`
	}{body, integrated, logID, index})
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(payload)
	return ecdsa.SignASN1(rand.Reader, f.logKey, sum[:])
}

// canonicalEntry turns a proposed entry into the body Rekor logs: a dsse entry
// keeps the envelope and payload hashes and the signature with its verifier; a
// hashedrekord entry is logged as proposed.
func canonicalEntry(raw []byte, wrongKind bool) ([]byte, error) {
	var proposed struct {
		Kind       string          `json:"kind"`
		APIVersion string          `json:"apiVersion"`
		Spec       json.RawMessage `json:"spec"`
	}
	if err := json.Unmarshal(raw, &proposed); err != nil {
		return nil, err
	}
	if wrongKind {
		proposed.Kind = "rekord"
	}
	if proposed.Kind != "dsse" {
		return json.Marshal(proposed)
	}
	var spec struct {
		ProposedContent struct {
			Envelope  string   `json:"envelope"`
			Verifiers [][]byte `json:"verifiers"`
		} `json:"proposedContent"`
	}
	if err := json.Unmarshal(proposed.Spec, &spec); err != nil {
		return nil, err
	}
	var env dsseJSON
	if err := json.Unmarshal([]byte(spec.ProposedContent.Envelope), &env); err != nil {
		return nil, err
	}
	envHash := sha256.Sum256([]byte(spec.ProposedContent.Envelope))
	payloadHash := sha256.Sum256(env.Payload)
	return json.Marshal(map[string]any{
		"apiVersion": "0.0.1", "kind": "dsse",
		"spec": map[string]any{
			"envelopeHash": map[string]string{"algorithm": "sha256", "value": hex.EncodeToString(envHash[:])},
			"payloadHash":  map[string]string{"algorithm": "sha256", "value": hex.EncodeToString(payloadHash[:])},
			"signatures": []map[string]any{{
				"signature": base64.StdEncoding.EncodeToString(env.Signatures[0].Sig),
				"verifier":  spec.ProposedContent.Verifiers[0],
			}},
		},
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
