package signing

import (
	"bytes"
	"crypto"
	"encoding/base64"
	"time"

	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/verify"
	"github.com/sigstore/sigstore/pkg/signature"
)

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
	TrustedRoot root.TrustedMaterial
	// Keys are the public keys a key-signed bundle may verify with.
	Keys []crypto.PublicKey
	// TLog defaults to TLogRequired.
	TLog TLogMode
}

func (v *Verifier) tlog() TLogMode {
	if v.TLog == "" {
		return TLogRequired
	}
	return v.TLog
}

// Verify checks a DSSE bundle offline and returns the signed payload. Failures
// are *Error: AR721 for a bad bundle, signature, chain or proof, AR725 when a
// needed trusted root is missing, AR726 when a required log entry is absent.
func (v *Verifier) Verify(data []byte) (*Result, error) {
	b, err := parseBundle(data)
	if err != nil {
		return nil, err
	}
	env, err := b.Envelope()
	if err != nil {
		return nil, wrap(CodeInvalid, err, "the bundle holds no DSSE envelope")
	}
	payload, err := env.DecodeB64Payload()
	if err != nil {
		return nil, wrap(CodeInvalid, err, "the envelope payload is not base64")
	}
	res := &Result{PayloadType: env.PayloadType, Payload: payload}
	if err := v.check(b, verify.WithoutArtifactUnsafe(), res); err != nil {
		return nil, err
	}
	if res.PayloadType == PayloadTypeInToto {
		if res.Statement, err = ParseStatement(payload); err != nil {
			return nil, err
		}
	}
	return res, nil
}

// VerifyBlob checks a message-signature bundle (the kind `cosign sign-blob
// --bundle` writes) against artifact, the exact bytes that were signed.
func (v *Verifier) VerifyBlob(data, artifact []byte) (*Result, error) {
	b, err := parseBundle(data)
	if err != nil {
		return nil, err
	}
	if b.GetMessageSignature() == nil {
		return nil, Errorf(CodeInvalid, "the bundle holds no message signature")
	}
	res := &Result{}
	if err := v.check(b, verify.WithArtifact(bytes.NewReader(artifact)), res); err != nil {
		return nil, err
	}
	return res, nil
}

// IsBlobBundle reports whether data is a message-signature bundle rather than a
// DSSE one.
func IsBlobBundle(data []byte) bool {
	b, err := parseBundle(data)
	return err == nil && b.GetMessageSignature() != nil
}

// check verifies the signature, chain, log proof and timestamps and fills res.
func (v *Verifier) check(b *bundle.Bundle, art verify.ArtifactPolicyOption, res *Result) error {
	vc, err := b.VerificationContent()
	if err != nil {
		return wrap(CodeInvalid, err, "the bundle has no verification material")
	}
	entries, err := b.TlogEntries()
	if err != nil {
		return wrap(CodeInvalid, err, "the bundle's log entries cannot be read")
	}
	res.Logged = len(entries) > 0
	if v.tlog() == TLogRequired && !res.Logged {
		return Errorf(CodeTLogMissing, "the bundle has no transparency log entry and [signing] tlog is %q", TLogRequired)
	}
	if cert := vc.Certificate(); cert != nil {
		return v.verifyKeyless(b, art, res)
	}
	hint := ""
	if pk := vc.PublicKey(); pk != nil {
		hint = pk.Hint()
	}
	return v.verifyKey(b, art, res, hint)
}

func (v *Verifier) verifyKeyless(b *bundle.Bundle, art verify.ArtifactPolicyOption, res *Result) error {
	if v.tlog() == TLogOff {
		return Errorf(CodeInvalid, "a certificate-signed bundle needs a transparency log or timestamp; [signing] tlog = %q only works with keys", TLogOff)
	}
	if v.TrustedRoot == nil {
		return Errorf(CodeRootUnavailable, "no trusted root to verify a certificate-signed bundle; pass --trusted-root or set [signing] trusted_root")
	}
	opts := []verify.VerifierOption{}
	if v.tlog() == TLogRequired {
		opts = append(opts, verify.WithTransparencyLog(1), verify.WithObserverTimestamps(1))
	} else {
		opts = append(opts, verify.WithObserverTimestamps(1))
	}
	if len(v.TrustedRoot.CTLogs()) > 0 {
		opts = append(opts, verify.WithSignedCertificateTimestamps(1))
	}
	out, err := v.run(b, v.TrustedRoot, opts, art, verify.WithoutIdentitiesUnsafe())
	if err != nil {
		return err
	}
	if out.Signature == nil || out.Signature.Certificate == nil {
		return Errorf(CodeInvalid, "the verified bundle carries no certificate")
	}
	res.Signer = SignerInfo{Kind: KindKeyless, Identity: out.Signature.Certificate.SubjectAlternativeName, Issuer: out.Signature.Certificate.Issuer}
	res.SignedAt = earliest(out.VerifiedTimestamps)
	res.Weak = res.SignedAt.IsZero()
	return nil
}

func (v *Verifier) verifyKey(b *bundle.Bundle, art verify.ArtifactPolicyOption, res *Result, hint string) error {
	if len(v.Keys) == 0 {
		return Errorf(CodeSignerNotTrusted, "the bundle is signed with a key and no trusted public key is configured")
	}
	var last error
	for _, key := range v.Keys {
		verifier, err := keyVerifier(key)
		if err != nil {
			return wrap(CodeInvalid, err, "a trusted public key cannot be used")
		}
		material := root.NewTrustedPublicKeyMaterial(func(string) (root.TimeConstrainedVerifier, error) {
			return root.NewExpiringKey(verifier, time.Time{}, time.Time{}), nil
		})
		var trusted root.TrustedMaterial = material
		if v.TrustedRoot != nil {
			trusted = root.TrustedMaterialCollection{material, v.TrustedRoot}
		}
		opts := []verify.VerifierOption{}
		// Under "optional" a log entry that no root can check is ignored, not an
		// error: the key signature is still verified and the time stays unknown
		// (Weak), as for a bundle without a log.
		checkLog := res.Logged && v.tlog() != TLogOff && (v.TrustedRoot != nil || v.tlog() == TLogRequired)
		if checkLog {
			if v.TrustedRoot == nil {
				return Errorf(CodeRootUnavailable, "the bundle has a transparency log entry and no trusted root verifies it")
			}
			opts = append(opts, verify.WithTransparencyLog(1), verify.WithObserverTimestamps(1))
		} else {
			opts = append(opts, verify.WithNoObserverTimestamps())
		}
		out, err := v.run(b, trusted, opts, art, verify.WithKey())
		if err != nil {
			last = err
			continue
		}
		fp, ferr := Fingerprint(key)
		if ferr != nil {
			return wrap(CodeInvalid, ferr, "fingerprint the matched key")
		}
		res.Signer = SignerInfo{Kind: KindKey, KeyID: fp}
		res.SignedAt = earliest(out.VerifiedTimestamps)
		res.Weak = res.SignedAt.IsZero()
		return nil
	}
	if !v.knowsHint(hint) {
		return Errorf(CodeSignerNotTrusted, "the bundle is signed by key %s, which is not a trusted key", hintID(hint))
	}
	return last
}

// knowsHint reports whether the bundle's key hint names one of the trusted keys.
// A hint that matches none (written by a tool with another convention is
// possible) is reported as an untrusted signer only after every key failed.
func (v *Verifier) knowsHint(hint string) bool {
	for _, key := range v.Keys {
		if h, err := hintOf(key); err == nil && string(h) == hint {
			return true
		}
	}
	return false
}

// hintID renders a key hint as a fingerprint when it is a base64 SHA-256.
func hintID(hint string) string {
	if raw, err := base64.StdEncoding.DecodeString(hint); err == nil && len(raw) == 32 {
		return "sha256:" + hexOf(raw)
	}
	return hint
}

func (v *Verifier) run(b *bundle.Bundle, trusted root.TrustedMaterial, opts []verify.VerifierOption, art verify.ArtifactPolicyOption, id verify.PolicyOption) (*verify.VerificationResult, error) {
	sev, err := verify.NewVerifier(trusted, opts...)
	if err != nil {
		return nil, wrap(CodeInvalid, err, "configure verification")
	}
	out, err := sev.Verify(b, verify.NewPolicy(art, id))
	if err != nil {
		return nil, wrap(CodeInvalid, err, "the signature does not verify")
	}
	return out, nil
}

func keyVerifier(pub crypto.PublicKey) (signature.Verifier, error) {
	algo, err := algorithmOf(pub)
	if err != nil {
		return nil, err
	}
	details, err := signature.GetAlgorithmDetails(algo)
	if err != nil {
		return nil, err
	}
	return signature.LoadVerifierFromAlgorithmDetails(pub, details)
}

func earliest(ts []verify.TimestampVerificationResult) time.Time {
	var out time.Time
	for _, t := range ts {
		if out.IsZero() || t.Timestamp.Before(out) {
			out = t.Timestamp
		}
	}
	return out
}

// Inspect reads who a bundle claims to be signed by without verifying anything:
// the certificate identity, or the key hint. It is for display after signing.
func Inspect(data []byte) (SignerInfo, error) {
	b, err := parseBundle(data)
	if err != nil {
		return SignerInfo{}, err
	}
	vc, err := b.VerificationContent()
	if err != nil {
		return SignerInfo{}, wrap(CodeInvalid, err, "the bundle has no verification material")
	}
	if cert := vc.Certificate(); cert != nil {
		sum, err := summarize(cert)
		if err != nil {
			return SignerInfo{}, err
		}
		return sum, nil
	}
	hint := ""
	if pk := vc.PublicKey(); pk != nil {
		hint = pk.Hint()
	}
	return SignerInfo{Kind: KindKey, KeyID: hintID(hint)}, nil
}
