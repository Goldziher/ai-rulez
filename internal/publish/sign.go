package publish

import (
	"context"
	"crypto"
	"time"

	"github.com/samber/oops"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/sign"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/Goldziher/ai-rulez/v5/internal/signing"
)

// SignResult is a signed archive: the Sigstore bundle and who it names.
type SignResult struct {
	Bundle []byte
	Signer SignerInfo
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

// VerifyArchiveSignature checks bundle against archive and then that its signer
// is one the caller trusts. A bundle that names a signer the caller never
// trusted fails: a valid signature alone only says who signed.
func VerifyArchiveSignature(bundle, archive []byte, o VerifyOptions) (*signing.Result, error) {
	tlog := o.TLog
	if tlog == "" {
		tlog = signing.TLogOff
		if len(o.Identities) > 0 {
			tlog = signing.TLogRequired
		}
	}
	v := signing.Verifier{TrustedRoot: o.TrustedRoot, Keys: o.Keys, TLog: tlog}
	res, err := v.VerifyBlob(bundle, archive)
	if err != nil {
		return nil, err //nolint:wrapcheck // a signing.Error carries its AR code
	}
	trust := signing.TrustSet{Entries: append([]signing.TrustEntry(nil), o.Identities...)}
	for _, k := range o.Keys {
		trust.Entries = append(trust.Entries, signing.TrustEntry{Key: k})
	}
	if err := trust.Check(res, "", o.Now); err != nil {
		return nil, err //nolint:wrapcheck // a signing.Error carries its AR code
	}
	return res, nil
}
