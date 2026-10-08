package sigstore

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/pem"

	"github.com/samber/oops"
	protocommon "github.com/sigstore/protobuf-specs/gen/pb-go/common/v1"
	"github.com/sigstore/sigstore/pkg/cryptoutils"
	"github.com/sigstore/sigstore/pkg/signature"

	"github.com/Goldziher/ai-rulez/v5/internal/signing"
)

// KeyPair is a signing key (ECDSA P-256, P-384, P-521 or ed25519): a long-lived
// key, or the ephemeral key of a keyless signature.
type KeyPair struct {
	priv    crypto.Signer
	details signature.AlgorithmDetails
	algo    protocommon.PublicKeyDetails
	hint    []byte
}

// algorithmOf maps a public key to the Sigstore algorithm that signs with it.
// RSA is refused: ai-rulez keys are ECDSA or ed25519.
func algorithmOf(pub crypto.PublicKey) (protocommon.PublicKeyDetails, error) {
	if err := signing.CheckKeyType(pub); err != nil {
		return 0, err
	}
	switch k := pub.(type) {
	case *ecdsa.PublicKey:
		switch k.Curve {
		case elliptic.P256():
			return protocommon.PublicKeyDetails_PKIX_ECDSA_P256_SHA_256, nil
		case elliptic.P384():
			return protocommon.PublicKeyDetails_PKIX_ECDSA_P384_SHA_384, nil
		default:
			return protocommon.PublicKeyDetails_PKIX_ECDSA_P521_SHA_512, nil
		}
	default:
		return protocommon.PublicKeyDetails_PKIX_ED25519, nil
	}
}

// NewKeyPair wraps a private key. The key must be ECDSA or ed25519.
func NewKeyPair(priv crypto.PrivateKey) (*KeyPair, error) {
	signer, ok := priv.(crypto.Signer)
	if !ok {
		return nil, oops.Errorf("unsupported private key type %T", priv)
	}
	algo, err := algorithmOf(signer.Public())
	if err != nil {
		return nil, err
	}
	details, err := signature.GetAlgorithmDetails(algo)
	if err != nil {
		return nil, oops.Wrapf(err, "select the signing algorithm")
	}
	hint, err := signing.KeyHint(signer.Public())
	if err != nil {
		return nil, err
	}
	return &KeyPair{priv: signer, details: details, algo: algo, hint: hint}, nil
}

// ParsePrivateKey reads a PEM private key: PKCS#8, SEC 1 EC, or a cosign
// "ENCRYPTED SIGSTORE PRIVATE KEY" decrypted with password. A nil password is
// an error for an encrypted key. The password is never logged.
func ParsePrivateKey(pemBytes, password []byte) (*KeyPair, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, oops.Errorf("the key is not PEM encoded")
	}
	encrypted := block.Type == string(cryptoutils.EncryptedSigstorePrivateKeyPEMType) || block.Type == "ENCRYPTED COSIGN PRIVATE KEY" || block.Type == "ENCRYPTED PRIVATE KEY"
	if encrypted && len(password) == 0 {
		return nil, oops.Hint("set the password variable (--key-password-env, default AI_RULEZ_SIGNING_KEY_PASSWORD or COSIGN_PASSWORD)").Errorf("the signing key is encrypted and no password was given")
	}
	priv, err := cryptoutils.UnmarshalPEMToPrivateKey(pemBytes, func(bool) ([]byte, error) { return password, nil })
	if err != nil {
		return nil, oops.Wrapf(err, "read the signing key")
	}
	return NewKeyPair(priv)
}

// GenerateKeyPair returns a new ECDSA P-256 key and its PEM encodings: the PKCS#8
// private key (encrypted in cosign's format when password is not empty) and the
// public key. It exists for tests and for documentation of the key format.
func GenerateKeyPair(password []byte) (privPEM, pubPEM []byte, err error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, oops.Wrapf(err, "generate a key")
	}
	pubPEM, err = cryptoutils.MarshalPublicKeyToPEM(priv.Public())
	if err != nil {
		return nil, nil, oops.Wrapf(err, "encode the public key")
	}
	if len(password) == 0 {
		privPEM, err = cryptoutils.MarshalPrivateKeyToPEM(priv)
	} else {
		var der []byte
		der, err = cryptoutils.MarshalPrivateKeyToEncryptedDER(priv, func(bool) ([]byte, error) { return password, nil })
		if err == nil {
			privPEM = cryptoutils.PEMEncode(cryptoutils.EncryptedSigstorePrivateKeyPEMType, der)
		}
	}
	if err != nil {
		return nil, nil, oops.Wrapf(err, "encode the private key")
	}
	return privPEM, pubPEM, nil
}

// Key accessors used when building a bundle.

// GetHashAlgorithm returns the digest algorithm the key signs.
func (k *KeyPair) GetHashAlgorithm() protocommon.HashAlgorithm { return k.details.GetProtoHashType() }

// GetSigningAlgorithm returns the Sigstore algorithm of the key.
func (k *KeyPair) GetSigningAlgorithm() protocommon.PublicKeyDetails { return k.algo }

// GetHint returns the key hint written to the bundle.
func (k *KeyPair) GetHint() []byte { return k.hint }

// GetKeyAlgorithm returns the top-level key algorithm.
func (k *KeyPair) GetKeyAlgorithm() string {
	switch k.details.GetKeyType() {
	case signature.ECDSA:
		return "ECDSA"
	case signature.ED25519:
		return "ED25519"
	default:
		return ""
	}
}

// GetPublicKey returns the public key.
func (k *KeyPair) GetPublicKey() crypto.PublicKey { return k.priv.Public() }

// GetPublicKeyPem returns the public key as PEM.
func (k *KeyPair) GetPublicKeyPem() (string, error) {
	b, err := cryptoutils.MarshalPublicKeyToPEM(k.priv.Public())
	return string(b), err
}

// Fingerprint returns the "sha256:<hex>" fingerprint of the public key.
func (k *KeyPair) Fingerprint() string {
	fp, err := signing.Fingerprint(k.priv.Public())
	if err != nil {
		return ""
	}
	return fp
}

// SignData signs data, returning the signature and the bytes that were signed
// (a digest, except for pure ed25519).
func (k *KeyPair) SignData(_ context.Context, data []byte) (sig, signed []byte, err error) {
	hf := k.details.GetHashType()
	toSign := data
	if hf != crypto.Hash(0) {
		h := hf.New()
		h.Write(data)
		toSign = h.Sum(nil)
	}
	sig, err = k.priv.Sign(rand.Reader, toSign, hf)
	if err != nil {
		return nil, nil, err
	}
	return sig, toSign, nil
}
