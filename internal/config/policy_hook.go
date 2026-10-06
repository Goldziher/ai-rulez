package config

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/samber/oops"
)

// PolicyViolation is one place where the repository configuration tried to
// loosen an organization policy (docs/policy.md). The policy package fills it;
// the lint package reports it as AR740-AR745.
type PolicyViolation struct {
	// Code is the rule code (AR740 ... AR745).
	Code string `json:"code"`
	// Key is the dotted configuration key, for example "lint.severity.AR008".
	Key string `json:"key"`
	// File is the configuration file that set the key.
	File string `json:"file,omitempty"`
	// Line is the best-known line of the key in File, 1 when unknown.
	Line int `json:"line,omitempty"`
	// Message states the attempt and the bound it broke.
	Message string `json:"message"`
	// Origin names the policy layer that sets the bound ("managed", "flag", "env").
	Origin string `json:"origin,omitempty"`
}

// PolicyOutcome is the result of applying the organization policy to a loaded
// configuration. It is nil when no policy is in force.
type PolicyOutcome struct {
	// Violations lists every attempt to loosen the policy, in a stable order.
	Violations []PolicyViolation
	// SeverityFloor maps a rule code to the lowest severity the policy allows;
	// the lint package raises the effective severity to it.
	SeverityFloor map[string]string
	// RequiredCodes lists the rule codes that may not be turned off or ignored.
	RequiredCodes []string
	// Accepted lists repository overrides the policy allowed, such as an
	// allowlist the repository narrowed.
	Accepted []string
}

// PolicyEnforcer applies an organization policy to a freshly loaded
// configuration. The cmd package installs one; library users that never do get
// no policy and no behavior change.
type PolicyEnforcer interface {
	// Enforce clamps cfg to the policy and reports what it had to clamp. A nil
	// outcome with a nil error means no policy applies. An error means the
	// policy was demanded but could not be used (fail closed).
	Enforce(ctx context.Context, cfg *Config) (*PolicyOutcome, error)
	// Locks reports whether the policy forbids the named network feature
	// ("telemetry", "llm") regardless of user scope. It must fail closed.
	Locks(feature string) bool
}

type enforcerBox struct{ e PolicyEnforcer }

var policyEnforcer atomic.Pointer[enforcerBox]

// SetPolicyEnforcer installs the process-wide policy enforcer (nil removes it).
// A policy is process-wide by nature: it comes from a flag, the environment or
// a managed path, never from the repository being loaded.
func SetPolicyEnforcer(e PolicyEnforcer) {
	if e == nil {
		policyEnforcer.Store(nil)
		return
	}
	policyEnforcer.Store(&enforcerBox{e})
}

// PolicyLocks reports whether the installed policy forbids network use of the
// named feature ("telemetry" or "llm"). It is false without a policy.
func PolicyLocks(feature string) bool {
	if b := policyEnforcer.Load(); b != nil {
		return b.e.Locks(feature)
	}
	return false
}

// applyPolicy runs the installed enforcer on a loaded configuration and
// records the outcome on it.
func applyPolicy(ctx context.Context, cfg *Config) error {
	b := policyEnforcer.Load()
	if b == nil {
		return nil
	}
	out, err := b.e.Enforce(ctx, cfg)
	if err != nil {
		return err
	}
	cfg.PolicyOutcome = out
	return nil
}

// CheckPolicy fails when the installed policy had to clamp cfg: generation,
// validation and the MCP servers refuse a configuration that tried to loosen
// the policy. It is nil without a policy or a violation.
func CheckPolicy(cfg *Config) error {
	if cfg == nil || cfg.PolicyOutcome == nil || len(cfg.PolicyOutcome.Violations) == 0 {
		return nil
	}
	lines := make([]string, 0, len(cfg.PolicyOutcome.Violations))
	for _, v := range cfg.PolicyOutcome.Violations {
		lines = append(lines, fmt.Sprintf("%s %s:%d  %s", v.Code, v.File, max(v.Line, 1), v.Message))
	}
	return oops.With("violations", lines).
		Hint("The organization policy only lets a repository add restrictions; remove the entries above or ask the policy owners to change the policy").
		Errorf("the configuration loosens the organization policy:\n  %s", strings.Join(lines, "\n  "))
}
