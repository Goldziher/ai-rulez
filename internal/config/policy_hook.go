package config

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

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
	// Subject names the pinned content a violation is about, when it is about
	// one (sources.deny_digests, AR747); nil otherwise. Consumers match on it,
	// never on Message.
	Subject *PolicySubject `json:"subject,omitempty"`
}

// PolicySubject is one pinned entry of the lock: its lock kind ("include",
// "skill", "source", "served", or an authored item kind), its name or id, the
// domain or serve view that qualifies it, and the digest the lock pins.
type PolicySubject struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Domain string `json:"domain,omitempty"`
	Digest string `json:"digest"`
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
	// MaxFindings maps a rule code to the most findings of it the policy
	// allows; the lint package reports AR749 for a code over its ceiling.
	MaxFindings map[string]int
	// NoInlineIgnore lists the rule codes whose inline `ai-rulez-lint-ignore`
	// comments are not honored.
	NoInlineIgnore []string
	// Accepted lists repository overrides the policy allowed, such as an
	// allowlist the repository narrowed.
	Accepted []string
	// Warn is --policy-mode warn: the values are still clamped, but the
	// violations are reported as warnings and do not fail the run.
	Warn bool
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

// ContentEnforcer is a PolicyEnforcer that also bounds the content includes and
// installed skills deliver. Enforce runs before anything is fetched, so what that
// content declares (hooks and MCP servers in its frontmatter) is judged once it
// is loaded. The violations it returns are added to the outcome.
type ContentEnforcer interface {
	EnforceContent(ctx context.Context, cfg *Config) []PolicyViolation
}

// A policy comes from a flag, the environment or a managed path, never from the
// repository being loaded, and it belongs to the load it is given to: WithPolicy,
// or WithPolicyContext for a call chain that passes a context but no options. A
// load given neither has no policy, so an embedding service is never bound by the
// policy of another caller in its process.

type policyCtxKey struct{}

// WithPolicyContext returns ctx carrying e, the policy loads made with ctx use
// unless WithPolicy names another. A nil e leaves ctx unchanged.
func WithPolicyContext(ctx context.Context, e PolicyEnforcer) context.Context {
	if e == nil {
		return ctx
	}
	return context.WithValue(ctx, policyCtxKey{}, e)
}

func policyFromContext(ctx context.Context) PolicyEnforcer {
	if ctx != nil {
		if e, ok := ctx.Value(policyCtxKey{}).(PolicyEnforcer); ok {
			return e
		}
	}
	return nil
}

// WithPolicy loads the configuration under the policy e: it clamps the loaded
// configuration, and the loaded Config keeps it for the questions asked later
// (PolicyLocksIn, nested loads). A nil e is no policy.
func WithPolicy(e PolicyEnforcer) LoadOption {
	return func(o *loadOptions) { o.policy = e }
}

// Policy is the enforcer the configuration was loaded under (nil: none).
func (c *Config) Policy() PolicyEnforcer {
	if c == nil {
		return nil
	}
	return c.enforcer
}

// PolicyLocks reports whether the policy forbids network use of the named
// feature ("telemetry" or "llm"). It is false without a policy. It knows no
// repository, so the organization policy of --discover-org is not part of it:
// use LocksIn where a project directory is known.
func PolicyLocks(e PolicyEnforcer, feature string) bool {
	return e != nil && e.Locks(feature)
}

// DirLocker is a PolicyEnforcer that can answer Locks for one project, whose
// organization policy it discovers from the repository's owner.
type DirLocker interface {
	LocksIn(feature, dir string) bool
}

// PolicyLocksIn is PolicyLocks for the project at dir: when organization
// discovery is on, the owner's policy can forbid telemetry export or model calls
// too. An enforcer that cannot discover answers as PolicyLocks does.
func PolicyLocksIn(e PolicyEnforcer, feature, dir string) bool {
	if e == nil {
		return false
	}
	if dl, ok := e.(DirLocker); ok {
		return dl.LocksIn(feature, dir)
	}
	return e.Locks(feature)
}

// PolicyProjectDir is the directory the organization policy is discovered from:
// PolicyDir, else BaseDir.
func (c *Config) PolicyProjectDir() string {
	if c == nil {
		return ""
	}
	if c.PolicyDir != "" {
		return c.PolicyDir
	}
	return c.BaseDir
}

// PolicyLocks reports whether the policy this configuration was loaded under
// forbids network use of the feature, for the project this configuration is.
func (c *Config) PolicyLocks(feature string) bool {
	return PolicyLocksIn(c.Policy(), feature, c.PolicyProjectDir())
}

// applyPolicy runs the installed enforcer on a loaded configuration and
// records the outcome on it.
func applyPolicy(ctx context.Context, cfg *Config) error {
	if cfg.enforcer == nil {
		return nil
	}
	out, err := cfg.enforcer.Enforce(ctx, cfg)
	if err != nil {
		return err
	}
	cfg.PolicyOutcome = out
	return nil
}

// applyContentPolicy bounds the imported content of a loaded configuration. It
// does nothing without a policy in force (a nil outcome).
func applyContentPolicy(ctx context.Context, cfg *Config) {
	if cfg.enforcer == nil || cfg.PolicyOutcome == nil {
		return
	}
	ce, ok := cfg.enforcer.(ContentEnforcer)
	if !ok {
		return
	}
	found := ce.EnforceContent(ctx, cfg)
	if len(found) == 0 {
		return
	}
	v := slices.Concat(cfg.PolicyOutcome.Violations, found)
	sort.SliceStable(v, func(i, j int) bool {
		if v[i].Code != v[j].Code {
			return v[i].Code < v[j].Code
		}
		return v[i].Key < v[j].Key
	})
	cfg.PolicyOutcome.Violations = v
}

func policyLines(out *PolicyOutcome) []string {
	lines := make([]string, 0, len(out.Violations))
	for _, v := range out.Violations {
		lines = append(lines, fmt.Sprintf("%s %s:%d  %s", v.Code, v.File, max(v.Line, 1), v.Message))
	}
	return lines
}

// PolicyWarnings lists the violations a --policy-mode warn run reports without
// failing; it is nil in the default mode, where CheckPolicy fails instead.
func PolicyWarnings(cfg *Config) []string {
	if cfg == nil || cfg.PolicyOutcome == nil || !cfg.PolicyOutcome.Warn {
		return nil
	}
	return policyLines(cfg.PolicyOutcome)
}

// CheckPolicy fails when the installed policy had to clamp cfg: generation,
// validation and the MCP servers refuse a configuration that tried to loosen
// the policy. It is nil without a policy or a violation.
//
// In --policy-mode warn it is nil: the violations are PolicyWarnings instead.
func CheckPolicy(cfg *Config) error {
	if cfg == nil || cfg.PolicyOutcome == nil || cfg.PolicyOutcome.Warn || len(cfg.PolicyOutcome.Violations) == 0 {
		return nil
	}
	lines := policyLines(cfg.PolicyOutcome)
	return oops.With("violations", lines).
		Hint("The organization policy only lets a repository add restrictions; remove the entries above or ask the policy owners to change the policy").
		Errorf("the configuration loosens the organization policy:\n  %s", strings.Join(lines, "\n  "))
}
