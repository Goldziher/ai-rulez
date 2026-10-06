package signing

import (
	"context"
	"time"

	"github.com/samber/oops"
	protobundle "github.com/sigstore/protobuf-specs/gen/pb-go/bundle/v1"
	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/sign"
	"google.golang.org/protobuf/encoding/protojson"
)

// Public-good Sigstore endpoints, used by keyless signing unless overridden.
const (
	DefaultFulcioURL = "https://fulcio.sigstore.dev"
	DefaultRekorURL  = "https://rekor.sigstore.dev"
	defaultTimeout   = 30 * time.Second
	defaultRetries   = 2
)

// Signer turns a DSSE payload into a Sigstore bundle.
type Signer interface {
	// Bundle signs content and returns the bundle.
	Bundle(ctx context.Context, content sign.Content) (*protobundle.Bundle, error)
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
	pb, err := s.Bundle(ctx, &sign.DSSEData{Data: payload, PayloadType: PayloadTypeInToto})
	if err != nil {
		return nil, oops.Wrapf(err, "sign the attestation")
	}
	data, err := protojson.Marshal(pb)
	if err != nil {
		return nil, oops.Wrapf(err, "encode the bundle")
	}
	return data, nil
}

// KeySigner signs with a long-lived key. It works offline unless TLog is set.
type KeySigner struct {
	Key *KeyPair
	// TLog also records the signature in a Rekor log (network).
	TLog     bool
	RekorURL string
}

// LoadKeySigner builds a KeySigner from a PEM private key (see ParsePrivateKey).
func LoadKeySigner(pemBytes, password []byte) (*KeySigner, error) {
	kp, err := ParsePrivateKey(pemBytes, password)
	if err != nil {
		return nil, err
	}
	return &KeySigner{Key: kp}, nil
}

// Bundle implements Signer.
func (k *KeySigner) Bundle(ctx context.Context, content sign.Content) (*protobundle.Bundle, error) {
	opts := sign.BundleOptions{Context: ctx}
	if k.TLog {
		opts.TransparencyLogs = []sign.Transparency{newRekor(k.RekorURL)}
	}
	return sign.Bundle(content, k.Key, opts)
}

// KeylessOptions configures keyless signing.
type KeylessOptions struct {
	// IDToken is the OIDC token Fulcio exchanges for a certificate.
	IDToken string
	// FulcioURL and RekorURL default to the public-good instances.
	FulcioURL string
	RekorURL  string
}

// KeylessSigner signs with an ephemeral key certified by Fulcio and logs the
// signature in Rekor. The token, certificate and subject digest leave the machine.
type KeylessSigner struct {
	opts KeylessOptions
}

// NewKeylessSigner validates the options.
func NewKeylessSigner(opts KeylessOptions) (*KeylessSigner, error) {
	if opts.IDToken == "" {
		return nil, oops.Errorf("keyless signing needs an OIDC identity token")
	}
	if opts.FulcioURL == "" {
		opts.FulcioURL = DefaultFulcioURL
	}
	return &KeylessSigner{opts: opts}, nil
}

// Bundle implements Signer.
func (k *KeylessSigner) Bundle(ctx context.Context, content sign.Content) (*protobundle.Bundle, error) {
	kp, err := sign.NewEphemeralKeypair(nil)
	if err != nil {
		return nil, oops.Wrapf(err, "generate an ephemeral key")
	}
	fulcio := sign.NewFulcio(&sign.FulcioOptions{BaseURL: k.opts.FulcioURL, Timeout: defaultTimeout, Retries: defaultRetries})
	return sign.Bundle(content, kp, sign.BundleOptions{
		Context:                    ctx,
		CertificateProvider:        fulcio,
		CertificateProviderOptions: &sign.CertificateProviderOptions{IDToken: k.opts.IDToken},
		TransparencyLogs:           []sign.Transparency{newRekor(k.opts.RekorURL)},
	})
}

func newRekor(url string) sign.Transparency {
	if url == "" {
		url = DefaultRekorURL
	}
	return sign.NewRekor(&sign.RekorOptions{BaseURL: url, Timeout: defaultTimeout, Retries: defaultRetries})
}

// parseBundle decodes bundle JSON with the size bound applied.
func parseBundle(data []byte) (*bundle.Bundle, error) {
	if len(data) > MaxBundleBytes {
		return nil, Errorf(CodeInvalid, "the bundle is larger than %d bytes", MaxBundleBytes)
	}
	b := &bundle.Bundle{Bundle: new(protobundle.Bundle)}
	if err := b.UnmarshalJSON(data); err != nil {
		return nil, wrap(CodeInvalid, err, "the file is not a Sigstore bundle")
	}
	return b, nil
}
