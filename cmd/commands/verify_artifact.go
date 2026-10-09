package commands

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/signing"
)

var (
	verifyBundleDir         string
	verifySkillDir          string
	verifySBOMFile          string
	verifySource            string
	verifyRequireProvenance bool
)

// verifyArtifactMode reports whether --bundle, --skill or --sbom names the
// subject; they imply --attestation.
func verifyArtifactMode() bool {
	return verifyBundleDir != "" || verifySkillDir != "" || verifySBOMFile != ""
}

// verifyArtifactSubject returns the subject kind and path the flags name.
func verifyArtifactSubject() (subject, target string, err error) {
	n := 0
	for _, v := range []string{verifyBundleDir, verifySkillDir, verifySBOMFile} {
		if v != "" {
			n++
		}
	}
	if n > 1 {
		return "", "", oops.Errorf("--bundle, --skill and --sbom are mutually exclusive: verify one subject at a time")
	}
	switch {
	case verifyBundleDir != "":
		return signing.SubjectBundle, verifyBundleDir, nil
	case verifySkillDir != "":
		return signing.SubjectSkill, verifySkillDir, nil
	case verifySBOMFile != "":
		return signing.SubjectSBOM, verifySBOMFile, nil
	}
	return signing.SubjectLock, "", nil
}

// signingConfigFor loads the project config whose [signing] policy applies. A
// consumer verifying a downloaded bundle has no project: when none is found and
// the flags name the trusted signer, an empty policy stands in. A project whose
// config cannot be read is an error, never replaced by an empty policy.
func signingConfigFor(path string, flagsTrust bool) (*config.Config, error) {
	if path == "" && flagsTrust && config.ResolveConfigDirName(".") == "" {
		abs, err := filepath.Abs(".")
		if err != nil {
			return nil, oops.Wrapf(err, "resolve the working directory")
		}
		return &config.Config{BaseDir: abs}, nil
	}
	cfg, _, err := loadForLockCheck(path)
	return cfg, err
}

// runVerifyArtifact verifies the attestation of a plugin bundle, a published skill
// or an SBOM file, offline. Exit codes as for the lock: 0 verified, 1 the check
// could not run, 2 verification failed.
func runVerifyArtifact(args []string, env ambient.Env, out io.Writer) int {
	subject, target, err := verifyArtifactSubject()
	if err == nil {
		err = validateVerifyArtifactFlags(subject)
	}
	if err != nil {
		renderError(os.Stderr, err)
		return 1
	}
	path := ""
	if len(args) > 0 {
		path = args[0]
	}
	cfg, err := signingConfigFor(path, len(verifyPublicKeys) > 0 || verifyIdentity != "")
	if err != nil {
		renderError(os.Stderr, err)
		return 1
	}
	now := time.Now()
	check, err := signing.PrepareArtifactCheck(cfg, subject, signing.VerifyOptions{
		TrustedRoot: verifyTrustedRoot, PublicKeys: verifyPublicKeys, Identity: verifyIdentity, Issuer: verifyIssuer,
		NoState: verifyNoState, Env: env, Now: now,
	})
	if err != nil {
		if signing.CodeOf(err) != "" {
			return reportAttestation(out, artifactFailure(subject, "", err), now)
		}
		renderError(os.Stderr, err)
		return 1
	}
	for _, w := range check.Warnings {
		fmt.Fprintln(os.Stderr, "warning: "+w)
	}
	return verifyArtifactWith(out, check, subject, target, now)
}

func validateVerifyArtifactFlags(subject string) error {
	if verifyRequireProvenance && subject != signing.SubjectBundle {
		return oops.Errorf("--require-provenance applies to --bundle")
	}
	if verifySource != "" && subject != signing.SubjectSkill {
		return oops.Errorf("--source applies to --skill")
	}
	return validateVerifyAttestationFlags()
}

// verifyArtifactWith recomputes the artifact's digest, reads its attestation
// files and judges them under check.
func verifyArtifactWith(out io.Writer, check *signing.ArtifactCheck, subject, target string, now time.Time) int {
	var (
		exp  signing.Expectation
		ts   signing.TreeSubject
		path string
		err  error
		hex  string
	)
	if subject == signing.SubjectSBOM {
		var fs signing.FileSubject
		if fs, err = signing.ReadFileSubject(target); err == nil {
			exp, hex, path = signing.SBOMExpectation(fs), fs.DigestHex, target+".sigstore.json"
		}
	} else {
		kind := signing.KindBundleTree
		if subject == signing.SubjectSkill {
			kind = signing.KindSkillTree
		}
		if ts, err = signing.ReadTreeSubject(kind, target); err == nil {
			exp, hex, path = signing.TreeExpectation(subject, ts), ts.HexDigest(), filepath.Join(target, signing.SidecarName)
		}
	}
	if err != nil {
		renderError(os.Stderr, err)
		return 1
	}
	if verifyAttFile != "" {
		path = verifyAttFile
	}
	bundles, err := signing.ReadBundleFiles(path)
	if err != nil {
		return reportAttestation(out, artifactFailure(subject, path, err), now)
	}
	report, err := check.VerifyFor(verifySource, bundles, exp)
	if err != nil {
		return reportAttestation(out, artifactFailure(subject, path, err), now)
	}
	res := artifactSuccess(subject, path, "sha256:"+hex, report, now)
	if subject == signing.SubjectBundle {
		prov, perr := verifyBundleProvenance(check, ts, path)
		if perr != nil {
			return reportAttestation(out, artifactFailure(subject, signing.ProvenanceSidecarFor(path), perr), now)
		}
		res.Provenance = prov
	}
	if err := check.Commit(report); err != nil {
		fmt.Fprintln(os.Stderr, "warning: the rollback state was not updated: "+err.Error())
	}
	return reportAttestation(out, res, now)
}

// verifyBundleProvenance judges the SLSA provenance next to a bundle's
// attestation. It is required by --require-provenance or [signing]
// require_provenance; a provenance file that exists is verified either way, so a
// forged one never passes unexamined.
func verifyBundleProvenance(check *signing.ArtifactCheck, ts signing.TreeSubject, attestation string) (*attestationProvenance, error) {
	path := signing.ProvenanceSidecarFor(attestation)
	required := verifyRequireProvenance || check.RequireProvenance
	bundles, err := signing.ReadBundleFiles(path)
	if err != nil {
		var se *signing.Error
		if errors.As(err, &se) && se.Code == signing.CodeMissing {
			if !required {
				return nil, nil //nolint:nilnil // no provenance and none required
			}
			return nil, signing.Errorf(signing.CodeProvenance, "no SLSA provenance at %s; sign the bundle with `ai-rulez sign --bundle --provenance`", path)
		}
		return nil, err
	}
	rep, prov, err := check.VerifyProvenance(bundles, ts, check.Builders)
	if err != nil {
		return nil, err
	}
	if cerr := check.Commit(rep); cerr != nil {
		fmt.Fprintln(os.Stderr, "warning: the rollback state was not updated: "+cerr.Error())
	}
	return &attestationProvenance{Attestation: path, Builder: prov.RunDetails.Builder.ID, Signer: &rep.Result.Signer}, nil
}

func artifactFailure(subject, path string, err error) attestationResult {
	r := attestationResult{Subject: subject, Status: attestationInvalid, Reason: err.Error(), Attestation: path}
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
	return r
}

func artifactSuccess(subject, path, digest string, rep *signing.ArtifactReport, now time.Time) attestationResult {
	logged, weak := rep.Result.Logged, rep.Result.Weak
	r := attestationResult{
		Subject: subject, Status: attestationValid, Attestation: path, Digest: digest,
		Signer: &rep.Result.Signer, Logged: &logged, Weak: &weak,
	}
	if len(rep.Cosigners) > 0 {
		r.Signers = rep.Signers()
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
