package signing

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// This file is the seam between the verification policy in this package (trust
// sets, thresholds, freshness, rollback state) and the Sigstore machinery that
// checks signatures (sigstore-go, Fulcio, Rekor, KMS), which lives in
// internal/signing/sigstore. This package imports none of it, so a library
// consumer that only needs the policy side does not link the Sigstore, Vault,
// AWS, GCP and Azure SDK graph. The command wires the real backend once at
// startup with UseBackend(sigstore.New()); without one, every cryptographic
// check fails closed with AR721.

// TrustedMaterial is a parsed Sigstore trusted root. It is opaque here: only
// the backend that produced it reads it.
type TrustedMaterial = any

// Backend checks bundles cryptographically.
type Backend interface {
	// Verify checks a DSSE bundle against v and returns the signed payload.
	Verify(v Verifier, data []byte) (*Result, error)
	// VerifyBlob checks a message-signature bundle against the signed artifact.
	VerifyBlob(v Verifier, data, artifact []byte) (*Result, error)
	// Inspect reads who a bundle claims to be signed by, verifying nothing.
	Inspect(data []byte) (SignerInfo, error)
	// IsBlobBundle reports whether data is a message-signature bundle.
	IsBlobBundle(data []byte) bool
	// MessageDigest returns the digest a message-signature bundle covers, and
	// whether it is a SHA-256 digest.
	MessageDigest(data []byte) (digest []byte, sha256 bool, err error)
	// LoadTrustedRoot parses a trusted root file.
	LoadTrustedRoot(data []byte) (TrustedMaterial, error)
}

type backendHolder struct{ b Backend }

var activeBackend atomic.Pointer[backendHolder]

// UseBackend installs the cryptographic backend. The command calls it once at
// startup; tests that verify real bundles call it from TestMain.
func UseBackend(b Backend) { activeBackend.Store(&backendHolder{b: b}) }

func backend() (Backend, error) {
	if h := activeBackend.Load(); h != nil && h.b != nil {
		return h.b, nil
	}
	return nil, Errorf(CodeInvalid, "no Sigstore verification backend is linked into this build, so signatures cannot be checked")
}

// LoadTrustedRoot parses a trusted root file with the installed backend.
func LoadTrustedRoot(data []byte) (TrustedMaterial, error) {
	b, err := backend()
	if err != nil {
		return nil, err
	}
	return b.LoadTrustedRoot(data)
}

// TLogMode says whether a transparency log entry is needed.
type TLogMode string

// Transparency log modes ([signing] tlog).
const (
	// TLogRequired needs a log entry with its proof; the entry time is the
	// signing time. The default whenever a keyless identity is trusted.
	TLogRequired TLogMode = "required"
	// TLogOptional accepts a log entry or a signed timestamp, and for a key
	// signature also neither (the signing time is then unknown).
	TLogOptional TLogMode = "optional"
	// TLogOff is key mode without a log.
	TLogOff TLogMode = "off"
)

// Signer kinds in a Result.
const (
	KindKey     = "key"
	KindKeyless = "keyless"
)

// SignerInfo says who signed: a key (KeyID, "sha256:<hex>") or a certificate
// (Identity is the SAN, Issuer the OIDC issuer).
type SignerInfo struct {
	Kind     string `json:"kind"`
	KeyID    string `json:"key_id,omitempty"`
	Identity string `json:"identity,omitempty"`
	Issuer   string `json:"issuer,omitempty"`
}

// Result is a cryptographically verified bundle. Verified means the signature,
// certificate chain, log proof and timestamps check out against the trust
// anchors; whether the signer is allowed is TrustSet.Check.
type Result struct {
	PayloadType string
	Payload     []byte
	// Statement is the decoded payload when PayloadType is in-toto.
	Statement *Statement
	Signer    SignerInfo
	// SignedAt is the earliest time a log or timestamp authority observed the
	// signature. Zero when there is none, which sets Weak.
	SignedAt time.Time
	// Logged reports a transparency log entry in the bundle.
	Logged bool
	// Weak means the signing time is unknown: freshness can only rest on a claim
	// inside the signed statement.
	Weak bool
}

// Verifier holds the trust anchors verification needs.
type Verifier struct {
	// TrustedRoot holds the Fulcio, Rekor, CT and timestamp roots. Required for
	// keyless bundles and for any bundle with a log entry.
	TrustedRoot TrustedMaterial
	// Keys are the public keys a key-signed bundle may verify with.
	Keys []crypto.PublicKey
	// TLog defaults to TLogRequired.
	TLog TLogMode
}

// Verify checks a DSSE bundle offline and returns the signed payload. Failures
// are *Error: AR721 for a bad bundle, signature, chain or proof, AR725 when a
// needed trusted root is missing, AR726 when a required log entry is absent.
func (v *Verifier) Verify(data []byte) (*Result, error) {
	b, err := backend()
	if err != nil {
		return nil, err
	}
	return b.Verify(*v, data)
}

// VerifyBlob checks a message-signature bundle (the kind `cosign sign-blob
// --bundle` writes) against artifact, the exact bytes that were signed.
func (v *Verifier) VerifyBlob(data, artifact []byte) (*Result, error) {
	b, err := backend()
	if err != nil {
		return nil, err
	}
	return b.VerifyBlob(*v, data, artifact)
}

// IsBlobBundle reports whether data is a message-signature bundle rather than a
// DSSE one.
func IsBlobBundle(data []byte) bool {
	b, err := backend()
	return err == nil && b.IsBlobBundle(data)
}

// Inspect reads who a bundle claims to be signed by without verifying anything:
// the certificate identity, or the key hint. It is for display after signing.
func Inspect(data []byte) (SignerInfo, error) {
	b, err := backend()
	if err != nil {
		return SignerInfo{}, err
	}
	return b.Inspect(data)
}

// Signer turns a statement into a Sigstore bundle. The implementations (keys,
// keyless, KMS) are in internal/signing/sigstore.
type Signer interface {
	// SignDSSE signs payload as a DSSE envelope and returns the bundle JSON.
	SignDSSE(ctx context.Context, payload []byte, payloadType string) ([]byte, error)
	// SignBlob signs data as is (a message signature, like `cosign sign-blob`)
	// and returns the bundle JSON.
	SignBlob(ctx context.Context, data []byte) ([]byte, error)
}

// SignStatement signs an in-toto statement as a DSSE envelope and returns the
// Sigstore bundle as JSON. Every feature that signs something (the lock,
// approvals, an SBOM, a policy) defines a predicate type and calls this.
// sigstore-go verifies in-toto payloads only, so there is no raw-payload variant.
func SignStatement(ctx context.Context, s Signer, st *Statement) ([]byte, error) {
	payload, err := st.Marshal()
	if err != nil {
		return nil, err
	}
	data, err := s.SignDSSE(ctx, payload, PayloadTypeInToto)
	if err != nil {
		return nil, oops.Wrapf(err, "sign the attestation")
	}
	return data, nil
}

// blobCovers reports AR724 when the digest a message-signature bundle signed is
// not the SHA-256 of artifact, so a lock that changed after signing is told
// apart from a forged signature.
func blobCovers(data, artifact []byte) error {
	b, err := backend()
	if err != nil {
		return err
	}
	digest, isSHA256, err := b.MessageDigest(data)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(artifact)
	if digest != nil && isSHA256 && !bytes.Equal(digest, sum[:]) {
		return Errorf(CodeSubjectMismatch, "%s changed since it was signed: the signed statement differs from the one recomputed from the lock", lockfile.FileName)
	}
	return nil
}

// CheckKeyType refuses a public key the signing format does not support. RSA is
// refused: ai-rulez keys are ECDSA (P-256, P-384, P-521) or ed25519.
func CheckKeyType(pub crypto.PublicKey) error {
	switch k := pub.(type) {
	case *ecdsa.PublicKey:
		switch k.Curve {
		case elliptic.P256(), elliptic.P384(), elliptic.P521():
			return nil
		}
		return oops.Errorf("unsupported ECDSA curve %s (use P-256, P-384 or P-521)", k.Curve.Params().Name)
	case ed25519.PublicKey:
		return nil
	}
	return oops.Errorf("unsupported key type %T (use an ECDSA or ed25519 key)", pub)
}

// Fingerprint returns "sha256:<hex>" of the key's DER SubjectPublicKeyInfo: the
// identifier trust entries and results use for a key.
func Fingerprint(pub crypto.PublicKey) (string, error) {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return "", oops.Wrapf(err, "encode the public key")
	}
	sum := sha256.Sum256(der)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// KeyHint is the hint a bundle carries for a key: base64 of the SHA-256 of its
// DER public key, as cosign and sigstore-go write it.
func KeyHint(pub crypto.PublicKey) ([]byte, error) {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, oops.Wrapf(err, "encode the public key")
	}
	sum := sha256.Sum256(der)
	return []byte(base64.StdEncoding.EncodeToString(sum[:])), nil
}

// ParsePublicKey reads a PEM public key (ECDSA or ed25519).
func ParsePublicKey(pemBytes []byte) (crypto.PublicKey, error) {
	pub, err := unmarshalPEMToPublicKey(pemBytes)
	if err != nil {
		return nil, oops.Wrapf(err, "read the public key")
	}
	if err := CheckKeyType(pub); err != nil {
		return nil, err
	}
	return pub, nil
}

func unmarshalPEMToPublicKey(pemBytes []byte) (crypto.PublicKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("PEM decoding failed")
	}
	switch block.Type {
	case "PUBLIC KEY":
		return x509.ParsePKIXPublicKey(block.Bytes)
	case "RSA PUBLIC KEY":
		return x509.ParsePKCS1PublicKey(block.Bytes)
	default:
		return nil, fmt.Errorf("unknown Public key PEM file type: %v. Are you passing the correct public key?", block.Type)
	}
}
