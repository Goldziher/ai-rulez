package sigstore

import (
	"context"
	"time"

	"github.com/samber/oops"
	protobundle "github.com/sigstore/protobuf-specs/gen/pb-go/bundle/v1"
	"github.com/sigstore/sigstore-go/pkg/bundle"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/Goldziher/ai-rulez/v5/internal/signing"
)

// Public-good Sigstore endpoints, used by keyless signing unless overridden.
const (
	DefaultFulcioURL = "https://fulcio.sigstore.dev"
	DefaultRekorURL  = "https://rekor.sigstore.dev"
	defaultTimeout   = 30 * time.Second
	defaultRetries   = 2
)

// signDSSE signs payload as a DSSE envelope with s and returns the bundle JSON.
func signDSSE(ctx context.Context, s bundler, payload []byte, payloadType string) ([]byte, error) {
	pb, err := s.Bundle(ctx, &DSSEData{Data: payload, PayloadType: payloadType})
	if err != nil {
		return nil, err
	}
	return encodeBundle(pb)
}

// signBlob signs data as is with s and returns the bundle JSON.
func signBlob(ctx context.Context, s bundler, data []byte) ([]byte, error) {
	pb, err := s.Bundle(ctx, &PlainData{Data: data})
	if err != nil {
		return nil, err
	}
	return encodeBundle(pb)
}

func encodeBundle(pb *protobundle.Bundle) ([]byte, error) {
	data, err := protojson.Marshal(pb)
	if err != nil {
		return nil, oops.Wrapf(err, "encode the bundle")
	}
	return data, nil
}

// bundler builds a Sigstore bundle for content.
type bundler interface {
	Bundle(ctx context.Context, content Content) (*protobundle.Bundle, error)
}

var (
	_ signing.Signer = (*KeySigner)(nil)
	_ signing.Signer = (*KeylessSigner)(nil)
)

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

// SignDSSE implements signing.Signer.
func (k *KeySigner) SignDSSE(ctx context.Context, payload []byte, payloadType string) ([]byte, error) {
	return signDSSE(ctx, k, payload, payloadType)
}

// SignBlob implements signing.Signer.
func (k *KeySigner) SignBlob(ctx context.Context, data []byte) ([]byte, error) {
	return signBlob(ctx, k, data)
}

// Bundle signs content and returns the bundle.
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
	// RetryBackoff overrides the delay before the first retry of a service call.
	// Zero keeps the default.
	RetryBackoff time.Duration
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

// SignDSSE implements signing.Signer.
func (k *KeylessSigner) SignDSSE(ctx context.Context, payload []byte, payloadType string) ([]byte, error) {
	return signDSSE(ctx, k, payload, payloadType)
}

// SignBlob implements signing.Signer.
func (k *KeylessSigner) SignBlob(ctx context.Context, data []byte) ([]byte, error) {
	return signBlob(ctx, k, data)
}

// Bundle signs content and returns the bundle.
func (k *KeylessSigner) Bundle(ctx context.Context, content Content) (*protobundle.Bundle, error) {
	kp, err := newEphemeralKeyPair()
	if err != nil {
		return nil, err
	}
	fulcio := &fulcioClient{newServiceClient(k.opts.FulcioURL)}
	rekor := newRekor(k.opts.RekorURL)
	if k.opts.RetryBackoff > 0 {
		fulcio.backoff, rekor.backoff = k.opts.RetryBackoff, k.opts.RetryBackoff
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
	if len(data) > signing.MaxBundleBytes {
		return nil, signing.Errorf(signing.CodeInvalid, "the bundle is larger than %d bytes", signing.MaxBundleBytes)
	}
	b := &bundle.Bundle{Bundle: new(protobundle.Bundle)}
	if err := b.UnmarshalJSON(data); err != nil {
		return nil, signing.Wrap(signing.CodeInvalid, err, "the file is not a Sigstore bundle")
	}
	return b, nil
}
