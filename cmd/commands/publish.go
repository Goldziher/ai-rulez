package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/publish"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
	"github.com/Goldziher/ai-rulez/v5/internal/signing"
)

var (
	publishTo           string
	publishDist         string
	publishTag          string
	publishRepo         string
	publishFormat       string
	publishChannel      string
	publishDryRun       bool
	publishExecute      bool
	publishYes          bool
	publishConfirmReg   string
	publishForce        bool
	publishAllowDirty   bool
	publishMarketplace  bool
	publishExperimental bool
	publishWithSBOM     bool
	publishPublic       bool
	publishTemplates    []string
	publishRuntimes     []string
	publishOnly         []string
	publishEmit         []string
	publishSince        string
	publishOCIRef       string
	publishNPMScope     string

	publishSignKey         string
	publishSignKeyPassEnv  string
	publishSignTokenEnv    string
	publishFulcioURL       string
	publishRekorURL        string
	publishSignKeyless     bool
	publishSignInteractive bool
	publishSignTLog        bool

	publishVerifyKeys     []string
	publishVerifyIdentity string
	publishVerifyIssuer   string
	publishVerifyRoot     string
	publishVerifyRequire  bool
	publishEmitOut        string
	// publishRunner starts gh, npm and git; nil means a real process. Tests set it.
	publishRunner runner.Runner
)

// ghEnvPass are the variables gh needs to authenticate and reach GitHub. They
// are passed through to gh untouched; ai-rulez never reads their values.
var ghEnvPass = []string{
	"GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN", "GH_HOST",
	"GH_CONFIG_DIR", "XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_DATA_HOME",
	"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "no_proxy",
	"SSL_CERT_FILE", "SSL_CERT_DIR",
}

// npmEnvPass are the variables npm needs to authenticate and reach its
// registry (an .npmrc may interpolate NODE_AUTH_TOKEN). They are passed to npm
// untouched; ai-rulez never reads their values.
var npmEnvPass = []string{
	"NODE_AUTH_TOKEN", "NPM_TOKEN", "NPM_CONFIG_USERCONFIG", "NPM_CONFIG_GLOBALCONFIG", "NPM_CONFIG_REGISTRY",
	"NPM_CONFIG_CACHE", "NPM_CONFIG_PREFIX",
	"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "no_proxy",
	"SSL_CERT_FILE", "SSL_CERT_DIR", "NODE_EXTRA_CA_CERTS",
}

// PublishCmd packages the verified plugin bundle into release artifacts.
var PublishCmd = &cobra.Command{
	Use:   "publish",
	Short: "Package the plugin bundle into deterministic, checksummed release artifacts",
	Long: `Turn the generated plugin bundle into release artifacts in a local dist directory:
a reproducible tar.gz, <name>-<version>.manifest.json, SHA256SUMS, a copy of
ai-rulez.lock, RELEASE_NOTES.md and publish-plan.json.

Preflight runs first and stops before anything is written: validate --strict,
lock --check, verify --plugin and a secret scan of the bundle, then the policy
gates of [publish]: require_signature (AR9N7) and require_approved (AR9N8). The
archive is byte-identical for the same bundle and commit (sorted entries, fixed
mtime, uid/gid 0, normalised modes, no gzip name or time). The mtime is
SOURCE_DATE_EPOCH, else the committer time of HEAD, else 0.

Nothing leaves the machine unless --execute --yes is given with --to:

  github-release   gh release create (gh authenticates itself)
  npm              npm pack, then npm publish of the tarball (npm authenticates itself)
  oci              an OCI artifact pushed with oras-go to --oci-ref or [publish.oci] ref
                   (credentials come from the Docker credential store)

--dry-run prints the artifacts and commands and writes nothing.

--sign-key FILE or --sign-keyless signs the archive (a Sigstore bundle holding a
message signature, the form cosign sign-blob --bundle writes). --sbom ships the
project SBOM. --marketplace writes a Claude marketplace index pinned to the
release commit under marketplace/, one per --channel. --emit NAME runs an
emitter (cursor-team-marketplace is verified; port, aws-agent-registry and
kiro-steering are experimental and need --experimental). --runtime limits the
bundle to some of the plugin runtimes. The release notes list what changed in the
lock since the previous tag (--since TAG chooses another). A [marketplace] with
members or domain plugins publishes one bundle per plugin under plugins/<name>
(--only limits it) plus an aggregate directory.

--template FILE renders a text/template (fields: Name, Version, Tag, Repo,
Commit, AIRulezVersion, Runtimes, BundleFile, BundleDigest, BundleSize,
LockTree, LockDigest, Files; function: json) into <dist>/emit/.

Exit codes: 0 done, 1 the run could not complete, 2 a gate failed.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		exitPublish(runPublish(ctx, cmd.OutOrStdout()))
	},
}

var publishVerifyCmd = &cobra.Command{
	Use:   "verify <dir|oci-ref>",
	Short: "Recompute the checksums, manifest, archive and signature of a release",
	Long: `Verify a dist directory (or a downloaded release, or an OCI reference) offline:
every SHA256SUMS entry, the manifest against the archive's files, the lock copy
against the manifest, the plan against the manifest, and the archive's
determinism rules (sorted entries, uid/gid 0, normalised modes).

An OCI reference (host/path:tag or host/path@sha256:...) is pulled into a
temporary directory first; pin by digest. verify prints the digest a reference resolved
to and warns when it is a tag, which its owner can move. A multi-plugin dist directory verifies
every plugin and the aggregate checksums.

A signed bundle is reported as unverified unless a trusted signer is named:
--key PUBLIC_KEY.pem (repeatable), or --identity and --issuer for a keyless
signature (with --trusted-root, else the root "ai-rulez trust update" cached).
--require-signature fails an unsigned or unverified bundle (AR9N7).

Exit codes: 0 verified, 1 the directory cannot be read, 2 a mismatch.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		exitPublish(runPublishVerify(ctx, cmd.OutOrStdout(), args[0]))
	},
}

var publishEmitCmd = &cobra.Command{
	Use:   "emit <emitter>",
	Short: "Write only the files of one emitter, without a release",
	Long: `Run preflight, build the release in memory and write only the files of one
emitter to --out (default emit/<emitter>). Nothing is uploaded and no dist
directory is written; use it to review or commit what a channel needs.

Emitters: cursor-team-marketplace (verified), port, aws-agent-registry and
kiro-steering (experimental: they need --experimental and carry no vendor
schema to test against).`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		publishEmit = []string{args[0]}
		exitPublish(runPublishEmit(ctx, cmd.OutOrStdout(), args[0]))
	},
}

func init() {
	f := PublishCmd.Flags()
	f.StringVar(&publishTo, "to", "", "Upload target: github-release, npm or oci (default: build only)")
	f.StringVar(&publishDist, "dist", "dist", "Directory the artifacts are written to")
	f.StringVar(&publishTag, "tag", "", "Release tag (default: v<[plugin] version>); with github-release it must already exist on the remote")
	f.StringVar(&publishRepo, "repo", "", "OWNER/REPO of the release (default: [plugin] repository, else the origin remote)")
	f.StringVar(&publishChannel, "channel", "", "Release channel: the pinned index directory (--marketplace) and the npm dist-tag")
	f.BoolVar(&publishMarketplace, "marketplace", false, "Write a Claude marketplace index pinned to the release commit under marketplace/")
	f.StringArrayVar(&publishRuntimes, "runtime", nil, "Publish only these plugin runtimes (repeatable; default: [publish] runtimes, else the [plugin] runtimes)")
	f.StringArrayVar(&publishOnly, "only", nil, "Multi-plugin: publish only the plugin with this name (repeatable)")
	f.StringArrayVar(&publishEmit, "emit", nil, "Run an emitter (repeatable): cursor-team-marketplace, port, aws-agent-registry, kiro-steering")
	f.BoolVar(&publishExperimental, "experimental", false, "Allow emitters whose format is not verified against vendor documentation")
	f.BoolVar(&publishWithSBOM, "sbom", false, "Ship the project SBOM (CycloneDX) with the release")
	f.StringVar(&publishSince, "since", "", "Tag whose lock the release notes diff against (default: the previous tag)")
	f.StringVar(&publishOCIRef, "oci-ref", "", "Repository for --to oci, host/path without a tag (default: [publish.oci] ref)")
	f.StringVar(&publishNPMScope, "npm-scope", "", "npm scope for --to npm, such as @acme (default: [publish.npm] scope)")
	f.BoolVar(&publishPublic, "public", false, "With --to npm: publish with public access (default restricted)")
	f.BoolVar(&publishDryRun, "dry-run", false, "Run preflight and print the artifacts and commands without writing or running anything")
	f.BoolVar(&publishExecute, "execute", false, "Run the upload (needs --to and --yes)")
	f.BoolVar(&publishYes, "yes", false, "Confirm --execute without a prompt")
	f.StringVar(&publishConfirmReg, "confirm-registry", "", "With --to npm: the registry URL [publish.npm] names, confirming it may receive your npm credentials (required for a registry other than the public one)")
	f.BoolVar(&publishForce, "force", false, "With --execute, replace the assets of an existing GitHub release or the artifact an existing OCI tag points at, instead of refusing")
	f.BoolVar(&publishAllowDirty, "allow-dirty", false, "Publish from a tree with uncommitted changes or no commit")
	f.StringArrayVar(&publishTemplates, "template", nil, "Render this text/template into <dist>/emit/ (repeatable)")
	addPublishSignFlags(f)
	f.StringVarP(&profile, "profile", "p", "", "Profile used to generate the plugin bundle")
	f.StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
	addFormatFlag(f, &publishFormat, formatText, formatText, formatText, formatJSON)
	addJSONFlagAlias(f)

	v := publishVerifyCmd.Flags()
	v.StringArrayVar(&publishVerifyKeys, "key", nil, "Trusted PEM public key for the release signature (repeatable)")
	v.StringVar(&publishVerifyIdentity, "identity", "", "Trusted certificate identity of a keyless signature (needs --issuer)")
	v.StringVar(&publishVerifyIssuer, "issuer", "", "OIDC issuer of --identity")
	v.StringVar(&publishVerifyRoot, "trusted-root", "", "Sigstore trusted root file (default: the root from ai-rulez trust update)")
	v.BoolVar(&publishVerifyRequire, "require-signature", false, "Fail an unsigned bundle, or one whose signer is not verified")
	addFormatFlag(v, &publishFormat, formatText, formatText, formatText, formatJSON)
	addJSONFlagAlias(v)

	e := publishEmitCmd.Flags()
	e.StringVar(&publishEmitOut, "out", "", "Directory to write the emitter's files to (default emit/<emitter>)")
	e.BoolVar(&publishExperimental, "experimental", false, "Allow an emitter whose format is not verified against vendor documentation")
	e.StringArrayVar(&publishRuntimes, "runtime", nil, "Use only these plugin runtimes")
	e.StringVar(&publishChannel, "channel", "", "Release channel")
	e.StringVarP(&profile, "profile", "p", "", "Profile used to generate the plugin bundle")
	e.StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
	e.BoolVar(&publishAllowDirty, "allow-dirty", false, "Run from a tree with uncommitted changes or no commit")
	PublishCmd.AddCommand(publishVerifyCmd, publishEmitCmd)
}

func addPublishSignFlags(f interface {
	StringVar(p *string, name, value, usage string)
	BoolVar(p *bool, name string, value bool, usage string)
}) {
	f.StringVar(&publishSignKey, "sign-key", "", "Sign the archive with this PEM private key (ECDSA or ed25519; cosign keys work)")
	f.StringVar(&publishSignKeyPassEnv, "sign-key-password-env", "", "Environment variable holding the key password (default AI_RULEZ_SIGNING_KEY_PASSWORD, then COSIGN_PASSWORD)")
	f.BoolVar(&publishSignKeyless, "sign-keyless", false, "Sign with a short-lived Fulcio certificate and log the signature in Rekor (network; public log)")
	f.StringVar(&publishSignTokenEnv, "sign-token-env", "", "With --sign-keyless: environment variable holding the OIDC token (default: the GitHub Actions runtime token)")
	f.BoolVar(&publishSignInteractive, "sign-interactive", false, "With --sign-keyless: open a browser for the OIDC login when no token is available")
	f.StringVar(&publishFulcioURL, "fulcio-url", "", "With --sign-keyless: Fulcio URL (default "+signing.DefaultFulcioURL+")")
	f.StringVar(&publishRekorURL, "rekor-url", "", "Rekor URL for --sign-keyless or --sign-tlog (default "+signing.DefaultRekorURL+")")
	f.BoolVar(&publishSignTLog, "sign-tlog", false, "With --sign-key: also record the signature in the Rekor transparency log (network; public log)")
}

// exitPublish prints err and exits with the status it carries.
func exitPublish(err error) {
	if err == nil {
		return
	}
	var pe *publish.Error
	if errors.As(err, &pe) {
		fmt.Fprintf(os.Stderr, "Error: %s\n", pe.Error())
		if pe.Hint != "" {
			fmt.Fprintf(os.Stderr, "\nHint: %s\n", pe.Hint)
		}
		os.Exit(pe.Exit)
	}
	fmtError(err)
	os.Exit(1)
}

func checkPublishFlags() error {
	if err := checkFormatFlag(publishFormat); err != nil {
		return err
	}
	if err := validatePublishSignFlags(); err != nil {
		return err
	}
	ghOrPin := publishTo == publish.TargetGitHubRelease || publishMarketplace || len(publishEmit) > 0
	switch {
	case publishTo != "" && !isPublishTarget(publishTo):
		return oops.Errorf("unknown --to %q (use %s)", publishTo, strings.Join(publish.Targets, ", "))
	case publishExecute && publishDryRun:
		return oops.Errorf("--execute and --dry-run cannot be combined")
	case publishExecute && publishTo == "":
		return oops.Errorf("--execute needs --to")
	case publishExecute && !publishYes:
		return oops.Hint("review the commands with --dry-run, then pass --yes").Errorf("--execute needs --yes")
	case publishForce && (!publishExecute || (publishTo != publish.TargetGitHubRelease && publishTo != publish.TargetOCI)):
		return oops.Errorf("--force only applies with --to github-release or --to oci, and --execute")
	case publishTag != "" && !ghOrPin:
		return oops.Errorf("--tag needs --to github-release or --marketplace")
	case publishSince != "" && !publish.ValidSince(publishSince):
		return publish.Errorf(publish.CodeConfig, publish.ExitFailed, "pass a tag name such as v1.3.0", "invalid --since %q", publishSince)
	case publishChannel != "" && !publish.ValidChannel(publishChannel):
		return publish.Errorf(publish.CodeConfig, publish.ExitFailed, "channels are lower-case letters, digits and '-'", "invalid --channel %q", publishChannel)
	case publishOCIRef != "" && publishTo != publish.TargetOCI:
		return oops.Errorf("--oci-ref needs --to oci")
	case (publishNPMScope != "" || publishPublic) && publishTo != publish.TargetNPM:
		return oops.Errorf("--npm-scope and --public need --to npm")
	}
	return nil
}

func isPublishTarget(name string) bool {
	for _, t := range publish.Targets {
		if t == name {
			return true
		}
	}
	return false
}

// verifiedBundle is what the gates verified: the generator and the bundle files of
// every configured runtime, which equal what is on disk.
type verifiedBundle struct {
	gen   *generator.Generator
	files []generator.PluginFile
}

func runPublish(ctx context.Context, out io.Writer) error {
	return runPublishWith(ctx, out, "")
}

// runPublishEmit is `publish emit`: the release is built in memory and only the
// files of one emitter are written, to --out.
func runPublishEmit(ctx context.Context, out io.Writer, name string) error {
	return runPublishWith(ctx, out, name)
}

func runPublishWith(ctx context.Context, out io.Writer, emitOnly string) error {
	if err := checkPublishFlags(); err != nil {
		return err
	}
	cfg, err := loadConfigForCommand(ctx, nil, config.WithoutLocal())
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return publishConfigError(err)
	}
	multi := isMultiPlugin(cfg)
	switch {
	case multi:
	case cfg.Plugin == nil:
		return publish.Errorf(publish.CodeSource, publish.ExitGate, "add a [plugin] block with name and version, or a [marketplace] with members or domain plugins", "no [plugin] block is configured")
	case len(publishOnly) > 0:
		return publish.Errorf(publish.CodeConfig, publish.ExitFailed, "--only selects plugins of a [marketplace] with members or domain plugins", "--only needs a multi-plugin project")
	default:
		if err := publish.ValidateName(cfg.Plugin.Name, cfg.Plugin.Version); err != nil {
			return err //nolint:wrapcheck // a publish.Error carries the exit status
		}
	}
	opts, err := resolvePublishOptions(cfg)
	if err != nil {
		return err
	}
	if emitOnly != "" {
		// `publish emit` builds in memory and writes review files; there is no
		// release to sign, so require_signature has nothing to gate.
		opts.requireSignature = false
	}
	distAbs, err := filepath.Abs(publishDist)
	if err != nil {
		return oops.Wrapf(err, "resolve --dist")
	}
	if opts.requireApproved {
		// Before the strict gate, which would report the same missing approvals as
		// plain findings: AR9N8 names the items and the way out.
		if _, err := approvalGate(cfg, true); err != nil {
			return err
		}
	}
	pre, err := publishPreflight(cfg)
	if err != nil {
		return err
	}
	pc, err := newPublishContext(ctx, cfg, opts, pre, distAbs, multi)
	if err != nil {
		return err
	}
	if multi {
		return runPublishMulti(out, pc, emitOnly)
	}
	return runPublishSingle(ctx, out, pc, emitOnly)
}

func runPublishSingle(ctx context.Context, out io.Writer, pc *publishContext, emitOnly string) error {
	spec, err := pc.singleSpec()
	if err != nil {
		return err
	}
	in, err := pc.newInput(spec)
	if err != nil {
		return err
	}
	pin, err := pc.pinFor(spec.tag)
	if err != nil {
		return err
	}
	in.Pin = pin
	dist, err := publish.Build(*in)
	if err != nil {
		return err //nolint:wrapcheck // a publish.Error carries the exit status
	}
	if emitOnly != "" {
		return pc.writeEmitOnly(out, dist, emitOnly)
	}
	if !publishDryRun {
		if err := dist.Write(pc.distAbs); err != nil {
			return err //nolint:wrapcheck // a publish.Error carries the exit status
		}
	}
	warnAll(dist.Warnings)
	if err := printPublish(out, dist, pc.distAbs); err != nil {
		return err
	}
	if !publishExecute {
		return nil
	}
	return executeDist(ctx, dist, pc.distAbs)
}

// executeDist runs the upload of one written dist directory.
func executeDist(ctx context.Context, d *publish.Dist, dir string) error {
	var (
		result string
		err    error
	)
	switch d.Plan.Target {
	case publish.TargetGitHubRelease:
		result, err = publish.Execute(ctx, publishRunner, d.Plan, publish.ExecuteOptions{
			Dir: dir, Env: runner.ScrubEnv(os.Environ(), ghEnvPass, nil), Force: publishForce,
		})
	case publish.TargetNPM:
		result, err = publish.ExecuteNPM(ctx, publishRunner, d.Plan, publish.NPMExecuteOptions{
			Dir: dir, Env: runner.ScrubEnv(os.Environ(), npmEnvPass, nil), ConfirmRegistry: publishConfirmReg, Notice: func(msg string) { logger.Info(msg) },
		})
	case publish.TargetOCI:
		result, err = publish.ExecuteOCI(ctx, d.Plan, publish.OCIExecuteOptions{Dir: dir, Force: publishForce})
	default:
		return oops.Errorf("the plan has no target to execute")
	}
	if err != nil {
		return err //nolint:wrapcheck // a publish.Error carries the exit status
	}
	fields := []any{"name", d.Plan.Name, "target", d.Plan.Target}
	if result != "" {
		fields = append(fields, "result", result)
	}
	logger.Success("Published", fields...)
	return nil
}

// checkDist runs the checks execution starts with, without uploading anything:
// whether the release, npm version or OCI tag already exists.
func checkDist(ctx context.Context, d *publish.Dist, dir string) error {
	var err error
	switch d.Plan.Target {
	case publish.TargetGitHubRelease:
		err = publish.CheckGitHub(ctx, publishRunner, d.Plan, publish.ExecuteOptions{
			Dir: dir, Env: runner.ScrubEnv(os.Environ(), ghEnvPass, nil), Force: publishForce,
		})
	case publish.TargetNPM:
		err = publish.CheckNPM(ctx, publishRunner, d.Plan, publish.NPMExecuteOptions{
			Dir: dir, Env: runner.ScrubEnv(os.Environ(), npmEnvPass, nil), ConfirmRegistry: publishConfirmReg,
		})
	case publish.TargetOCI:
		_, err = publish.CheckOCI(ctx, d.Plan, publish.OCIExecuteOptions{Dir: dir, Force: publishForce})
	}
	return err //nolint:wrapcheck // a publish.Error carries the exit status
}

func warnAll(warnings []string) {
	for _, w := range warnings {
		logger.Warn(w)
	}
}

// publishPreflight runs the four gates and returns the verified bundle files.
func publishPreflight(cfg *config.Config) (*verifiedBundle, error) {
	if err := strictGate(cfg); err != nil {
		return nil, err
	}
	logger.Info("preflight: validate --strict ok")
	if code := checkLockAt(""); code != 0 {
		exit := publish.ExitFailed
		if code == exitDrift {
			exit = publish.ExitGate
		}
		return nil, publish.Errorf(publish.CodePreflight, exit, "run `ai-rulez lock` after reviewing `ai-rulez lock --diff`", "lock --check failed")
	}
	gen := generator.NewGenerator(cfg)
	if err := gen.VerifyPlugin(profile); err != nil {
		return nil, publish.Errorf(publish.CodePreflight, publish.ExitGate, "run `ai-rulez generate --plugin` and commit the result", "verify --plugin failed: %v", err)
	}
	logger.Info("preflight: verify --plugin ok")
	files, err := gen.PluginFiles(profile)
	if err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = f.Path
	}
	if err := publish.CheckTree(cfg.BaseDir, paths); err != nil {
		return nil, err //nolint:wrapcheck // a publish.Error carries the exit status
	}
	if err := secretGate(files); err != nil {
		return nil, err
	}
	logger.Info("preflight: secret scan ok", "files", len(files))
	return &verifiedBundle{gen: gen, files: files}, nil
}

// strictGate is `validate --strict` as a gate: the same lint, baseline and
// budget handling, failing at the configured threshold but never above error.
func strictGate(cfg *config.Config) error {
	report, err := strictLint(cfg)
	if err != nil {
		return publish.Errorf(publish.CodePreflight, publish.ExitFailed, "", "validate --strict could not run: %v", err)
	}
	reports, cfgs := []*lint.Report{report}, []*config.Config{cfg}
	excess, _, done := prepareReports(reports, cfgs)
	if done {
		return publish.Errorf(publish.CodePreflight, publish.ExitFailed, "", "validate --strict could not run")
	}
	threshold := failOnFor(cfg)
	if threshold == "none" {
		threshold = "error"
	}
	if lint.FailedWithExcess(report.Findings, threshold, budgetsFor(cfg), excess[0]) || baselineBlocks(reports) {
		if werr := lint.Write(os.Stderr, lint.FormatText, lint.Combine(reports), lint.WriteOptions{Version: Version, FailOn: threshold}); werr != nil {
			logger.Warn("Could not print the findings", "error", werr)
		}
		return publish.Errorf(publish.CodePreflight, publish.ExitGate, "fix the findings above (see `ai-rulez validate --strict`)", "validate --strict reported findings")
	}
	return nil
}

// secretGate scans every bundle file with the security scan's secret patterns.
// The report names the file and the pattern, never the value.
func secretGate(files []generator.PluginFile) error {
	var hits []string
	for _, f := range files {
		if name, ok := lint.DetectSecret(string(f.Data)); ok {
			hits = append(hits, f.Path+" ("+name+")")
		}
	}
	if len(hits) == 0 {
		return nil
	}
	return publish.Errorf(publish.CodeSecret, publish.ExitGate, "remove the value, rotate the credential and regenerate the bundle",
		"the bundle contains credentials: %s", strings.Join(hits, ", "))
}

// shippedLock returns the lock bytes that go into the dist directory. Reviewer
// approvals (emails, notes) are repository-internal and outside the tree
// digest, so the shipped copy is the same lock without its [[approval]]
// records: its tree, content pins and output pins are unchanged and
// `lock --check` against it behaves as against the original. A lock without
// approvals ships byte for byte.
func shippedLock(raw []byte, lock *lockfile.File) ([]byte, error) {
	if len(lock.Approval) == 0 {
		return raw, nil
	}
	tmp, err := os.MkdirTemp("", "ai-rulez-lock-*")
	if err != nil {
		return nil, oops.Wrapf(err, "create temporary directory")
	}
	defer os.RemoveAll(tmp) //nolint:errcheck // best effort cleanup of our own directory
	stripped := *lock
	stripped.Approval = nil
	if err := lockfile.Save(tmp, &stripped); err != nil {
		return nil, oops.Wrapf(err, "render the shipped lock copy")
	}
	out, err := os.ReadFile(lockfile.Path(tmp)) //nolint:gosec // the file Save just wrote in our temp directory
	if err != nil {
		return nil, oops.Wrapf(err, "read the shipped lock copy")
	}
	return out, nil
}

// sourceDateEpoch resolves the archive mtime: SOURCE_DATE_EPOCH, else commitTime.
func sourceDateEpoch(commitTime int64) (int64, error) {
	raw := os.Getenv("SOURCE_DATE_EPOCH")
	if raw == "" {
		return commitTime, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 0 {
		return 0, oops.Errorf("SOURCE_DATE_EPOCH must be a non-negative integer, got %q", raw)
	}
	return n, nil
}

func printPublish(out io.Writer, d *publish.Dist, dir string) error {
	if publishFormat == formatJSON {
		_, err := out.Write(d.Files[publish.PlanFile])
		return oops.Wrapf(err, "write plan")
	}
	verb := "wrote"
	if publishDryRun {
		verb = "would write"
	}
	fmt.Fprintf(out, "preflight   validate --strict ok | lock ok | verify --plugin ok | secrets 0\n")
	if m := d.Manifest; m.Signature != nil && publishDryRun {
		fmt.Fprintf(out, "signature   would sign (%s)\n", publishSignFlagText())
	} else if m.Signature != nil {
		fmt.Fprintf(out, "signature   %s (%s)\n", m.Signature.File, publishSignerText(m.Signature.Signer))
	} else if d.Manifest.Name != "" {
		fmt.Fprintf(out, "signature   none\n")
	}
	if a := d.Manifest.Approval; a != nil {
		fmt.Fprintf(out, "approval    %d of %d selected items approved\n", a.Approved, a.Required)
	}
	fmt.Fprintf(out, "artifacts   %s %d files to %s\n", verb, len(d.Files), dir)
	for _, a := range d.Plan.Artifacts {
		fmt.Fprintf(out, "            %-40s %s  %d bytes\n", a.Path, a.Digest, a.Size)
	}
	fmt.Fprintf(out, "            %-40s %s  %d bytes\n", publish.PlanFile, publish.Digest(d.Files[publish.PlanFile]), len(d.Files[publish.PlanFile]))
	if d.Plan.Ref != "" {
		fmt.Fprintf(out, "push        %s (manifest %s)\n", d.Plan.Ref, d.Plan.OCIDigest)
	}
	for _, c := range d.Plan.Commands {
		verb := "would run"
		if publishExecute {
			verb = "running"
		}
		fmt.Fprintf(out, "%-11s %s\n", verb, shellJoin(c.Argv))
	}
	if d.Plan.NPM != nil {
		fmt.Fprintf(out, "registry    %s (npm runs from an empty temporary directory with explicit --userconfig and --globalconfig; no project .npmrc applies)\n",
			publish.NPMEffectiveRegistry(*d.Plan.NPM, runner.ScrubEnv(os.Environ(), npmEnvPass, nil)))
	}
	if d.Plan.Credentials != "" {
		fmt.Fprintf(out, "credentials %s\n", d.Plan.Credentials)
	}
	return nil
}

// publishSignFlagText names the signing flag of a dry run.
func publishSignFlagText() string {
	if publishSignKeyless {
		return "--sign-keyless"
	}
	return "--sign-key"
}

func publishSignerText(s publish.SignerInfo) string {
	if s.Kind == "key" {
		return "key " + s.KeyID
	}
	return s.Identity + ", issuer " + s.Issuer
}

// shellJoin renders argv so it can be pasted into a POSIX shell: arguments with
// characters outside a safe set are single-quoted. The argv itself never goes
// through a shell.
func shellJoin(argv []string) string {
	parts := make([]string, len(argv))
	for i, a := range argv {
		if a != "" && strings.Trim(a, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789@%+=:,./_-") == "" {
			parts[i] = a
			continue
		}
		parts[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
	}
	return strings.Join(parts, " ")
}

func runPublishVerify(ctx context.Context, out io.Writer, target string) error {
	if err := checkFormatFlag(publishFormat); err != nil {
		return err
	}
	trust, err := publishVerifyOptions(nil)
	if err != nil {
		return err
	}
	checks := publish.VerifyChecks{Signature: trust, RequireSignature: publishVerifyRequire}
	dir, cleanup, err := resolveVerifyTarget(ctx, target)
	if err != nil {
		return err
	}
	defer cleanup()
	results, err := verifyTree(dir, checks)
	if err != nil {
		return err //nolint:wrapcheck // a publish.Error carries the exit status
	}
	return reportVerify(out, results, target)
}

// verifyResult is one verified directory of a (possibly multi-plugin) release.
type verifyResult struct {
	Dir string `json:"dir,omitempty"`
	publish.VerifyResult
}

// verifyTree verifies dir; a multi-plugin directory verifies every plugin and
// the aggregate checksums.
func verifyTree(dir string, checks publish.VerifyChecks) ([]verifyResult, error) {
	plugins, err := os.ReadDir(filepath.Join(dir, "plugins"))
	if err != nil || fileExists(filepath.Join(dir, publish.SumsFile)) {
		res, verr := publish.VerifyWith(dir, checks)
		return []verifyResult{{VerifyResult: res}}, verr
	}
	var out []verifyResult
	for _, e := range plugins {
		if !e.IsDir() {
			continue
		}
		sub := filepath.Join("plugins", e.Name())
		res, err := publish.VerifyWith(filepath.Join(dir, sub), checks)
		if err != nil {
			return nil, err //nolint:wrapcheck // a publish.Error carries the exit status
		}
		out = append(out, verifyResult{Dir: filepath.ToSlash(sub), VerifyResult: res})
	}
	if len(out) == 0 {
		return nil, publish.Errorf(publish.CodeVerify, publish.ExitFailed, "run `ai-rulez publish` first", "%s holds no release to verify", dir)
	}
	agg := verifyResult{Dir: "aggregate"}
	if fileExists(filepath.Join(dir, "aggregate", publish.SumsFile)) {
		res, err := publish.VerifySums(filepath.Join(dir, "aggregate"))
		if err != nil {
			return nil, err //nolint:wrapcheck // a publish.Error carries the exit status
		}
		agg.VerifyResult = res
	} else {
		agg.Problems = []publish.Problem{}
	}
	// The aggregate names the plugins of the release: a plugin directory that is
	// gone (or one that was never part of it) is a mismatch, not a clean verify.
	agg.Problems = append(agg.Problems, publish.VerifyPluginList(dir)...)
	return append(out, agg), nil
}

func fileExists(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}

func reportVerify(out io.Writer, results []verifyResult, target string) error {
	problems := 0
	for _, r := range results {
		problems += len(r.Problems)
	}
	if publishFormat == formatJSON {
		var doc any = results[0].VerifyResult
		if len(results) > 1 || results[0].Dir != "" {
			doc = map[string]any{"results": results}
		}
		data, merr := json.MarshalIndent(doc, "", "  ")
		if merr != nil {
			return oops.Wrapf(merr, "encode result")
		}
		if _, werr := out.Write(append(data, '\n')); werr != nil {
			return oops.Wrapf(werr, "write result")
		}
	}
	for _, r := range results {
		label := r.Dir
		if label != "" {
			label += ": "
		}
		if r.OK() && publishFormat != formatJSON {
			if r.Name == "" {
				fmt.Fprintf(out, "%sverified: %d files match SHA256SUMS\n", label, r.Files)
				continue
			}
			fmt.Fprintf(out, "%sverified %s %s: %d files match SHA256SUMS, the manifest and the archive (signature: %s)\n", label, r.Name, r.Version, r.Files, verifyOrNone(r.Signature))
			if r.Signer != "" {
				fmt.Fprintf(out, "%ssigner: %s\n", label, r.Signer)
			}
			continue
		}
		if publishFormat != formatJSON {
			for _, p := range r.Problems {
				fmt.Fprintf(os.Stderr, "%s%s: %s\n", label, p.Path, p.Message)
			}
		}
	}
	if problems > 0 {
		code := publish.CodeVerify
		if signatureProblems(results) {
			code = publish.CodeUnsigned
		}
		return publish.Errorf(code, publish.ExitGate, "", "%d mismatch(es) in %s", problems, target)
	}
	return nil
}

func verifyOrNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

func signatureProblems(results []verifyResult) bool {
	for _, r := range results {
		for _, p := range r.Problems {
			if strings.Contains(p.Message, publish.CodeUnsigned) {
				return true
			}
		}
	}
	return false
}
