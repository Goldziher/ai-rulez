package signing

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"

	"github.com/samber/oops"
	protocommon "github.com/sigstore/protobuf-specs/gen/pb-go/common/v1"
	"github.com/sigstore/sigstore/pkg/cryptoutils"
	"github.com/sigstore/sigstore/pkg/signature"
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
	switch k := pub.(type) {
	case *ecdsa.PublicKey:
		switch k.Curve {
		case elliptic.P256():
			return protocommon.PublicKeyDetails_PKIX_ECDSA_P256_SHA_256, nil
		case elliptic.P384():
			return protocommon.PublicKeyDetails_PKIX_ECDSA_P384_SHA_384, nil
		case elliptic.P521():
			return protocommon.PublicKeyDetails_PKIX_ECDSA_P521_SHA_512, nil
		}
		return 0, oops.Errorf("unsupported ECDSA curve %s (use P-256, P-384 or P-521)", k.Curve.Params().Name)
	case ed25519.PublicKey:
		return protocommon.PublicKeyDetails_PKIX_ED25519, nil
	}
	return 0, oops.Errorf("unsupported key type %T (use an ECDSA or ed25519 key)", pub)
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

// hintOf is the hint a bundle carries for a key: base64 of the SHA-256 of its
// DER public key, as cosign and sigstore-go write it.
func hintOf(pub crypto.PublicKey) ([]byte, error) {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, oops.Wrapf(err, "encode the public key")
	}
	sum := sha256.Sum256(der)
	return []byte(base64.StdEncoding.EncodeToString(sum[:])), nil
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
	hint, err := hintOf(signer.Public())
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

// ParsePublicKey reads a PEM public key (ECDSA or ed25519).
func ParsePublicKey(pemBytes []byte) (crypto.PublicKey, error) {
	pub, err := cryptoutils.UnmarshalPEMToPublicKey(pemBytes)
	if err != nil {
		return nil, oops.Wrapf(err, "read the public key")
	}
	if _, err := algorithmOf(pub); err != nil {
		return nil, err
	}
	return pub, nil
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
	fp, _ := Fingerprint(k.priv.Public())
	return fp
}

// SignData signs data, returning the signature and the bytes that were signed
// (a digest, except for pure ed25519).
func (k *KeyPair) SignData(_ context.Context, data []byte) ([]byte, []byte, error) {
	hf := k.details.GetHashType()
	toSign := data
	if hf != crypto.Hash(0) {
		h := hf.New()
		h.Write(data)
		toSign = h.Sum(nil)
	}
	sig, err := k.priv.Sign(rand.Reader, toSign, hf)
	if err != nil {
		return nil, nil, err
	}
	return sig, toSign, nil
}
