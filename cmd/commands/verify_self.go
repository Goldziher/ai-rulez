package commands

import (
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/signing"
)

var verifySelf bool

// checkVerifySelfFlags rejects --self with the modes that verify something else.
func checkVerifySelfFlags(cmd *cobra.Command) error {
	if !verifySelf {
		return nil
	}
	if verifyApprovals || verifyArtifactMode() || verifyPlugin || verifyIfConfigured || verifyIfGenerated || verifyRecursive {
		return oops.Errorf("--self cannot be combined with --approvals, --bundle, --skill, --sbom, --plugin, --if-configured, --if-generated or --recursive")
	}
	for _, name := range []string{"lock", "no-state", keySource, "require-provenance"} {
		if cmd.Flags().Changed(name) {
			return oops.Errorf("--%s does not apply to --self", name)
		}
	}
	if (verifyIdentity == "") != (verifyIssuer == "") {
		return oops.Errorf("--identity and --issuer go together")
	}
	return checkFormatFlag(verifyFormat)
}

// runVerifySelf verifies the ai-rulez binary at exe against the Sigstore bundle
// of its release, offline. Exit codes: 0 official build, 1 the check could not
// run, 2 verification failed. A missing bundle or trusted root is 1: nothing was
// verified, and nothing was found wrong.
func runVerifySelf(exe string, env ambient.Env, out io.Writer) int {
	if exe == "" {
		var err error
		if exe, err = selfExecutable(); err != nil {
			renderError(os.Stderr, err)
			return 1
		}
	}
	now := time.Now()
	rep, err := signing.VerifySelf(signing.SelfOptions{
		Exe: exe, BundlePath: verifyAttFile, TrustedRoot: verifyTrustedRoot, PublicKeys: verifyPublicKeys,
		Identity: verifyIdentity, Issuer: verifyIssuer, Env: env, Now: now,
	})
	if err != nil {
		if signing.CodeOf(err) != "" {
			code := reportAttestation(out, artifactFailure(signing.SubjectRelease, verifyAttFile, err), now)
			if cannotVerify(signing.CodeOf(err)) {
				return 1 // no bundle or no trusted root is "could not check", not a verdict on the binary
			}
			return code
		}
		renderError(os.Stderr, err)
		return 1
	}
	res := artifactSuccess(signing.SubjectRelease, rep.Attestation, "sha256:"+rep.Digest, rep.ArtifactReport, now)
	if verifyFormat != formatJSON {
		reportWriter{out}.printf("%s is an official ai-rulez build.\n", exe)
	}
	return reportAttestation(out, res, now)
}

// cannotVerify reports the codes that mean the check had nothing to verify
// against (no bundle, no trusted root) rather than that the binary failed it.
func cannotVerify(code string) bool {
	return code == signing.CodeMissing || code == signing.CodeRootUnavailable
}

// selfExecutable is the path of the running binary with symlinks resolved, so a
// Homebrew or npm shim is verified as the binary it points at.
func selfExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", oops.Wrapf(err, "locate the running ai-rulez binary")
	}
	if resolved, rerr := filepath.EvalSymlinks(exe); rerr == nil {
		exe = resolved
	}
	return exe, nil
}
