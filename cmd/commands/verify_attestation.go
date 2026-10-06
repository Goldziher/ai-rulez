package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/signing"
)

var (
	verifyAttestation     bool
	verifyAttLock         bool
	verifyAttFile         string
	verifyTrustedRoot     string
	verifyPublicKeys      []string
	verifyIdentity        string
	verifyIssuer          string
	verifyNoState         bool
	verifyFormat          string
	verifyAttestationOnly = []string{"lock", "attestation-file", "trusted-root", "public-key", "identity", "issuer", "no-state"}
)

// attestationReportVersion versions the JSON of `verify --attestation --format json`.
const attestationReportVersion = 1

// attestationReport is the JSON of `verify --attestation` (schema/verify-attestation.schema.json).
type attestationReport struct {
	SchemaVersion int                 `json:"schema_version"`
	Results       []attestationResult `json:"results"`
}

type attestationResult struct {
	Subject     string              `json:"subject"`
	Status      string              `json:"status"`
	Code        string              `json:"code,omitempty"`
	Name        string              `json:"name,omitempty"`
	Reason      string              `json:"reason,omitempty"`
	Attestation string              `json:"attestation,omitempty"`
	Signer      *signing.SignerInfo `json:"signer,omitempty"`
	SignedAt    string              `json:"signed_at,omitempty"`
	Logged      *bool               `json:"logged,omitempty"`
	Weak        *bool               `json:"weak,omitempty"`
	AgeSeconds  *int64              `json:"age_seconds,omitempty"`
	HashVersion int                 `json:"hash_version,omitempty"`
}

// Statuses of a result.
const (
	attestationValid   = "valid"
	attestationInvalid = "invalid"
	attestationMissing = "missing"
)

// rejectAttestationFlags fails when an attestation-only flag is used without --attestation.
func rejectAttestationFlags(cmd *cobra.Command) error {
	if verifyAttestation {
		return nil
	}
	for _, name := range verifyAttestationOnly {
		if cmd.Flags().Changed(name) {
			return oops.Errorf("--%s applies to --attestation", name)
		}
	}
	if verifyFormat != "" {
		return oops.Errorf("--format applies to --attestation")
	}
	return nil
}

func validateVerifyAttestationFlags() error {
	if verifyPlugin || verifyIfConfigured || verifyIfGenerated || verifyRecursive {
		return oops.Errorf("--attestation cannot be combined with --plugin, --if-configured, --if-generated or --recursive")
	}
	if (verifyIdentity == "") != (verifyIssuer == "") {
		return oops.Errorf("--identity and --issuer go together")
	}
	return checkFormatFlag(verifyFormat)
}

// runVerifyAttestation verifies the lock attestation of the project at args[0]
// (or the current directory) offline. Exit codes: 0 verified, 1 the check could
// not run (unreadable lock, no trusted signer, no trusted root), 2 verification
// failed.
func runVerifyAttestation(args []string, env ambient.Env, out io.Writer) int {
	if err := validateVerifyAttestationFlags(); err != nil {
		fmtError(err)
		return 1
	}
	path := ""
	if len(args) > 0 {
		path = args[0]
	}
	cfg, _, err := loadForLockCheck(path)
	if err != nil {
		fmtError(err)
		return 1
	}
	now := time.Now()
	check, err := signing.PrepareLockCheck(cfg, signing.VerifyOptions{
		TrustedRoot: verifyTrustedRoot, PublicKeys: verifyPublicKeys, Identity: verifyIdentity, Issuer: verifyIssuer,
		BundlePath: verifyAttFile, NoState: verifyNoState, Env: env, Now: now,
	})
	if err != nil {
		if code := signing.CodeOf(err); code != "" {
			return reportAttestation(out, failedResult(check, err), now)
		}
		fmtError(err)
		return 1
	}
	for _, w := range check.Warnings {
		fmt.Fprintln(os.Stderr, "warning: "+w)
	}
	report, err := check.Verify()
	if err != nil {
		return reportAttestation(out, failedResult(check, err), now)
	}
	if err := check.Commit(report); err != nil {
		fmt.Fprintln(os.Stderr, "warning: the rollback state was not updated: "+err.Error())
	}
	return reportAttestation(out, validResult(check, report, now), now)
}

func failedResult(check *signing.LockCheck, err error) attestationResult {
	r := attestationResult{Subject: signing.SubjectLock, Status: attestationInvalid, Reason: err.Error()}
	var se *signing.Error
	if errors.As(err, &se) {
		r.Code, r.Name, r.Reason = se.Code, signing.Names[se.Code], se.Reason
		if se.Err != nil {
			r.Reason += ": " + se.Err.Error()
		}
		if se.Code == signing.CodeMissing {
			r.Status = attestationMissing
		}
	}
	if check != nil {
		r.Attestation = check.BundlePath
	}
	return r
}

func validResult(check *signing.LockCheck, rep *signing.LockReport, now time.Time) attestationResult {
	logged, weak := rep.Result.Logged, rep.Result.Weak
	r := attestationResult{
		Subject: signing.SubjectLock, Status: attestationValid, Attestation: check.BundlePath, Signer: &rep.Result.Signer,
		Logged: &logged, Weak: &weak, HashVersion: rep.Predicate.HashVersion,
	}
	if !rep.SigningTime.IsZero() {
		age := int64(now.Sub(rep.SigningTime).Seconds())
		r.AgeSeconds = &age
	}
	if !rep.Result.SignedAt.IsZero() {
		r.SignedAt = rep.Result.SignedAt.UTC().Format(time.RFC3339)
	}
	return r
}

func reportAttestation(out io.Writer, r attestationResult, _ time.Time) int {
	code := 0
	if r.Status != attestationValid {
		code = exitDrift
		if r.Code == signing.CodeRootUnavailable {
			code = 1
		}
	}
	if verifyFormat == formatJSON {
		doc := attestationReport{SchemaVersion: attestationReportVersion, Results: []attestationResult{r}}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(doc); err != nil {
			fmtError(oops.Wrapf(err, "write the report"))
			return 1
		}
		return code
	}
	if r.Status == attestationValid {
		fmt.Fprintf(out, "OK  %s  signer=%s\n    issuer=%s  logged=%s  age=%s  hash_version=%d%s\n", r.Subject, signerText(r.Signer), issuerText(r.Signer), loggedText(r), ageText(r.AgeSeconds), r.HashVersion, weakText(r))
		return code
	}
	fmt.Fprintf(os.Stderr, "FAIL  %s  %s %s: %s\n", r.Subject, r.Code, r.Name, r.Reason)
	return code
}

func signerText(s *signing.SignerInfo) string {
	if s.Kind == signing.KindKey {
		return s.KeyID
	}
	return s.Identity
}

func issuerText(s *signing.SignerInfo) string {
	if s.Kind == signing.KindKey {
		return "none (key)"
	}
	return s.Issuer
}

func loggedText(r attestationResult) string {
	if r.SignedAt != "" {
		return r.SignedAt
	}
	return "no"
}

func weakText(r attestationResult) string {
	if r.Weak != nil && *r.Weak {
		return "\n    no transparency log or timestamp: the signing time is the signer's own claim (weak freshness)"
	}
	return ""
}

func ageText(seconds *int64) string {
	if seconds == nil {
		return "unknown"
	}
	d := time.Duration(*seconds) * time.Second
	if d >= 24*time.Hour {
		return fmt.Sprintf("%dd", d/(24*time.Hour))
	}
	return d.Round(time.Second).String()
}

// signingRequiredLines is what `generate --locked` adds: the attestation
// problems when [signing] require names the lock, one line each.
func signingRequiredLines(cfg *config.Config) []string {
	var lines []string
	for _, e := range signing.RequiredLockFindings(cfg, nil, time.Now()) {
		lines = append(lines, e.Code+" "+signing.Names[e.Code]+": "+e.Reason)
	}
	return lines
}

// checkLockSignatureAt is the attestation half of `lock --check`: 0 when nothing
// is required or the attestation verifies, exitDrift when it does not (AR720 to
// AR727). A configuration that cannot be loaded is not this check's finding:
// checkLockContentAt has already reported it with its own exit code.
func checkLockSignatureAt(path string) int {
	cfg, _, err := loadForLockCheck(path)
	if err != nil {
		return 0
	}
	lines := signingRequiredLines(cfg)
	if len(lines) == 0 {
		return 0
	}
	fmt.Fprintf(os.Stderr, "%s does not carry the signature [signing] require asks for:\n  %s\n", lockfile.FileName, strings.Join(lines, "\n  "))
	return exitDrift
}

// signingFindingsFor returns the AR720 to AR727 findings for strict validation:
// nothing unless [signing] require names the lock.
func signingFindingsFor(cfg *config.Config) []lint.ApprovalFinding {
	lockRel := filepath.ToSlash(filepath.Join(relToBase(cfg, cfg.ConfigDir), lockfile.FileName))
	var out []lint.ApprovalFinding
	for _, e := range signing.RequiredLockFindings(cfg, nil, time.Now()) {
		out = append(out, lint.ApprovalFinding{Code: e.Code, Path: lockRel, Message: e.Reason})
	}
	return out
}
