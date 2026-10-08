// Package fakesigstore is an in-process Fulcio and Rekor for tests of code that
// signs or verifies Sigstore bundles.
package fakesigstore

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

	"github.com/Goldziher/ai-rulez/v5/internal/signing"
	"github.com/Goldziher/ai-rulez/v5/internal/signing/sigstore"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
)

// Fake is an in-process Fulcio and Rekor v1 that speak the HTTP APIs the
// keyless signer uses, plus the trusted root that verifies what they issue. The
// leaf certificates carry no signed certificate timestamp, so the root lists no
// CT log.
type Fake struct {
	t                        *testing.T
	Identity, Issuer, Token  string
	CaKey, LogKey            *ecdsa.PrivateKey
	CaCert                   *x509.Certificate
	LogID                    []byte
	Fulcio, Rekor            *httptest.Server
	Mu                       sync.Mutex
	Entries                  map[string][]byte // uuid -> response entry JSON
	FulcioCalls, RekorCalls  int
	RekorFailures            int  // answer HTTP 503 this many times first
	Conflict                 bool // answer HTTP 409 and serve the entry by UUID
	WrongKind, WrongCertKey  bool
	LastAuth, LastProposed   string
	LastProofSubjectVerified bool
}

// New starts a fake for a certificate naming identity and issuer.
func New(t *testing.T, identity, issuer string) *Fake {
	t.Helper()
	f := &Fake{t: t, Identity: identity, Issuer: issuer, Entries: map[string][]byte{}}
	var err error
	f.CaKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	f.LogKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "fake fulcio"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, f.CaKey.Public(), f.CaKey)
	require.NoError(t, err)
	f.CaCert, err = x509.ParseCertificate(der)
	require.NoError(t, err)
	logDER, err := x509.MarshalPKIXPublicKey(f.LogKey.Public())
	require.NoError(t, err)
	sum := sha256.Sum256(logDER)
	f.LogID = sum[:]
	claims, err := json.Marshal(map[string]string{"sub": identity, "iss": issuer})
	require.NoError(t, err)
	f.Token = "e30." + base64.RawURLEncoding.EncodeToString(claims) + ".c2ln"
	f.Fulcio = httptest.NewServer(http.HandlerFunc(f.serveFulcio))
	f.Rekor = httptest.NewServer(http.HandlerFunc(f.serveRekor))
	t.Cleanup(f.Fulcio.Close)
	t.Cleanup(f.Rekor.Close)
	return f
}

// Signer is a keyless signer pointed at the fake services, with no retry delay.
func (f *Fake) Signer() *sigstore.KeylessSigner {
	f.t.Helper()
	s, err := sigstore.NewKeylessSigner(sigstore.KeylessOptions{IDToken: f.Token, FulcioURL: f.Fulcio.URL, RekorURL: f.Rekor.URL, RetryBackoff: time.Millisecond})
	require.NoError(f.t, err)
	return s
}

// Bundle signs payload as an in-toto DSSE envelope and returns the bundle JSON.
func (f *Fake) Bundle(payload []byte) []byte {
	f.t.Helper()
	pb, err := f.Signer().Bundle(context.Background(), &sigstore.DSSEData{Data: payload, PayloadType: signing.PayloadTypeInToto})
	require.NoError(f.t, err)
	data, err := protojson.Marshal(pb)
	require.NoError(f.t, err)
	return data
}

// TrustedRoot verifies the fake's certificates and log entries.
func (f *Fake) TrustedRoot() root.TrustedMaterial {
	return &fakeRoot{
		ca: &root.FulcioCertificateAuthority{Root: f.CaCert, ValidityPeriodStart: f.CaCert.NotBefore, ValidityPeriodEnd: f.CaCert.NotAfter},
		logs: map[string]*root.TransparencyLog{hex.EncodeToString(f.LogID): {
			BaseURL: f.Rekor.URL, ID: f.LogID, ValidityPeriodStart: time.Now().Add(-time.Hour),
			HashFunc: crypto.SHA256, PublicKey: f.LogKey.Public(), SignatureHashFunc: crypto.SHA256,
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

func (f *Fake) serveFulcio(w http.ResponseWriter, r *http.Request) {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	f.FulcioCalls++
	f.LastAuth = r.Header.Get("Authorization")
	if r.Method != http.MethodPost || r.URL.Path != "/api/v2/signingCert" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if f.LastAuth != "Bearer "+f.Token {
		http.Error(w, "bad token", http.StatusUnauthorized)
		return
	}
	var req fulcioRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	pub, err := signing.ParsePublicKey([]byte(req.PublicKeyRequest.PublicKey.Content))
	if err != nil {
		http.Error(w, "bad key", http.StatusBadRequest)
		return
	}
	proof, err := base64.StdEncoding.DecodeString(req.PublicKeyRequest.ProofOfPossession)
	digest := sha256.Sum256([]byte(f.Identity))
	ecPub, ok := pub.(*ecdsa.PublicKey)
	f.LastProofSubjectVerified = err == nil && ok && ecdsa.VerifyASN1(ecPub, digest[:], proof)
	if !f.LastProofSubjectVerified {
		http.Error(w, "bad proof of possession", http.StatusBadRequest)
		return
	}
	certPub := pub
	if f.WrongCertKey {
		other, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		certPub = other.Public()
	}
	issuerExt, _ := asn1.Marshal(f.Issuer)
	leaf := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()), EmailAddresses: []string{f.Identity},
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(10 * time.Minute),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning},
		ExtraExtensions: []pkix.Extension{
			{Id: asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 57264, 1, 1}, Value: []byte(f.Issuer)},
			{Id: asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 57264, 1, 8}, Value: issuerExt},
		},
	}
	der, err := x509.CreateCertificate(rand.Reader, leaf, f.CaCert, certPub, f.CaKey)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	chain := []string{
		string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: f.CaCert.Raw})),
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"signedCertificateEmbeddedSct": map[string]any{"chain": map[string]any{"certificates": chain}}})
}

func (f *Fake) serveRekor(w http.ResponseWriter, r *http.Request) {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	f.RekorCalls++
	const prefix = "/api/v1/log/entries"
	switch {
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, prefix+"/"):
		uuid := strings.TrimPrefix(r.URL.Path, prefix+"/")
		entry, ok := f.Entries[uuid]
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
	if f.RekorFailures > 0 {
		f.RekorFailures--
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	raw, _ := io.ReadAll(r.Body)
	f.LastProposed = string(raw)
	body, err := canonicalEntry(raw, f.WrongKind)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	index := int64(len(f.Entries))
	integrated := time.Now().Unix()
	b64 := base64.StdEncoding.EncodeToString(body)
	logID := hex.EncodeToString(f.LogID)
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
	f.Entries[uuid] = entry
	if f.Conflict {
		w.Header().Set("Location", prefix+"/"+uuid)
		http.Error(w, "an equivalent entry already exists", http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]json.RawMessage{uuid: entry})
}

// signCheckpoint returns a Rekor v1 signed tree head for a one-leaf tree.
func (f *Fake) signCheckpoint(rootHash []byte) (string, error) {
	const origin = "rekor.test - 1"
	body := fmt.Sprintf("%s\n1\n%s\n", origin, base64.StdEncoding.EncodeToString(rootHash))
	sum := sha256.Sum256([]byte(body))
	sig, err := ecdsa.SignASN1(rand.Reader, f.LogKey, sum[:])
	if err != nil {
		return "", err
	}
	der, err := x509.MarshalPKIXPublicKey(f.LogKey.Public())
	if err != nil {
		return "", err
	}
	keyHash := sha256.Sum256(der)
	stamped := append(keyHash[:4:4], sig...)
	return fmt.Sprintf("%s\n\u2014 rekor.test %s\n", body, base64.StdEncoding.EncodeToString(stamped)), nil
}

// signSET signs the inclusion promise: the canonical JSON (keys sorted) of the
// body, integrated time, log ID and log index.
func (f *Fake) signSET(body string, integrated int64, logID string, index int64) ([]byte, error) {
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
	return ecdsa.SignASN1(rand.Reader, f.LogKey, sum[:])
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

// fulcioRequest is the body of a Fulcio v2 signingCert request.
type fulcioRequest struct {
	PublicKeyRequest struct {
		PublicKey struct {
			Algorithm string `json:"algorithm"`
			Content   string `json:"content"`
		} `json:"publicKey"`
		ProofOfPossession string `json:"proofOfPossession"`
	} `json:"publicKeyRequest"`
}

type dsseJSONSignature struct {
	Sig []byte `json:"sig,omitempty"`
}

type dsseJSON struct {
	Payload     []byte              `json:"payload,omitempty"`
	PayloadType string              `json:"payloadType,omitempty"`
	Signatures  []dsseJSONSignature `json:"signatures,omitempty"`
}
