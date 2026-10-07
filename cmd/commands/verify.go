package commands

import (
	"errors"
	"os"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

var verifyPlugin bool
var verifyIfConfigured bool
var verifyRecursive bool
var verifyIfGenerated bool

// VerifyCmd verifies generated artifacts without modifying them.
var VerifyCmd = &cobra.Command{
	Use:   "verify [config-file]",
	Short: "Verify generated artifacts",
	Long: `Verify generated files without modifying them.

Without --plugin, every file in the generated manifest must exist and still match
the Content-Hash in its own header, which catches hand edits and deleted files
offline. Use "generate --check" to also catch sources that changed since the last
generate. With --plugin, generated plugin bundles are checked against their
provenance hashes.

With --attestation, verify instead checks the Sigstore bundle that "ai-rulez sign
--lock" wrote against the lock and the [signing] policy, offline: the signature,
who signed (identity and issuer, or key), the log proof, freshness and rollback.
--bundle <dir>, --skill <dir> and --sbom <file> verify the attestation of a plugin
bundle, a published skill or an SBOM file the same way (they imply --attestation;
--public-key or --identity with --issuer can stand in for a project's [signing]
table). See docs/signing.md.

With --self, verify the running ai-rulez binary against the Sigstore bundle its release
published (an SLSA provenance statement naming the binary's sha256, signed by the
release workflow; default next to the binary as <binary>.sigstore.json, else
--attestation-file). It needs the cached trusted root ('ai-rulez trust update'),
changes nothing and is independent of any project. --public-key or --identity with
--issuer replace the pinned release identity, to verify a fork's build. See
docs/signing.md.

With --approvals, verify instead re-checks the signed approvals recorded in the lock
against their attestations and the [[signing.trust]] entries for approvals, offline;
--online also asks the forge whether each review-linked approval still holds. See
docs/approvals.md.

Exit codes: 0 verified, 1 the check could not run, 2 generated files differ (with
--attestation: the attestation failed verification; with --approvals: an approval
no longer holds).`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := rejectApprovalFlags(cmd); err != nil {
			fmtError(err)
			os.Exit(1)
		}
		if err := checkVerifySelfFlags(cmd); err != nil {
			fmtError(err)
			os.Exit(1)
		}
		if verifySelf {
			os.Exit(runVerifySelf("", nil, cmd.OutOrStdout()))
		}
		if verifyApprovals {
			os.Exit(runVerifyApprovals(args, cmd.OutOrStdout()))
		}
		if verifyArtifactMode() {
			verifyAttestation = true // --bundle, --skill and --sbom imply --attestation
		}
		if err := rejectAttestationFlags(cmd); err != nil {
			fmtError(err)
			os.Exit(1)
		}
		if verifyAttestation {
			os.Exit(runVerifyAttestation(args, nil, cmd.OutOrStdout()))
		}
		if !verifyPlugin {
			if verifyIfConfigured || verifyIfGenerated {
				fmtError(oops.Errorf("--if-configured and --if-generated only apply to --plugin"))
				os.Exit(1)
			}
			if code := runDriftCheck(args, verifyRecursive, driftManifest); code != 0 {
				os.Exit(code)
			}
			return
		}
		if verifyRecursive {
			runRecursivePluginVerify()
			return
		}
		cfg, err := loadConfigForCommand(cmdContext(), args, config.WithoutLocal())
		if err != nil {
			fmtError(err)
			os.Exit(1)
		}
		if err := cfg.Validate(); err != nil {
			fmtError(err)
			os.Exit(1)
		}
		if verifyIfConfigured && !cfg.HasPluginAuthoring() {
			logger.Info("Skipping plugin verification: no plugin authoring configuration")
			return
		}
		if err := generator.NewGenerator(cfg).VerifyPlugin(profile); err != nil {
			if verifyIfGenerated && errors.Is(err, generator.ErrPluginNotGenerated) {
				logger.Info("Skipping plugin verification: the plugin bundle has not been generated")
				return
			}
			fmtError(err)
			os.Exit(1)
		}
		logger.Success("Generated plugin artifacts are valid", "path", cfg.BaseDir)
	},
}

func init() {
	VerifyCmd.Flags().BoolVar(&verifyPlugin, "plugin", false, "Verify generated plugin bundles using provenance hashes")
	VerifyCmd.Flags().BoolVar(&verifyIfConfigured, "if-configured", false, "Skip plugin verification when no plugin authoring configuration is present")
	VerifyCmd.Flags().BoolVar(&verifyIfGenerated, "if-generated", false, "Skip plugin verification when the plugin bundle has not been generated yet")
	VerifyCmd.Flags().BoolVarP(&verifyRecursive, "recursive", "r", false, "Verify plugin outputs for configurations recursively")
	f := VerifyCmd.Flags()
	f.BoolVar(&verifyAttestation, "attestation", false, "Verify the signed lock (ai-rulez.lock.sigstore.json) offline against the [signing] policy")
	f.Bool("lock", false, "With --attestation: verify the lock attestation (the default and only subject)")
	f.StringVar(&verifyAttFile, "attestation-file", "", "With --attestation: the bundle to verify (default: next to the lock)")
	f.StringVar(&verifyTrustedRoot, "trusted-root", "", "With --attestation: Sigstore trusted root file (default: [signing] trusted_root, else the cache of 'ai-rulez trust update')")
	f.StringArrayVar(&verifyPublicKeys, "public-key", nil, "With --attestation: also trust this PEM public key for the lock (repeatable)")
	f.StringVar(&verifyIdentity, "identity", "", "With --attestation: also trust this certificate identity (needs --issuer)")
	f.StringVar(&verifyIssuer, "issuer", "", "With --attestation: the OIDC issuer of --identity")
	f.BoolVar(&verifySelf, "self", false, "Verify this ai-rulez binary against its release's Sigstore bundle (offline)")
	f.BoolVar(&verifyApprovals, "approvals", false, "Re-check the signed and review-linked approvals that apply to the current content")
	f.BoolVar(&verifyOnline, "online", false, "With --approvals: also check review-linked approvals against the forge (needs the network and a token)")
	f.BoolVar(&verifyNoState, "no-state", false, "With --attestation: do not read or update the per-user rollback state")
	f.StringVar(&verifyBundleDir, "bundle", "", "Verify the attestation of this plugin bundle directory (implies --attestation)")
	f.StringVar(&verifySkillDir, "skill", "", "Verify the publisher attestation of this skill directory (implies --attestation)")
	f.StringVar(&verifySBOMFile, "sbom", "", "Verify the attestation of this SBOM file (implies --attestation)")
	f.StringVar(&verifySource, "source", "", "With --skill: the skill source or installed skill name, to apply [[signing.trust]] entries scoped by source")
	f.BoolVar(&verifyRequireProvenance, "require-provenance", false, "With --bundle: require a verified SLSA provenance statement next to the bundle attestation")
	addFormatFlag(f, &verifyFormat, "", formatText, formatText, formatJSON)
	VerifyCmd.Flags().StringVarP(&profile, "profile", "p", "", "Profile used to generate the plugin bundle")
	VerifyCmd.Flags().StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
}
