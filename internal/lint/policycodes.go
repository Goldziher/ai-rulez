package lint

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// Codes of the organization policy (internal/policy, docs/policy.md). AR74x is
// the policy block of the allocation table in docs/strict-validation.md.
const (
	CodePolicyLoosened        = "AR740"
	CodePolicyDigestMismatch  = "AR741"
	CodePolicyUnavailable     = "AR742"
	CodePolicyInvalid         = "AR743"
	CodePolicyRequiredMissing = "AR744"
	CodeSourceNotAllowed      = "AR745"
	// AR746 to AR749 are in their own block so each lands with its feature.
	CodeCapabilityNotAllowed = "AR748"
)

var policyCodes = []string{
	CodePolicyLoosened, CodePolicyDigestMismatch, CodePolicyUnavailable,
	CodePolicyInvalid, CodePolicyRequiredMissing, CodeSourceNotAllowed,
	CodeCapabilityNotAllowed,
}

func init() {
	registerRules(
		RuleInfo{CodePolicyLoosened, "policy-loosened", SeverityError, "the repository configuration weakens a key the organization policy only lets it tighten; the policy value is enforced and the attempt reported (always an error)"},
		RuleInfo{CodePolicyDigestMismatch, "policy-digest-mismatch", SeverityError, "a policy file does not match the digest it was pinned to (reserved for pinned policies)"},
		RuleInfo{CodePolicyUnavailable, "policy-unavailable", SeverityError, "a policy that was demanded by --policy or AI_RULEZ_POLICY cannot be read; ai-rulez fails closed instead of running without it"},
		RuleInfo{CodePolicyInvalid, "policy-invalid", SeverityError, "a policy file is unusable: not TOML, an unknown key, a bad pattern, or newer than this ai-rulez"},
		RuleInfo{CodePolicyRequiredMissing, "policy-required-missing", SeverityError, "the repository turns off or ignores a rule code the organization policy requires"},
		RuleInfo{CodeSourceNotAllowed, "source-not-allowed", SeverityError, "an include, installed skill or skill source comes from a host the organization policy does not allow, or one it denies"},
		RuleInfo{CodeCapabilityNotAllowed, "capability-not-allowed", SeverityError, "an MCP server or hook group the organization policy forbids (a denied transport, a command outside mcp.allowed_commands, or hooks when hooks.allow is false); it is not loaded"},
	)
	registerRuleDocs(map[string]RuleDoc{
		CodePolicyLoosened: {
			Why:  "A policy is a floor the repository may raise but never lower. Without clamping and reporting, one pull request could delete the control that the same pull request violates; the stricter value is enforced and the attempt is named here so the author learns why.",
			Bad:  "`[lint.severity] AR008 = \"off\"` under a policy floor of `warning`, or an entry in `lint.security.allowed_hosts` that the policy list does not cover",
			Good: "Remove the entry, or ask the policy owners to widen the policy; the repository cannot",
		},
		CodePolicyDigestMismatch: {
			Why:  "A pinned policy file that changed is indistinguishable from a tampered one, so it is refused.",
			Bad:  "A policy whose SHA-256 differs from the pinned `sha256:` value",
			Good: "Review the new policy, then update the pin where it is configured",
		},
		CodePolicyUnavailable: {
			Why:  "A policy named by `--policy` or `AI_RULEZ_POLICY` was demanded. Skipping it when it cannot be read would let breaking the path switch the policy off, so the run fails instead.",
			Bad:  "`AI_RULEZ_POLICY=/etc/missing.toml ai-rulez generate`",
			Good: "Point the variable at a readable policy file, or unset it where no policy is meant to apply",
		},
		CodePolicyInvalid: {
			Why:  "A typo in a policy must not silently loosen it, so unknown keys, bad patterns, unknown rule codes and a `policy_version` this build does not read are errors.",
			Bad:  "`[lint] required_code = [\"AR001\"]` (the key is required_codes)",
			Good: "Fix the key; for a policy newer than the binary, upgrade ai-rulez",
		},
		CodePolicyRequiredMissing: {
			Why:  "A required code is part of the organization's baseline. Turning it off or ignoring it removes the check the baseline relies on; it stays on at its default or floor severity.",
			Bad:  "`[lint] ignore = [\"AR001\"]` when the policy requires AR001",
			Good: "Remove the ignore, and fix or accept the findings through the baseline process instead",
		},
		CodeSourceNotAllowed: {
			Why:  "The policy lists the hosts remote content may come from, so a typosquatted or attacker-controlled include cannot be added by editing the repository. The source is dropped from the run and reported.",
			Bad:  "An include from `github.com/other-org/rules` under `sources.allowed_hosts = [\"github.com/example-org\"]`",
			Good: "Mirror the content into an allowed organization, or ask the policy owners to allow the host",
		},
		CodeCapabilityNotAllowed: {
			Why:  "An MCP server runs a command or reaches a remote endpoint, and a hook runs a command on every tool event, so the organization bounds both. A server or hook outside the bound is dropped from the run and reported, so a pull request cannot add one that the policy bars.",
			Bad:  "`[[mcp_servers]] command = \"bash\"` under `mcp.allowed_commands = [\"npx\", \"uvx\"]`, or any `[[hooks]]` group when the policy sets `hooks.allow = false`",
			Good: "Use an allowed command, or ask the policy owners to widen the policy; the repository cannot",
		},
	})
	for _, code := range policyCodes {
		SetAnalyzer(code, AnalyzerConfig, ScopeBundle)
	}
	registerRunCheck((*runner).checkPolicy, AnalyzerConfig)
}

// ResolveCode returns the rule code a code or name stands for.
func ResolveCode(key string) (string, bool) {
	rule, ok := lookupRule(key)
	return rule.Code, ok
}

// IsPolicyCode reports whether code is one of AR740-AR745.
func IsPolicyCode(code string) bool {
	for _, c := range policyCodes {
		if c == code {
			return true
		}
	}
	return false
}

// applyPolicy enforces the organization policy on the resolved settings: a
// severity never falls below its floor, a required code is never off or
// ignored, and the policy's own findings are always errors.
func (r *runner) applyPolicy() {
	if r.cfg == nil || r.cfg.PolicyOutcome == nil {
		return
	}
	out := r.cfg.PolicyOutcome
	r.protected = ProtectedCodes(out)
	for _, code := range policyCodes {
		r.sev[code] = SeverityError
		delete(r.ignore, code)
	}
	for code, name := range out.SeverityFloor {
		floor, ok := ParseSeverity(name)
		if !ok {
			continue
		}
		if r.sev[code].rank() < floor.rank() {
			r.sev[code] = floor
		}
		delete(r.ignore, code)
	}
	for _, code := range out.RequiredCodes {
		if r.sev[code] == SeverityOff {
			rule, _ := lookupRule(code) //nolint:errcheck // a required code was checked against the registry
			r.sev[code] = rule.Default
			if r.sev[code] == SeverityOff {
				r.sev[code] = SeverityWarning
			}
		}
		delete(r.ignore, code)
	}
}

// checkPolicy reports what the policy clamped.
func (r *runner) checkPolicy() {
	if r.cfg == nil || r.cfg.PolicyOutcome == nil {
		return
	}
	fallback := filepath.Join(r.cfg.ConfigDir, r.cfg.ConfigFile)
	for _, v := range r.cfg.PolicyOutcome.Violations {
		file := v.File
		if file == "" {
			file = fallback
		}
		r.add(v.Code, file, max(v.Line, 1), "%s", v.Message)
	}
}

// ProtectedCodes lists the rule codes a policy shields from suppression: its
// required codes, the codes it floors and its own AR740-AR745. Ignore comments,
// ignore_paths, baselines and [lint.tolerate] do not apply to them. It is nil
// without a policy.
func ProtectedCodes(out *config.PolicyOutcome) map[string]bool {
	if out == nil {
		return nil
	}
	set := map[string]bool{}
	for _, code := range policyCodes {
		set[code] = true
	}
	for code := range out.SeverityFloor {
		set[resolveOrKeep(code)] = true
	}
	for _, code := range out.RequiredCodes {
		set[resolveOrKeep(code)] = true
	}
	return set
}

func resolveOrKeep(key string) string {
	if code, ok := ResolveCode(key); ok {
		return code
	}
	return key
}

// Suppression routes named in the AR740 finding of an attempt.
const (
	routeIgnorePaths = "[lint] ignore_paths"
	routeInline      = "ai-rulez-lint-ignore comment"
	routeBaseline    = "baseline"
	routeTolerate    = "[lint.tolerate]"
)

// suppressed reports whether path or inline ignores hide a finding. A code the
// policy protects is never hidden: the attempt is recorded for one report.
func (r *runner) suppressed(code, abs string, line int) bool {
	if r.pathIgnored(abs) {
		if !r.refuseSuppression(code, routeIgnorePaths) {
			return true
		}
	}
	if r.inlineIgnored(abs, line, code) {
		if !r.refuseSuppression(code, routeInline) {
			return true
		}
	}
	return false
}

// pathSuppresses is suppressed for the path route alone.
func (r *runner) pathSuppresses(code, abs string) bool {
	return r.pathIgnored(abs) && !r.refuseSuppression(code, routeIgnorePaths)
}

// refuseSuppression records an attempted suppression of a protected code and
// reports true; it reports false for an unprotected one.
func (r *runner) refuseSuppression(code, route string) bool {
	if !r.protected[code] {
		return false
	}
	if r.attempts == nil {
		r.attempts = map[string]map[string]bool{}
	}
	if r.attempts[route] == nil {
		r.attempts[route] = map[string]bool{}
	}
	r.attempts[route][code] = true
	return true
}

// reportSuppressionAttempts emits one AR740 per route that tried to hide a
// protected code. It appends directly: AR740 itself is not suppressible.
func (r *runner) reportSuppressionAttempts() {
	routes := make([]string, 0, len(r.attempts))
	for route := range r.attempts {
		routes = append(routes, route)
	}
	sort.Strings(routes)
	file := r.configFilePath()
	for _, route := range routes {
		codes := make([]string, 0, len(r.attempts[route]))
		for code := range r.attempts[route] {
			codes = append(codes, code)
		}
		sort.Strings(codes)
		r.findings = append(r.findings, suppressionAttempt(r.display(file), route, codes, r.display(r.rootAbs())))
	}
}

func suppressionAttempt(file, route string, codes []string, root string) Finding {
	rule, _ := lookupRule(CodePolicyLoosened) //nolint:errcheck // registered
	return Finding{
		Code: CodePolicyLoosened, Name: rule.Name, Severity: SeverityError, File: file, Line: 1, Root: root,
		Message: fmt.Sprintf("%s tries to hide %s; the organization policy protects these codes, so the findings are reported anyway",
			route, strings.Join(codes, ", ")),
	}
}

// refuse adds the one AR740 finding per route that tried to
// accept or tolerate a protected code through a baseline or [lint.tolerate].
func (r *Report) refuse(route string, codes []string) {
	if len(codes) == 0 {
		return
	}
	sort.Strings(codes)
	f := suppressionAttempt(r.ConfigFile, route, codes, r.Root)
	f.meta().Path = r.ConfigFile
	annotateAnalyzer(&f)
	r.Findings = append(r.Findings, f)
}

// RefuseTolerate reports a [lint.tolerate] entry for protected codes, which the
// caller dropped from the budgets.
func (r *Report) RefuseTolerate(codes []string) { r.refuse(routeTolerate, codes) }
