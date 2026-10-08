package signing

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/samber/oops"
	protobundle "github.com/sigstore/protobuf-specs/gen/pb-go/bundle/v1"
	protocommon "github.com/sigstore/protobuf-specs/gen/pb-go/common/v1"
	protodsse "github.com/sigstore/protobuf-specs/gen/pb-go/dsse"
	protorekor "github.com/sigstore/protobuf-specs/gen/pb-go/rekor/v1"
	"github.com/sigstore/sigstore/pkg/oauthflow"

	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
)

// This file builds Sigstore bundles: it signs the content, asks Fulcio for a
// certificate (keyless) and records the signature in Rekor. It replaces
// sigstore-go's pkg/sign, whose Rekor client pulls in golang.org/x/crypto/openpgp
// (deprecated, with open advisories) through rekor's pki package. Only the Rekor
// v1 API and the Fulcio v2 signingCert API are spoken, which is what the
// public-good instances serve.

const (
	bundleMediaType = "application/vnd.dev.sigstore.bundle.v0.3+json"
	maxServiceBody  = 1 << 20
	userAgent       = "ai-rulez"
)

// Content is what a Signer signs: a DSSE envelope's payload (DSSEData) or raw
// bytes (PlainData, a message signature).
type Content interface {
	// PreAuthEncoding returns the bytes the signature covers.
	PreAuthEncoding() []byte
	// Bundle stores the signed content in b.
	Bundle(b *protobundle.Bundle, signature, digest []byte, alg protocommon.HashAlgorithm)
}

// DSSEData is a DSSE payload; the signature covers its pre-authentication encoding.
type DSSEData struct {
	Data        []byte
	PayloadType string
}

// PreAuthEncoding implements Content.
func (d *DSSEData) PreAuthEncoding() []byte {
	return fmt.Appendf(nil, "DSSEv1 %d %s %d %s", len(d.PayloadType), d.PayloadType, len(d.Data), d.Data)
}

// Bundle implements Content.
func (d *DSSEData) Bundle(b *protobundle.Bundle, signature, _ []byte, _ protocommon.HashAlgorithm) {
	b.Content = &protobundle.Bundle_DsseEnvelope{DsseEnvelope: &protodsse.Envelope{
		Payload:     d.Data,
		PayloadType: d.PayloadType,
		Signatures:  []*protodsse.Signature{{Sig: signature}},
	}}
}

// PlainData is signed as is (a message signature, like `cosign sign-blob`).
type PlainData struct {
	Data []byte
}

// PreAuthEncoding implements Content.
func (p *PlainData) PreAuthEncoding() []byte { return p.Data }

// Bundle implements Content.
func (p *PlainData) Bundle(b *protobundle.Bundle, signature, digest []byte, alg protocommon.HashAlgorithm) {
	b.Content = &protobundle.Bundle_MessageSignature{MessageSignature: &protocommon.MessageSignature{
		MessageDigest: &protocommon.HashOutput{Algorithm: alg, Digest: digest},
		Signature:     signature,
	}}
}

// serviceClient is the HTTP side shared by the Fulcio and Rekor clients: a
// bounded timeout, retries on 5xx and 429, and no redirects (the Fulcio request
// carries the identity token, which must reach only the configured host).
type serviceClient struct {
	baseURL string
	http    *http.Client
	retries int
	backoff time.Duration
}

func newServiceClient(baseURL string) serviceClient {
	return serviceClient{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		http: &http.Client{
			Timeout:       defaultTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		retries: defaultRetries,
		backoff: time.Second,
	}
}

// answer is a service's final HTTP answer; the body is bounded.
type answer struct {
	status int
	header http.Header
	body   []byte
}

// do sends the request built by build, retrying on a transient status.
func (c serviceClient) do(ctx context.Context, build func() (*http.Request, error)) (answer, error) {
	for attempt := 0; ; attempt++ {
		req, err := build()
		if err != nil {
			return answer{}, err
		}
		req = req.WithContext(ctx)
		req.Header.Set("User-Agent", userAgent)
		resp, err := c.http.Do(req) //nolint:gosec // the URL is the configured, https-checked service URL
		if err != nil {
			return answer{}, err
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxServiceBody))
		_ = resp.Body.Close()
		if err != nil {
			return answer{}, err
		}
		transient := resp.StatusCode >= http.StatusInternalServerError || resp.StatusCode == http.StatusTooManyRequests
		if !transient || attempt >= c.retries {
			return answer{status: resp.StatusCode, header: resp.Header, body: body}, nil
		}
		timer := time.NewTimer(c.backoff << attempt)
		select {
		case <-ctx.Done():
			timer.Stop()
			return answer{}, ctx.Err()
		case <-timer.C:
		}
	}
}

// snippet is a short, single-line excerpt of a service's error body.
func snippet(body []byte) string {
	s := strings.Join(strings.Fields(string(body)), " ")
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}

// fulcioClient exchanges an OIDC token and a proof of possession for a
// short-lived code-signing certificate (POST /api/v2/signingCert).
type fulcioClient struct{ serviceClient }

type fulcioRequest struct {
	PublicKeyRequest struct {
		PublicKey struct {
			Algorithm string `json:"algorithm"`
			Content   string `json:"content"`
		} `json:"publicKey"`
		ProofOfPossession string `json:"proofOfPossession"`
	} `json:"publicKeyRequest"`
}

type fulcioChain struct {
	Chain struct {
		Certificates []string `json:"certificates"`
	} `json:"chain"`
}

type fulcioResponse struct {
	Embedded fulcioChain `json:"signedCertificateEmbeddedSct"`
	Detached fulcioChain `json:"signedCertificateDetachedSct"`
}

// certificate returns the DER leaf certificate Fulcio issues for kp.
func (f fulcioClient) certificate(ctx context.Context, kp *KeyPair, idToken string) ([]byte, error) {
	payload, err := certificateRequest(ctx, kp, idToken)
	if err != nil {
		return nil, err
	}
	ans, err := f.do(ctx, func() (*http.Request, error) {
		req, err := http.NewRequest(http.MethodPost, f.baseURL+"/api/v2/signingCert", bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+idToken)
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	})
	if err != nil {
		return nil, oops.Wrapf(err, "request a certificate from Fulcio")
	}
	if ans.status != http.StatusOK && ans.status != http.StatusCreated {
		return nil, oops.Errorf("Fulcio answered HTTP %d: %s", ans.status, snippet(ans.body))
	}
	return leafCertificate(ans.body, kp)
}

// certificateRequest is the signingCert request body: kp's public key and its
// signature over the token's subject (the proof of possession).
func certificateRequest(ctx context.Context, kp *KeyPair, idToken string) ([]byte, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) < 2 {
		return nil, oops.Errorf("the identity token is not a JWT")
	}
	claims, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, oops.Errorf("the identity token is not a JWT")
	}
	// The token is untrusted here: Fulcio verifies it. The subject only feeds the
	// proof of possession.
	subject, err := oauthflow.SubjectFromUnverifiedToken(claims)
	if err != nil {
		return nil, oops.Wrapf(err, "read the identity token subject")
	}
	proof, _, err := kp.SignData(ctx, []byte(subject))
	if err != nil {
		return nil, oops.Wrapf(err, "sign the proof of possession")
	}
	pubPEM, err := kp.GetPublicKeyPem()
	if err != nil {
		return nil, oops.Wrapf(err, "encode the public key")
	}
	var reqBody fulcioRequest
	reqBody.PublicKeyRequest.PublicKey.Algorithm = kp.GetKeyAlgorithm()
	reqBody.PublicKeyRequest.PublicKey.Content = pubPEM
	reqBody.PublicKeyRequest.ProofOfPossession = base64.StdEncoding.EncodeToString(proof)
	payload, err := json.Marshal(&reqBody)
	if err != nil {
		return nil, oops.Wrapf(err, "encode the certificate request")
	}
	return payload, nil
}

// leafCertificate is the first certificate of Fulcio's chain, which must
// certify kp.
func leafCertificate(body []byte, kp *KeyPair) ([]byte, error) {
	var resp fulcioResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, oops.Errorf("Fulcio returned a response that is not JSON")
	}
	certs := resp.Embedded.Chain.Certificates
	if len(certs) == 0 {
		certs = resp.Detached.Chain.Certificates
	}
	if len(certs) == 0 {
		return nil, oops.Errorf("Fulcio returned no certificate")
	}
	block, _ := pem.Decode([]byte(certs[0]))
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, oops.Errorf("Fulcio returned a certificate that is not PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, oops.Wrapf(err, "parse the Fulcio certificate")
	}
	if !publicKeysEqual(cert.PublicKey, kp.GetPublicKey()) {
		return nil, oops.Errorf("Fulcio returned a certificate for a different key")
	}
	return block.Bytes, nil
}

func publicKeysEqual(a, b crypto.PublicKey) bool {
	type equaler interface{ Equal(x crypto.PublicKey) bool }
	e, ok := a.(equaler)
	return ok && e.Equal(b)
}

// rekorClient records a signature in a Rekor v1 log (POST /api/v1/log/entries).
type rekorClient struct{ serviceClient }

// rekorUUID is a Rekor entry UUID: the leaf hash, optionally prefixed by the
// tree ID (16 hex characters).
var rekorUUID = regexp.MustCompile(`^([0-9a-f]{16})?[0-9a-f]{64}$`)

type rekorProof struct {
	Checkpoint string   `json:"checkpoint"`
	Hashes     []string `json:"hashes"`
	LogIndex   *int64   `json:"logIndex"`
	RootHash   string   `json:"rootHash"`
	TreeSize   *int64   `json:"treeSize"`
}

type rekorEntry struct {
	Body           string `json:"body"`
	IntegratedTime *int64 `json:"integratedTime"`
	LogID          string `json:"logID"`
	LogIndex       *int64 `json:"logIndex"`
	Verification   *struct {
		InclusionProof       *rekorProof `json:"inclusionProof"`
		SignedEntryTimestamp []byte      `json:"signedEntryTimestamp"`
	} `json:"verification"`
}

type dsseJSONSignature struct {
	Sig []byte `json:"sig,omitempty"`
}

type dsseJSON struct {
	Payload     []byte              `json:"payload,omitempty"`
	PayloadType string              `json:"payloadType,omitempty"`
	Signatures  []dsseJSONSignature `json:"signatures,omitempty"`
}

// proposedEntry is the Rekor entry for the signed content of b: a dsse entry for
// an envelope, a hashedrekord entry for a message signature. verifierPEM is the
// signing certificate or public key.
func proposedEntry(b *protobundle.Bundle, verifierPEM []byte) (kind string, entry []byte, err error) {
	switch {
	case b.GetDsseEnvelope() != nil:
		env := b.GetDsseEnvelope()
		dj := dsseJSON{Payload: env.GetPayload(), PayloadType: env.GetPayloadType()}
		for _, s := range env.GetSignatures() {
			dj.Signatures = append(dj.Signatures, dsseJSONSignature{Sig: s.GetSig()})
		}
		envJSON, err := json.Marshal(dj)
		if err != nil {
			return "", nil, oops.Wrapf(err, "encode the envelope")
		}
		entry, err = json.Marshal(map[string]any{
			"apiVersion": "0.0.1",
			"kind":       "dsse",
			"spec": map[string]any{"proposedContent": map[string]any{
				"envelope":  string(envJSON),
				"verifiers": [][]byte{verifierPEM},
			}},
		})
		return "dsse", entry, err
	case b.GetMessageSignature() != nil:
		ms := b.GetMessageSignature()
		alg, ok := map[int]string{sha256.Size: contentlock.Algorithm, sha512.Size384: "sha384", sha512.Size: "sha512"}[len(ms.GetMessageDigest().GetDigest())]
		if !ok {
			return "", nil, oops.Errorf("a message signature logged in Rekor needs a SHA-2 digest (use an ECDSA key)")
		}
		entry, err = json.Marshal(map[string]any{
			"apiVersion": "0.0.1",
			"kind":       "hashedrekord",
			"spec": map[string]any{
				"data": map[string]any{"hash": map[string]string{
					"algorithm": alg, "value": hex.EncodeToString(ms.GetMessageDigest().GetDigest()),
				}},
				"signature": map[string]any{
					"content":   ms.GetSignature(),
					"publicKey": map[string]any{"content": verifierPEM},
				},
			},
		})
		return "hashedrekord", entry, err
	}
	return "", nil, oops.Errorf("the bundle has no signature to log")
}

// entry logs the signature of b and returns the transparency log entry. An
// entry that already exists (HTTP 409) is fetched instead.
func (r rekorClient) entry(ctx context.Context, b *protobundle.Bundle, verifierPEM []byte) (*protorekor.TransparencyLogEntry, error) {
	kind, payload, err := proposedEntry(b, verifierPEM)
	if err != nil {
		return nil, err
	}
	ans, err := r.do(ctx, func() (*http.Request, error) {
		req, err := http.NewRequest(http.MethodPost, r.baseURL+"/api/v1/log/entries", bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		return req, nil
	})
	if err != nil {
		return nil, oops.Wrapf(err, "record the signature in Rekor")
	}
	if ans.status == http.StatusConflict {
		if ans, err = r.existing(ctx, ans.header.Get("Location")); err != nil {
			return nil, err
		}
	}
	if ans.status != http.StatusCreated && ans.status != http.StatusOK {
		return nil, oops.Errorf("Rekor answered HTTP %d: %s", ans.status, snippet(ans.body))
	}
	return parseRekorEntry(ans.body, kind)
}

// existing fetches the entry a 409 answer points at, by UUID on the same log.
func (r rekorClient) existing(ctx context.Context, location string) (answer, error) {
	uuid := path.Base(location)
	if !rekorUUID.MatchString(uuid) {
		return answer{}, oops.Errorf("Rekor reported a conflicting entry without a valid location")
	}
	ans, err := r.do(ctx, func() (*http.Request, error) {
		req, err := http.NewRequest(http.MethodGet, r.baseURL+"/api/v1/log/entries/"+uuid, http.NoBody)
		if err == nil {
			req.Header.Set("Accept", "application/json")
		}
		return req, err
	})
	if err != nil {
		return answer{}, oops.Wrapf(err, "fetch the existing Rekor entry")
	}
	return ans, nil
}

// parseRekorEntry converts Rekor's {uuid: entry} answer into the bundle's log
// entry, checking that the logged body is the kind that was submitted.
func parseRekorEntry(body []byte, kind string) (*protorekor.TransparencyLogEntry, error) {
	var answer map[string]rekorEntry
	if err := json.Unmarshal(body, &answer); err != nil || len(answer) != 1 {
		return nil, oops.Errorf("Rekor returned an unexpected response")
	}
	var e rekorEntry
	for _, v := range answer {
		e = v
	}
	if e.IntegratedTime == nil || e.LogIndex == nil || e.Verification == nil {
		return nil, oops.Errorf("Rekor returned an entry without its time, index or verification")
	}
	if e.Verification.InclusionProof == nil {
		// A v0.3 bundle must carry the inclusion proof.
		return nil, oops.Errorf("Rekor returned an entry without an inclusion proof")
	}
	logID, err1 := hex.DecodeString(e.LogID)
	canonical, err2 := base64.StdEncoding.DecodeString(e.Body)
	if err := errors.Join(err1, err2); err != nil || len(logID) == 0 {
		return nil, oops.Errorf("Rekor returned a malformed entry")
	}
	proof, err := inclusionProof(e.Verification.InclusionProof)
	if err != nil {
		return nil, err
	}
	var head struct {
		Kind       string `json:"kind"`
		APIVersion string `json:"apiVersion"`
	}
	if err := json.Unmarshal(canonical, &head); err != nil || head.Kind != kind || head.APIVersion == "" {
		return nil, oops.Errorf("Rekor logged a %q entry, not the %q entry that was submitted", head.Kind, kind)
	}
	tle := &protorekor.TransparencyLogEntry{
		LogIndex:          *e.LogIndex,
		LogId:             &protocommon.LogId{KeyId: logID},
		KindVersion:       &protorekor.KindVersion{Kind: head.Kind, Version: head.APIVersion},
		IntegratedTime:    *e.IntegratedTime,
		CanonicalizedBody: canonical,
		InclusionProof:    proof,
	}
	if len(e.Verification.SignedEntryTimestamp) > 0 {
		tle.InclusionPromise = &protorekor.InclusionPromise{SignedEntryTimestamp: e.Verification.SignedEntryTimestamp}
	}
	return tle, nil
}

// inclusionProof converts Rekor's hex-encoded inclusion proof.
func inclusionProof(p *rekorProof) (*protorekor.InclusionProof, error) {
	if p.LogIndex == nil || p.TreeSize == nil {
		return nil, oops.Errorf("Rekor returned an incomplete inclusion proof")
	}
	rootHash, err := hex.DecodeString(p.RootHash)
	if err != nil {
		return nil, oops.Errorf("Rekor returned a malformed inclusion proof")
	}
	hashes := make([][]byte, len(p.Hashes))
	for i, h := range p.Hashes {
		if hashes[i], err = hex.DecodeString(h); err != nil {
			return nil, oops.Errorf("Rekor returned a malformed inclusion proof")
		}
	}
	return &protorekor.InclusionProof{
		LogIndex:   *p.LogIndex,
		RootHash:   rootHash,
		TreeSize:   *p.TreeSize,
		Hashes:     hashes,
		Checkpoint: &protorekor.Checkpoint{Envelope: p.Checkpoint},
	}, nil
}

// newEphemeralKeyPair is the ECDSA P-256 key a keyless signature is made with.
func newEphemeralKeyPair() (*KeyPair, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, oops.Wrapf(err, "generate an ephemeral key")
	}
	return NewKeyPair(priv)
}

// buildBundle signs content with kp and assembles the bundle. With a Fulcio
// client the verification material is the certificate Fulcio issues for kp
// (idToken authenticates the request); otherwise it is kp's key hint. With a
// Rekor client the signature is also logged.
func buildBundle(ctx context.Context, content Content, kp *KeyPair, fulcio *fulcioClient, idToken string, rekor *rekorClient) (*protobundle.Bundle, error) {
	b := &protobundle.Bundle{MediaType: bundleMediaType}
	sig, digest, err := kp.SignData(ctx, content.PreAuthEncoding())
	if err != nil {
		return nil, oops.Wrapf(err, "sign the content")
	}
	content.Bundle(b, sig, digest, kp.GetHashAlgorithm())
	var verifierPEM []byte
	if fulcio != nil {
		der, err := fulcio.certificate(ctx, kp, idToken)
		if err != nil {
			return nil, err
		}
		b.VerificationMaterial = &protobundle.VerificationMaterial{
			Content: &protobundle.VerificationMaterial_Certificate{Certificate: &protocommon.X509Certificate{RawBytes: der}},
		}
		verifierPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	} else {
		b.VerificationMaterial = &protobundle.VerificationMaterial{
			Content: &protobundle.VerificationMaterial_PublicKey{PublicKey: &protocommon.PublicKeyIdentifier{Hint: string(kp.GetHint())}},
		}
		pubPEM, err := kp.GetPublicKeyPem()
		if err != nil {
			return nil, oops.Wrapf(err, "encode the public key")
		}
		verifierPEM = []byte(pubPEM)
	}
	if rekor != nil {
		tle, err := rekor.entry(ctx, b, verifierPEM)
		if err != nil {
			return nil, err
		}
		b.VerificationMaterial.TlogEntries = append(b.VerificationMaterial.TlogEntries, tle)
	}
	return b, nil
}
