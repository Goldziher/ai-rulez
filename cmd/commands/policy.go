package commands

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/policy"
	"github.com/samber/oops"
)

// policyFlag is --policy: a policy file the organization owns (a CI step, a
// managed wrapper). It is a persistent flag, so every command that loads a
// configuration honors it.
var policyFlag string

// policyModeFlag is --policy-mode: "enforce" (default) or "warn".
var policyModeFlag string

// validateShowPolicy is validate --show-policy.
var validateShowPolicy bool

// policyEnforcer discovers the policy lazily, from the flag, AI_RULEZ_POLICY
// and the managed location, never from the repository being loaded.
var policyEnforcer = policy.NewEnforcer(func() policy.DiscoverOptions {
	return policy.DiscoverOptions{Flag: policyFlag, Mode: policyModeFlag}
})

func init() {
	RootCmd.PersistentFlags().StringVar(&policyFlag, "policy", "",
		"Organization policy file (tighten-only; also AI_RULEZ_POLICY and the managed path). A repository can only add restrictions to it; see docs/policy.md")
	RootCmd.PersistentFlags().StringVar(&policyModeFlag, "policy-mode", "",
		"How a repository that loosens the organization policy is treated: enforce (default, the run fails) or warn (reported as warnings, for rollout; the policy values are still enforced)")
	ValidateCmd.Flags().BoolVar(&validateShowPolicy, "show-policy", false,
		"Print the effective organization policy with the origin of every value and what the repository tried to loosen (text, or JSON with --format json), then exit")
	config.SetPolicyEnforcer(policyEnforcer)
}

// policyGate fails when the policy had to clamp the repository configuration:
// generation and plain validation refuse to continue on a loosening attempt.
func policyGate(cfg *config.Config) error {
	for _, line := range config.PolicyWarnings(cfg) {
		logger.Warn("Organization policy violation (--policy-mode warn): " + line)
	}
	return config.CheckPolicy(cfg) //nolint:wrapcheck // already contextual
}

// runShowPolicy prints the effective policy and returns the exit code.
func runShowPolicy(ctx context.Context, args []string, out io.Writer) int {
	resolved, err := policyEnforcer.Load()
	if err != nil {
		fmtError(err)
		return 1
	}
	var result *policy.Result
	if resolved != nil {
		if cfg, lerr := loadConfigForCommand(ctx, args, config.WithoutRemote()); lerr == nil {
			result = &policy.Result{Outcome: cfg.PolicyOutcome}
			if cfg.PolicyOutcome != nil {
				result.Accepted = cfg.PolicyOutcome.Accepted
			}
		} else {
			fmt.Fprintf(os.Stderr, "no configuration to compare with the policy: %v\n", lerr)
		}
	}
	report := policy.BuildReport(resolved, result)
	if structuredFormat(validateFormat) && validateFormat != formatJSON {
		fmtError(oops.Errorf("validate --show-policy supports --format text and json, not %q", validateFormat))
		return 1
	}
	if validateFormat == formatJSON {
		if err := report.WriteJSON(out); err != nil {
			fmtError(err)
			return 1
		}
	} else {
		report.WriteText(out)
	}
	if report.Overrides.Rejected > 0 && report.Mode != policy.ModeWarn {
		return 1
	}
	return 0
}
