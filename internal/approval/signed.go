package approval

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"sync"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
	"github.com/Goldziher/ai-rulez/v5/internal/signing"
)

// SignedVerifier verifies the attestation of a signed approval.
type SignedVerifier interface {
	// Verify checks the attestation of a against the subject s and returns the
	// reviewer identity the signature proves. The record's own reviewer string
	// is never trusted.
	Verify(a lockfile.Approval, s Subject, now time.Time) (reviewer string, err error)
}

// IdentityMapper is a SignedVerifier that knows whom a signing key belongs to.
type IdentityMapper interface {
	// IdentityOf returns the person the [[signing.trust]] entry of the key
	// "key:<fingerprint>" names, "" when it names nobody.
	IdentityOf(reviewer string) string
}

var attestationPattern = regexp.MustCompile(`^sha256:([0-9a-f]{64})$`)

// AttestationFile is the path of the bundle a record's Attestation digest names,
// under the configuration directory. A digest that is not sha256:<64 hex> is
// refused, so the field cannot point outside AttestationDir.
func AttestationFile(configDir, digest string) (string, error) {
	m := attestationPattern.FindStringSubmatch(digest)
	if m == nil {
		return "", fmt.Errorf("attestation %q is not a sha256 digest", digest)
	}
	return filepath.Join(configDir, lockfile.AttestationDir, m[1]+".sigstore.json"), nil
}

// BundleDigest is the "sha256:<hex>" digest a record stores for a bundle.
func BundleDigest(bundle []byte) string {
	sum := sha256.Sum256(bundle)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// configVerifier verifies signed approvals with the [signing] policy of a
// project, preparing it on first use (it may read a trusted root and key files).
type configVerifier struct {
	cfg   *config.Config
	once  sync.Once
	check *signing.ApprovalCheck
	err   error
}

func newConfigVerifier(cfg *config.Config) SignedVerifier { return &configVerifier{cfg: cfg} }

// IdentityOf implements IdentityMapper.
func (v *configVerifier) IdentityOf(reviewer string) string {
	v.once.Do(func() { v.check, v.err = signing.PrepareApprovalCheck(v.cfg, nil) })
	if v.err != nil {
		return ""
	}
	return v.check.KeyReviewers()[reviewer]
}

// maxBundleFileBytes bounds an attestation file; signing.MaxBundleBytes is the same limit.
const maxBundleFileBytes = signing.MaxBundleBytes

func (v *configVerifier) Verify(a lockfile.Approval, s Subject, now time.Time) (string, error) {
	v.once.Do(func() { v.check, v.err = signing.PrepareApprovalCheck(v.cfg, nil) })
	if v.err != nil {
		return "", v.err //nolint:wrapcheck // already contextual
	}
	if a.Attestation == "" {
		return "", errors.New("a signed approval needs an attestation")
	}
	path, err := AttestationFile(v.cfg.ConfigDir, a.Attestation)
	if err != nil {
		return "", err
	}
	data, _, err := safefs.ReadRegularKeepMode(path)
	if err != nil {
		return "", fmt.Errorf("read the attestation %s: %w", filepath.Base(path), err)
	}
	if len(data) > maxBundleFileBytes {
		return "", errors.New("the attestation file is too large")
	}
	if BundleDigest(data) != a.Attestation {
		return "", fmt.Errorf("the attestation file %s does not match the digest in the record", filepath.Base(path))
	}
	res, err := v.check.Verify(data, signing.ApprovalSubject{Kind: s.Kind, Domain: s.Domain, ID: s.ID, Digest: s.Digest}, now)
	if err != nil {
		return "", err //nolint:wrapcheck // *signing.Error carries the code and reason
	}
	if !slices.Equal(sortedStrings(res.Predicate.AcceptedFindings), sortedStrings(a.AcceptedFindings)) || res.Predicate.Expires != a.Expires {
		return "", errors.New("the record's expiry or accepted findings differ from what was signed")
	}
	if !SameReviewer(a.Reviewer, res.Reviewer) {
		return "", fmt.Errorf("the record names reviewer %q but the signer is %q", a.Reviewer, res.Reviewer)
	}
	return res.Reviewer, nil
}

func sortedStrings(in []string) []string {
	out := append([]string(nil), in...)
	slices.Sort(out)
	return out
}

// ReadAttestation reads the bundle a record names, for display.
func ReadAttestation(configDir, digest string) ([]byte, error) {
	path, err := AttestationFile(configDir, digest)
	if err != nil {
		return nil, err
	}
	data, _, err := safefs.ReadRegularKeepMode(path)
	if err != nil {
		return nil, fmt.Errorf("read the attestation: %w", err)
	}
	return data, nil
}
