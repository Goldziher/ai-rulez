package signing

import (
	"context"
	"crypto"
	"io"
	"os"
	"strings"

	"github.com/samber/oops"
	"github.com/sigstore/sigstore/pkg/signature/kms"

	// The cloud providers register their key URI schemes (awskms://, gcpkms://,
	// azurekms://, hashivault://) with the kms package when imported.
	_ "github.com/sigstore/sigstore/pkg/signature/kms/aws"
	_ "github.com/sigstore/sigstore/pkg/signature/kms/azure"
	_ "github.com/sigstore/sigstore/pkg/signature/kms/gcp"
	_ "github.com/sigstore/sigstore/pkg/signature/kms/hashivault"
)

// IsKMSRef reports whether key names a key held by a KMS (a cosign key URI such
// as awskms:///alias/release, gcpkms://projects/..., azurekms://vault/key,
// hashivault://key) rather than a key file: it has a URI scheme and is not an
// existing file.
func IsKMSRef(key string) bool {
	if !strings.Contains(key, "://") {
		return false
	}
	_, err := os.Stat(key)
	return err != nil
}

// KMSProviders lists the key URI schemes this build can sign with.
func KMSProviders() []string { return kms.SupportedProviders() }

// LoadKMSSigner opens the KMS key named by ref and returns a signer that works
// like a key signer (offline except for the KMS calls; optionally logged in
// Rekor). The private key never leaves the KMS: ai-rulez sends a digest and
// receives a signature. Credentials come from the provider's usual ambient
// configuration (AWS_*, GOOGLE_APPLICATION_CREDENTIALS, AZURE_*, VAULT_*); none
// is read from a flag. The key must be ECDSA (P-256 recommended) or ed25519.
func LoadKMSSigner(ctx context.Context, ref string) (*KeySigner, error) {
	sv, err := kms.Get(ctx, ref, crypto.SHA256)
	if err != nil {
		return nil, oops.Hint("supported key URI schemes: "+strings.Join(kms.SupportedProviders(), ", ")).
			Errorf("open the KMS key: %s", redactKMSError(ref, err))
	}
	cs, _, err := sv.CryptoSigner(ctx, func(error) {})
	if err != nil {
		return nil, oops.Errorf("use the KMS key to sign: %s", redactKMSError(ref, err))
	}
	pub := cs.Public()
	if pub == nil {
		return nil, oops.Errorf("the KMS did not return the public key of %s", redactKMSRef(ref))
	}
	kp, err := NewKeyPair(&cachedKeySigner{Signer: cs, pub: pub})
	if err != nil {
		return nil, err
	}
	return &KeySigner{Key: kp}, nil
}

// redactKMSRef drops the query string of a key reference, which some providers
// use for options that may carry a token.
func redactKMSRef(ref string) string {
	before, _, _ := strings.Cut(ref, "?")
	return before
}

// redactKMSError is err's text with the key reference's query string removed:
// providers echo the reference they were given, and an option in it may be a token.
func redactKMSError(ref string, err error) string {
	return strings.ReplaceAll(err.Error(), ref, redactKMSRef(ref))
}

// cachedKeySigner answers Public from memory: the KMS crypto.Signer asks the
// service on every call, and a bundle needs the key several times.
type cachedKeySigner struct {
	crypto.Signer
	pub crypto.PublicKey
}

func (c *cachedKeySigner) Public() crypto.PublicKey { return c.pub }

// Sign delegates to the KMS.
func (c *cachedKeySigner) Sign(r io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	sig, err := c.Signer.Sign(r, digest, opts)
	if err != nil {
		return nil, oops.Wrapf(err, "the KMS refused to sign")
	}
	return sig, nil
}
