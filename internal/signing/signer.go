package signing

import (
	"context"
	"time"

	"github.com/samber/oops"
	protobundle "github.com/sigstore/protobuf-specs/gen/pb-go/bundle/v1"
	"github.com/sigstore/sigstore-go/pkg/bundle"
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
	Bundle(ctx context.Context, content Content) (*protobundle.Bundle, error)
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
	pb, err := s.Bundle(ctx, &DSSEData{Data: payload, PayloadType: PayloadTypeInToto})
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
func (k *KeySigner) Bundle(ctx context.Context, content Content) (*protobundle.Bundle, error) {
	var rekor *rekorClient
	if k.TLog {
		rekor = newRekor(k.RekorURL)
	}
	return buildBundle(ctx, content, k.Key, nil, "", rekor)
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
	// backoff overrides the delay before the first retry (tests).
	backoff time.Duration
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
func (k *KeylessSigner) Bundle(ctx context.Context, content Content) (*protobundle.Bundle, error) {
	kp, err := newEphemeralKeyPair()
	if err != nil {
		return nil, err
	}
	fulcio := &fulcioClient{newServiceClient(k.opts.FulcioURL)}
	rekor := newRekor(k.opts.RekorURL)
	if k.backoff > 0 {
		fulcio.backoff, rekor.backoff = k.backoff, k.backoff
	}
	return buildBundle(ctx, content, kp, fulcio, k.opts.IDToken, rekor)
}

func newRekor(url string) *rekorClient {
	if url == "" {
		url = DefaultRekorURL
	}
	return &rekorClient{newServiceClient(url)}
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
