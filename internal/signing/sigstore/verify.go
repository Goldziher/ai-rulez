package sigstore

import (
	"bytes"
	"crypto"
	"encoding/base64"
	"time"

	protocommon "github.com/sigstore/protobuf-specs/gen/pb-go/common/v1"
	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/verify"
	"github.com/sigstore/sigstore/pkg/signature"

	"github.com/Goldziher/ai-rulez/v5/internal/signing"
)

// Engine is the Sigstore implementation of signing.Backend: it checks bundles
// with sigstore-go. Install it once at startup with signing.UseBackend(New()).
type Engine struct{}

// New returns the Sigstore backend.
func New() *Engine { return &Engine{} }

var _ signing.Backend = (*Engine)(nil)

// verifier is a signing.Verifier being run.
type verifier struct{ signing.Verifier }

func (v *verifier) tlog() signing.TLogMode {
	if v.TLog == "" {
		return signing.TLogRequired
	}
	return v.TLog
}

// trustedRoot is the configured root, nil when there is none.
func (v *verifier) trustedRoot() root.TrustedMaterial {
	tm, _ := v.TrustedRoot.(root.TrustedMaterial)
	return tm
}

// Verify checks a DSSE bundle offline and returns the signed payload. Failures
// are *Error: AR721 for a bad bundle, signature, chain or proof, AR725 when a
// needed trusted root is missing, AR726 when a required log entry is absent.
func (*Engine) Verify(cfg signing.Verifier, data []byte) (*signing.Result, error) {
	v := &verifier{cfg}
	b, err := parseBundle(data)
	if err != nil {
		return nil, err
	}
	env, err := b.Envelope()
	if err != nil {
		return nil, signing.Wrap(signing.CodeInvalid, err, "the bundle holds no DSSE envelope")
	}
	payload, err := env.DecodeB64Payload()
	if err != nil {
		return nil, signing.Wrap(signing.CodeInvalid, err, "the envelope payload is not base64")
	}
	res := &signing.Result{PayloadType: env.PayloadType, Payload: payload}
	if err := v.check(b, verify.WithoutArtifactUnsafe(), res); err != nil {
		return nil, err
	}
	if res.PayloadType == signing.PayloadTypeInToto {
		if res.Statement, err = signing.ParseStatement(payload); err != nil {
			return nil, err
		}
	}
	return res, nil
}

// VerifyBlob checks a message-signature bundle (the kind `cosign sign-blob
// --bundle` writes) against artifact, the exact bytes that were signed.
func (*Engine) VerifyBlob(cfg signing.Verifier, data, artifact []byte) (*signing.Result, error) {
	v := &verifier{cfg}
	b, err := parseBundle(data)
	if err != nil {
		return nil, err
	}
	if b.GetMessageSignature() == nil {
		return nil, signing.Errorf(signing.CodeInvalid, "the bundle holds no message signature")
	}
	res := &signing.Result{}
	if err := v.check(b, verify.WithArtifact(bytes.NewReader(artifact)), res); err != nil {
		return nil, err
	}
	return res, nil
}

// IsBlobBundle reports whether data is a message-signature bundle rather than a
// DSSE one.
func (*Engine) IsBlobBundle(data []byte) bool {
	b, err := parseBundle(data)
	return err == nil && b.GetMessageSignature() != nil
}

// check verifies the signature, chain, log proof and timestamps and fills res.
func (v *verifier) check(b *bundle.Bundle, art verify.ArtifactPolicyOption, res *signing.Result) error {
	vc, err := b.VerificationContent()
	if err != nil {
		return signing.Wrap(signing.CodeInvalid, err, "the bundle has no verification material")
	}
	entries, err := b.TlogEntries()
	if err != nil {
		return signing.Wrap(signing.CodeInvalid, err, "the bundle's log entries cannot be read")
	}
	res.Logged = len(entries) > 0
	if v.tlog() == signing.TLogRequired && !res.Logged {
		return signing.Errorf(signing.CodeTLogMissing, "the bundle has no transparency log entry and [signing] tlog is %q", signing.TLogRequired)
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

func (v *verifier) verifyKeyless(b *bundle.Bundle, art verify.ArtifactPolicyOption, res *signing.Result) error {
	if v.tlog() == signing.TLogOff {
		return signing.Errorf(signing.CodeInvalid, "a certificate-signed bundle needs a transparency log or timestamp; [signing] tlog = %q only works with keys", signing.TLogOff)
	}
	if v.trustedRoot() == nil {
		return signing.Errorf(signing.CodeRootUnavailable, "no trusted root to verify a certificate-signed bundle; pass --trusted-root or set [signing] trusted_root")
	}
	opts := []verify.VerifierOption{}
	if v.tlog() == signing.TLogRequired {
		opts = append(opts, verify.WithTransparencyLog(1), verify.WithObserverTimestamps(1))
	} else {
		opts = append(opts, verify.WithObserverTimestamps(1))
	}
	if len(v.trustedRoot().CTLogs()) > 0 {
		opts = append(opts, verify.WithSignedCertificateTimestamps(1))
	}
	out, err := v.run(b, v.trustedRoot(), opts, art, verify.WithoutIdentitiesUnsafe())
	if err != nil {
		return err
	}
	if out.Signature == nil || out.Signature.Certificate == nil {
		return signing.Errorf(signing.CodeInvalid, "the verified bundle carries no certificate")
	}
	res.Signer = signing.SignerInfo{Kind: signing.KindKeyless, Identity: out.Signature.Certificate.SubjectAlternativeName, Issuer: out.Signature.Certificate.Issuer}
	res.SignedAt = earliest(out.VerifiedTimestamps)
	res.Weak = res.SignedAt.IsZero()
	return nil
}

func (v *verifier) verifyKey(b *bundle.Bundle, art verify.ArtifactPolicyOption, res *signing.Result, hint string) error {
	if len(v.Keys) == 0 {
		return signing.Errorf(signing.CodeSignerNotTrusted, "the bundle is signed with a key and no trusted public key is configured")
	}
	var last error
	for _, key := range v.Keys {
		verifier, err := keyVerifier(key)
		if err != nil {
			return signing.Wrap(signing.CodeInvalid, err, "a trusted public key cannot be used")
		}
		material := root.NewTrustedPublicKeyMaterial(func(string) (root.TimeConstrainedVerifier, error) {
			return root.NewExpiringKey(verifier, time.Time{}, time.Time{}), nil
		})
		var trusted root.TrustedMaterial = material
		if v.trustedRoot() != nil {
			trusted = root.TrustedMaterialCollection{material, v.trustedRoot()}
		}
		opts := []verify.VerifierOption{}
		// Under "optional" a log entry that no root can check is ignored, not an
		// error: the key signature is still verified and the time stays unknown
		// (Weak), as for a bundle without a log.
		checkLog := res.Logged && v.tlog() != signing.TLogOff && (v.trustedRoot() != nil || v.tlog() == signing.TLogRequired)
		if checkLog {
			if v.trustedRoot() == nil {
				return signing.Errorf(signing.CodeRootUnavailable, "the bundle has a transparency log entry and no trusted root verifies it")
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
		fp, ferr := signing.Fingerprint(key)
		if ferr != nil {
			return signing.Wrap(signing.CodeInvalid, ferr, "fingerprint the matched key")
		}
		res.Signer = signing.SignerInfo{Kind: signing.KindKey, KeyID: fp}
		res.SignedAt = earliest(out.VerifiedTimestamps)
		res.Weak = res.SignedAt.IsZero()
		return nil
	}
	if !v.knowsHint(hint) {
		return signing.Errorf(signing.CodeSignerNotTrusted, "the bundle is signed by key %s, which is not a trusted key", hintID(hint))
	}
	return last
}

// knowsHint reports whether the bundle's key hint names one of the trusted keys.
// A hint that matches none (written by a tool with another convention is
// possible) is reported as an untrusted signer only after every key failed.
func (v *verifier) knowsHint(hint string) bool {
	for _, key := range v.Keys {
		if h, err := signing.KeyHint(key); err == nil && string(h) == hint {
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

func (v *verifier) run(b *bundle.Bundle, trusted root.TrustedMaterial, opts []verify.VerifierOption, art verify.ArtifactPolicyOption, id verify.PolicyOption) (*verify.VerificationResult, error) {
	sev, err := verify.NewVerifier(trusted, opts...)
	if err != nil {
		return nil, signing.Wrap(signing.CodeInvalid, err, "configure verification")
	}
	out, err := sev.Verify(b, verify.NewPolicy(art, id))
	if err != nil {
		return nil, signing.Wrap(signing.CodeInvalid, err, "the signature does not verify")
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
func (*Engine) Inspect(data []byte) (signing.SignerInfo, error) {
	b, err := parseBundle(data)
	if err != nil {
		return signing.SignerInfo{}, err
	}
	vc, err := b.VerificationContent()
	if err != nil {
		return signing.SignerInfo{}, signing.Wrap(signing.CodeInvalid, err, "the bundle has no verification material")
	}
	if cert := vc.Certificate(); cert != nil {
		sum, err := summarize(cert)
		if err != nil {
			return signing.SignerInfo{}, err
		}
		return sum, nil
	}
	hint := ""
	if pk := vc.PublicKey(); pk != nil {
		hint = pk.Hint()
	}
	return signing.SignerInfo{Kind: signing.KindKey, KeyID: hintID(hint)}, nil
}

// MessageDigest implements signing.Backend.
func (*Engine) MessageDigest(data []byte) ([]byte, bool, error) {
	b, err := parseBundle(data)
	if err != nil {
		return nil, false, err
	}
	md := b.GetMessageSignature().GetMessageDigest()
	if md == nil {
		return nil, false, nil
	}
	return md.Digest, md.Algorithm == protocommon.HashAlgorithm_SHA2_256, nil
}

// LoadTrustedRoot implements signing.Backend.
func (*Engine) LoadTrustedRoot(data []byte) (signing.TrustedMaterial, error) {
	tr, err := root.NewTrustedRootFromJSON(data)
	if err != nil {
		return nil, err //nolint:wrapcheck // the caller adds the AR725 context
	}
	return tr, nil
}
