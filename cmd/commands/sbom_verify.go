package commands

import (
	"errors"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/sbom"
	"github.com/Goldziher/ai-rulez/v5/internal/signing"
)

// lockSignature verifies the lock attestation with the [signing] trust of cfg,
// offline and without touching the rollback state, and reports the outcome for
// the SBOM. A missing attestation is "absent" and a failed verification
// "invalid" (with its AR72x code): neither stops the SBOM. An error means the
// check could not run at all (no lock, no trusted signer).
func lockSignature(cfg *config.Config) (*sbom.Signature, error) {
	now := time.Now()
	check, err := signing.PrepareLockCheck(cfg, signing.VerifyOptions{NoState: true, Now: now})
	if err != nil {
		if signing.CodeOf(err) == "" {
			return nil, err //nolint:wrapcheck // already contextual
		}
		return signatureFailure(err), nil
	}
	report, err := check.Verify()
	if err != nil {
		return signatureFailure(err), nil
	}
	sig := &sbom.Signature{Status: sbom.SignatureVerified, Signer: signerText(&report.Result.Signer), SignedAt: report.Result.SignedAt}
	if report.Result.Signer.Kind != signing.KindKey {
		sig.Issuer = report.Result.Signer.Issuer
	}
	return sig, nil
}

func signatureFailure(err error) *sbom.Signature {
	var se *signing.Error
	if !errors.As(err, &se) {
		return &sbom.Signature{Status: sbom.SignatureInvalid}
	}
	if se.Code == signing.CodeMissing {
		return &sbom.Signature{Status: sbom.SignatureAbsent}
	}
	return &sbom.Signature{Status: sbom.SignatureInvalid, Code: se.Code}
}
