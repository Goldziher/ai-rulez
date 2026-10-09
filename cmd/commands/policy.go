package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/policy"
	"github.com/Goldziher/ai-rulez/v5/internal/project"
	"github.com/samber/oops"
)

// policyFlag is --policy: a policy file the organization owns (a CI step, a
// managed wrapper). It is a persistent flag, so every command that loads a
// configuration honors it.
var policyFlag string

// policyModeFlag is --policy-mode: "enforce" (default) or "warn".
var policyModeFlag string

// URL policy flags: the digest the --policy URL must have, offline use of the
// cached copy, how stale that copy may be, and trust-on-first-use.
var (
	policyDigestFlag   string
	policyOfflineFlag  bool
	policyMaxStaleFlag string
	policyTOFUFlag     bool
	discoverOrgFlag    bool

	// Signature flags: who may sign a policy and whether a signature is required.
	policyRequireSignedFlag bool
	policySignerKeyFlag     []string
	policySignerIDFlag      string
	policySignerIssuerFlag  string
	policyTrustedRootFlag   string
)

// validateShowPolicy is validate --show-policy.
var validateShowPolicy bool

// policyEnforcer discovers the policy lazily, from the flag, AI_RULEZ_POLICY
// and the managed location, never from the repository being loaded.
var policyEnforcer = policy.NewEnforcer(func() policy.DiscoverOptions {
	return policy.DiscoverOptions{
		Flag: policyFlag, FlagDigest: policyDigestFlag, Mode: policyModeFlag,
		Offline: policyOfflineFlag, MaxStale: policyMaxStaleFlag,
		TrustOnFirstUse: policyTOFUFlag, Interactive: stdinIsTerminal(), DiscoverOrg: discoverOrgFlag,
		Signature: policy.SignatureOptions{
			Require: policyRequireSignedFlag, Identity: policySignerIDFlag, Issuer: policySignerIssuerFlag,
			KeyFiles: policySignerKeyFlag, TrustedRoot: policyTrustedRootFlag,
		},
	}
})

// activePolicy is the policy every load the command line makes runs under. The
// library never reads a process-wide policy: this is the CLI handing its own to
// each load (tests swap it).
var activePolicy config.PolicyEnforcer = policyEnforcer

// cmdContext is the root context of a command that has no cobra context to
// inherit: it carries the command line's policy, so the loads, CRUD operations and
// MCP setup made with it are bound by the organization policy. Commands never use
// a bare context.Background() (a guard test enforces it).
func cmdContext() context.Context {
	return config.WithPolicyContext(context.Background(), activePolicy)
}

// cliLockPolicy is the lock policy of this command's loads, set from its flags
// (generate --locked/--frozen/--no-fetch, lock). The library never reads it: the
// command line hands it to each load (withCLILockPolicy), as it does activePolicy.
var cliLockPolicy config.LockPolicy

// withCLILockPolicy puts the command line's lock policy before opts, so a load
// that passes its own config.WithLockPolicy overrides it.
func withCLILockPolicy(opts []config.LoadOption) []config.LoadOption {
	return append([]config.LoadOption{config.WithLockPolicy(cliLockPolicy)}, opts...)
}

// loadProject, loadProjectFile and loadProjectDir are the project loaders under
// the command line's policy.
func loadProject(ctx context.Context, dir string, opts ...config.LoadOption) (*config.Config, error) {
	return refusalsReported(project.Load(config.WithPolicyContext(ctx, activePolicy), dir, withCLILockPolicy(opts)...))
}

func loadProjectFile(ctx context.Context, path string, opts ...config.LoadOption) (*config.Config, error) {
	cfg, err := refusalsReported(project.LoadFile(config.WithPolicyContext(ctx, activePolicy), path, withCLILockPolicy(opts)...))
	if err != nil && errors.Is(err, fs.ErrNotExist) && strings.Contains(err.Error(), "stat config path") {
		return nil, oops.Hint("-C/--config (or the argument) takes a config file or a config directory such as .ai-rulez; check the path").
			Errorf("config path %s does not exist", path)
	}
	return cfg, err
}

func loadProjectDir(ctx context.Context, dir, configDirName string, opts ...config.LoadOption) (*config.Config, error) {
	return refusalsReported(project.LoadDir(config.WithPolicyContext(ctx, activePolicy), dir, configDirName, withCLILockPolicy(opts)...))
}

// refusalsReported prints the symlinked content of the project the loader refused
// to stderr, which --quiet does not silence: the library records them in
// Config.ContentProblems and prints nothing itself.
func refusalsReported(cfg *config.Config, err error) (*config.Config, error) {
	if err == nil && cfg != nil {
		for _, p := range cfg.ContentProblems {
			fmt.Fprintf(os.Stderr, "WARN  refusing symlinked content %s: %s\n", p.Path, p.Reason)
		}
	}
	return cfg, err //nolint:wrapcheck // already contextual
}

// --policy-trust-tofu uses stdinIsTerminal (approve.go): it refuses to run in a
// pipe or in CI, where nobody reviews the digest.

func init() {
	RootCmd.PersistentFlags().StringVar(&policyFlag, "policy", "",
		"Organization policy: a file, or an https URL pinned with @sha256:<hex> (tighten-only; also AI_RULEZ_POLICY and the managed path). A repository can only add restrictions to it; see docs/policy.md")
	RootCmd.PersistentFlags().StringVar(&policyDigestFlag, "policy-digest", "",
		"The digest (sha256:<hex>) the --policy file or URL must have; a mismatch fails closed (AR741)")
	RootCmd.PersistentFlags().BoolVar(&policyOfflineFlag, "policy-offline", false,
		"Load a URL policy from the user cache only (also AI_RULEZ_POLICY_OFFLINE=1); a cached copy older than --policy-max-stale fails closed")
	RootCmd.PersistentFlags().StringVar(&policyMaxStaleFlag, "policy-max-stale", "",
		"How long a cached copy of a URL policy may stand in for an unreachable URL, for example 7d or 168h; 0 allows none (default 7d, or AI_RULEZ_POLICY_MAX_STALE)")
	RootCmd.PersistentFlags().BoolVar(&discoverOrgFlag, "discover-org", false,
		"Also load the organization policy of the repository's GitHub owner (ai-rulez-policy.toml in <owner>/.github); needs a digest from [policy.digests] in the user config or --policy-trust-tofu. A convenience layer, not an anchor: see docs/policy.md")
	RootCmd.PersistentFlags().BoolVar(&policyRequireSignedFlag, "policy-require-signed", false,
		"Refuse a policy that has no valid signature (<policy>.sigstore.json next to it); needs a trusted signer (also AI_RULEZ_POLICY_REQUIRE_SIGNED=1)")
	RootCmd.PersistentFlags().StringSliceVar(&policySignerKeyFlag, "policy-signer-key", nil,
		"PEM public key trusted to sign the policy (repeatable; also AI_RULEZ_POLICY_SIGNER_KEY and [[policy.signers]] in the user config)")
	RootCmd.PersistentFlags().StringVar(&policySignerIDFlag, "policy-signer-identity", "",
		"Certificate identity trusted to sign the policy, with --policy-signer-issuer (keyless signing)")
	RootCmd.PersistentFlags().StringVar(&policySignerIssuerFlag, "policy-signer-issuer", "",
		"OIDC issuer of --policy-signer-identity")
	RootCmd.PersistentFlags().StringVar(&policyTrustedRootFlag, "policy-trusted-root", "",
		"Sigstore trusted root file for keyless policy signatures (default: the root that ai-rulez trust update cached)")
	RootCmd.PersistentFlags().BoolVar(&policyTOFUFlag, "policy-trust-tofu", false,
		"Accept the digest of an unpinned --policy URL once, in a terminal, and record it in the user cache; pin it afterwards")
	RootCmd.PersistentFlags().StringVar(&policyModeFlag, "policy-mode", "",
		"How a repository that loosens the organization policy is treated: enforce (default, the run fails) or warn (reported as warnings, for rollout; the policy values are still enforced)")
	ValidateCmd.Flags().BoolVar(&validateShowPolicy, "show-policy", false,
		"Print the effective organization policy with the origin of every value and what the repository tried to loosen (text, or JSON with --format json), then exit")
}

// policyGate fails when the policy had to clamp the repository configuration:
// generation and plain validation refuse to continue on a loosening attempt.
func policyGate(cfg *config.Config) error {
	for _, line := range config.PolicyWarnings(cfg) {
		logger.Warn("Organization policy violation (--policy-mode warn): " + line)
	}
	return config.CheckPolicy(cfg) //nolint:wrapcheck // already contextual
}

// policyReportFor describes the effective policy and what it did to cfg: the
// report of `validate --show-policy`, which the policy_show tool returns as well.
// cfg is nil when the configuration did not load; the policy is shown regardless.
func policyReportFor(cfg *config.Config) (policy.Report, error) {
	resolved, err := policyEnforcer.Load()
	if err != nil {
		return policy.Report{}, err //nolint:wrapcheck // already contextual
	}
	var result *policy.Result
	if cfg != nil {
		// The organization policy of the repository's owner, when discovery is on,
		// is a layer of this repository only.
		withOrg, oerr := policyEnforcer.LoadFor(cfg.BaseDir)
		if oerr != nil {
			return policy.Report{}, oerr //nolint:wrapcheck // already contextual
		}
		resolved = withOrg
		result = &policy.Result{Outcome: cfg.PolicyOutcome}
		if cfg.PolicyOutcome != nil {
			result.Accepted = cfg.PolicyOutcome.Accepted
		}
	}
	return policy.BuildReport(resolved, result), nil
}

// runShowPolicy prints the effective policy and returns the exit code.
func runShowPolicy(ctx context.Context, args []string, out io.Writer) int {
	cfg, lerr := loadConfigForCommand(ctx, args, config.WithoutRemote())
	if lerr != nil {
		cfg = nil
	}
	report, err := policyReportFor(cfg)
	if err != nil {
		renderStderr(err)
		return 1
	}
	if lerr != nil && len(report.Layers) > 0 {
		fmt.Fprintf(os.Stderr, "no configuration to compare with the policy: %v\n", lerr)
	}
	if structuredFormat(validateFormat) && validateFormat != formatJSON {
		renderStderr(oops.Errorf("validate --show-policy supports --format text and json, not %q", validateFormat))
		return 1
	}
	if validateFormat == formatJSON {
		if err := report.WriteJSON(out); err != nil {
			renderStderr(err)
			return 1
		}
	} else if err := report.WriteText(out); err != nil {
		renderStderr(oops.Wrapf(err, "write the policy report"))
		return 1
	}
	if report.Overrides.Rejected > 0 && report.Mode != policy.ModeWarn {
		return exitCodeFor(config.ErrPolicyLoosens)
	}
	return 0
}
