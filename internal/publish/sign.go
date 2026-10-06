package publish

import (
	"context"
	"crypto"
	"strings"
	"time"

	"github.com/samber/oops"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/sign"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/Goldziher/ai-rulez/v5/internal/signing"
)

// PredicateRelease is the predicate type of the release attestation.
const PredicateRelease = "https://github.com/Goldziher/ai-rulez/attestations/publish/v1"

// SignResult is a signed release: the Sigstore bundle over the archive bytes,
// the signed attestation that binds the rest, and who signed.
type SignResult struct {
	Bundle []byte
	// Attestation is the Sigstore bundle of the DSSE statement ReleaseStatement
	// builds; empty only for an archive signed on its own (SignArchive).
	Attestation []byte
	Signer      SignerInfo
}

// SignRequest is what Build hands the signing callback: the archive and the
// statement that binds the release's name, version and digests.
type SignRequest struct {
	Archive   []byte
	Statement *signing.Statement
}

// FileRef names a release file and its digest in the attestation predicate.
type FileRef struct {
	File   string `json:"file"`
	Digest string `json:"digest"`
}

// ReleasePredicate is the signed claim: which plugin and version the digests
// belong to. A signature over the archive alone does not stop a verifier being
// shown a different name, version, lock or SBOM next to it.
type ReleasePredicate struct {
	Name     string   `json:"name"`
	Version  string   `json:"version"`
	Bundle   FileRef  `json:"bundle"`
	Lock     FileRef  `json:"lock"`
	LockTree string   `json:"lock_tree,omitempty"` //nolint:tagliatelle // matches the manifest's lock.tree naming
	SBOM     *FileRef `json:"sbom,omitempty"`
}

func hexOf(digest string) string { return strings.TrimPrefix(digest, "sha256:") }

// ReleaseStatement builds the in-toto statement a release signs from its
// manifest: the archive, the lock copy and the SBOM are its subjects, and the
// predicate names the plugin and version. The manifest itself cannot be signed
// (it records the signature), so what it claims is bound here instead.
func ReleaseStatement(m Manifest) (*signing.Statement, error) {
	pred := ReleasePredicate{
		Name: m.Name, Version: m.Version, Bundle: FileRef{File: m.Bundle.File, Digest: m.Bundle.Digest},
		Lock: FileRef{File: LockFile, Digest: m.Lock.FileDigest}, LockTree: m.Lock.Tree,
	}
	subjects := []signing.Subject{
		{Name: m.Bundle.File, Digest: map[string]string{"sha256": hexOf(m.Bundle.Digest)}},
		{Name: LockFile, Digest: map[string]string{"sha256": hexOf(m.Lock.FileDigest)}},
	}
	if m.SBOM != nil {
		pred.SBOM = &FileRef{File: m.SBOM.File, Digest: m.SBOM.Digest}
		subjects = append(subjects, signing.Subject{Name: m.SBOM.File, Digest: map[string]string{"sha256": hexOf(m.SBOM.Digest)}})
	}
	st, err := signing.NewStatement(PredicateRelease, subjects, pred)
	if err != nil {
		return nil, oops.Wrapf(err, "build the release statement")
	}
	return st, nil
}

// SignRelease signs the archive (the cosign-compatible message signature) and
// the release statement with signer.
func SignRelease(ctx context.Context, s signing.Signer, req SignRequest) (*SignResult, error) {
	res, err := SignArchive(ctx, s, req.Archive)
	if err != nil {
		return nil, err
	}
	if req.Statement == nil {
		return res, nil
	}
	if res.Attestation, err = signing.SignStatement(ctx, s, req.Statement); err != nil {
		return nil, oops.Wrapf(err, "sign the release attestation")
	}
	return res, nil
}

// SignArchive signs the exact bytes of the release archive with signer and
// returns the Sigstore bundle. The bundle holds a message signature over the
// archive, the form `cosign sign-blob --bundle` writes and `cosign verify-blob
// --bundle` reads, so cosign can verify a release signed here.
func SignArchive(ctx context.Context, s signing.Signer, archive []byte) (*SignResult, error) {
	pb, err := s.Bundle(ctx, &sign.PlainData{Data: archive})
	if err != nil {
		return nil, oops.Wrapf(err, "sign the release archive")
	}
	bundle, err := protojson.Marshal(pb)
	if err != nil {
		return nil, oops.Wrapf(err, "encode the signature bundle")
	}
	info, err := signing.Inspect(bundle)
	if err != nil {
		return nil, oops.Wrapf(err, "read the signer of the new bundle")
	}
	return &SignResult{Bundle: bundle, Signer: SignerInfo{Kind: info.Kind, KeyID: info.KeyID, Identity: info.Identity, Issuer: info.Issuer}}, nil
}

// VerifyOptions say whom to trust when verifying a release signature.
type VerifyOptions struct {
	// Keys are trusted public keys (key-signed bundles).
	Keys []crypto.PublicKey
	// Identities are trusted certificate identities (keyless bundles).
	Identities []signing.TrustEntry
	// TrustedRoot verifies certificates and transparency log entries; nil for key bundles without a log.
	TrustedRoot root.TrustedMaterial
	// TLog is the transparency-log policy; empty means "required" for keyless
	// bundles and "off" when only keys are trusted.
	TLog signing.TLogMode
	Now  time.Time
}

// Trusts reports whether any signer was named.
func (o VerifyOptions) Trusts() bool { return len(o.Keys) > 0 || len(o.Identities) > 0 }

func (o VerifyOptions) verifier() signing.Verifier {
	tlog := o.TLog
	if tlog == "" {
		tlog = signing.TLogOff
		if len(o.Identities) > 0 {
			tlog = signing.TLogRequired
		}
	}
	return signing.Verifier{TrustedRoot: o.TrustedRoot, Keys: o.Keys, TLog: tlog}
}

// trusted checks that the signer of a verified bundle is one the caller named.
func (o VerifyOptions) trusted(res *signing.Result) (*signing.Result, error) {
	trust := signing.TrustSet{Entries: append([]signing.TrustEntry(nil), o.Identities...)}
	for _, k := range o.Keys {
		trust.Entries = append(trust.Entries, signing.TrustEntry{Key: k})
	}
	if err := trust.Check(res, "", o.Now); err != nil {
		return nil, err //nolint:wrapcheck // a signing.Error carries its AR code
	}
	return res, nil
}

// VerifyArchiveSignature checks bundle against archive and then that its signer
// is one the caller trusts. A bundle that names a signer the caller never
// trusted fails: a valid signature alone only says who signed.
func VerifyArchiveSignature(bundle, archive []byte, o VerifyOptions) (*signing.Result, error) {
	v := o.verifier()
	res, err := v.VerifyBlob(bundle, archive)
	if err != nil {
		return nil, err //nolint:wrapcheck // a signing.Error carries its AR code
	}
	return o.trusted(res)
}

// ReleaseFiles are the release files the attestation's digests are checked against.
type ReleaseFiles struct {
	Archive, Lock, SBOM []byte
}

// VerifyReleaseAttestation checks the signed release statement: its signature,
// that the signer is trusted, and that it names the manifest's plugin and
// version and the digests of the files in hand. A manifest relabelled after
// signing, or paired with another archive, lock or SBOM, fails here.
func VerifyReleaseAttestation(bundle []byte, m Manifest, files ReleaseFiles, o VerifyOptions) (*signing.Result, error) {
	v := o.verifier()
	res, err := v.Verify(bundle)
	if err != nil {
		return nil, err //nolint:wrapcheck // a signing.Error carries its AR code
	}
	if res, err = o.trusted(res); err != nil {
		return nil, err
	}
	st := res.Statement
	if st == nil || st.PredicateType != PredicateRelease {
		return nil, oops.Errorf("the attestation is not a release statement")
	}
	var pred ReleasePredicate
	if err := st.DecodePredicate(&pred); err != nil {
		return nil, err //nolint:wrapcheck // a signing.Error carries its AR code
	}
	if pred.Name != m.Name || pred.Version != m.Version {
		return nil, oops.Errorf("the attestation signs %s %s, the manifest names %s %s", pred.Name, pred.Version, m.Name, m.Version)
	}
	for _, c := range []struct{ what, file, got, signed string }{
		{"archive", m.Bundle.File, Digest(files.Archive), pred.Bundle.Digest},
		{"lock copy", LockFile, Digest(files.Lock), pred.Lock.Digest},
	} {
		if c.got != c.signed {
			return nil, oops.Errorf("the %s %s has digest %s, the attestation signs %s", c.what, c.file, c.got, c.signed)
		}
		if err := st.RequireSubject("sha256", hexOf(c.got)); err != nil {
			return nil, err //nolint:wrapcheck // a signing.Error carries its AR code
		}
	}
	switch {
	case m.SBOM == nil && pred.SBOM != nil:
		return nil, oops.Errorf("the attestation signs an SBOM the manifest does not name")
	case m.SBOM != nil && pred.SBOM == nil:
		return nil, oops.Errorf("the manifest names an SBOM the attestation does not sign")
	case m.SBOM != nil && Digest(files.SBOM) != pred.SBOM.Digest:
		return nil, oops.Errorf("the SBOM %s has digest %s, the attestation signs %s", m.SBOM.File, Digest(files.SBOM), pred.SBOM.Digest)
	}
	return res, nil
}
