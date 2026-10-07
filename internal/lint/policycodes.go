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
	CodePolicySignature      = "AR746"
	CodeDigestDenied         = "AR747"
	CodeCapabilityNotAllowed = "AR748"
	CodePolicyBudgetExceeded = "AR749"
)

var policyCodes = []string{
	CodePolicyLoosened, CodePolicyDigestMismatch, CodePolicyUnavailable,
	CodePolicyInvalid, CodePolicyRequiredMissing, CodeSourceNotAllowed,
	CodePolicySignature, CodeDigestDenied, CodeCapabilityNotAllowed, CodePolicyBudgetExceeded,
}

func registerPolicycodes(s *ruleSet) {
	s.addRules(
		RuleInfo{CodePolicyLoosened, "policy-loosened", SeverityError, "the repository configuration weakens a key the organization policy only lets it tighten; the policy value is enforced and the attempt reported (always an error)"},
		RuleInfo{CodePolicyDigestMismatch, "policy-digest-mismatch", SeverityError, "a policy file or URL does not match the digest it is pinned to, or a policy URL has no digest (a URL policy is never loaded unpinned)"},
		RuleInfo{CodePolicyUnavailable, "policy-unavailable", SeverityError, "a policy that was demanded by --policy or AI_RULEZ_POLICY cannot be read, or its URL cannot be reached and no cached copy younger than max_stale exists; ai-rulez fails closed instead of running without it"},
		RuleInfo{CodePolicyInvalid, "policy-invalid", SeverityError, "a policy file is unusable: not TOML, an unknown key, a bad pattern, or newer than this ai-rulez"},
		RuleInfo{CodePolicyRequiredMissing, "policy-required-missing", SeverityError, "the repository turns off or ignores a rule code the organization policy requires"},
		RuleInfo{CodeSourceNotAllowed, "source-not-allowed", SeverityError, "an include, installed skill or skill source comes from a host the organization policy does not allow, or one it denies"},
		RuleInfo{CodePolicySignature, "policy-signature-invalid", SeverityError, "a policy is unsigned where signatures are required, or its signature does not verify: not a trusted signer, not covering the policy, or older than one already seen (fails closed)"},
		RuleInfo{CodeDigestDenied, "digest-denied", SeverityError, "ai-rulez.lock pins content whose digest is on the organization policy's sources.deny_digests list; a denied include, installed skill or skill source is not loaded (always an error)"},
		RuleInfo{CodePolicyBudgetExceeded, "policy-budget-exceeded", SeverityError, "a rule has more findings than the organization policy's lint.max_findings ceiling allows (0 allows none); always an error"},
		RuleInfo{CodeCapabilityNotAllowed, "capability-not-allowed", SeverityError, "an MCP server or hook group the organization policy forbids (a denied transport, a command outside mcp.allowed_commands, or hooks when hooks.allow is false); it is not loaded"},
	)
	s.addDocs(map[string]RuleDoc{
		CodePolicyLoosened: {
			Why:  "A policy is a floor the repository may raise but never lower. Without clamping and reporting, one pull request could delete the control that the same pull request violates; the stricter value is enforced and the attempt is named here so the author learns why.",
			Bad:  "`[lint.severity] AR008 = \"off\"` under a policy floor of `warning`, or an entry in `lint.security.allowed_hosts` that the policy list does not cover",
			Good: "Remove the entry, or ask the policy owners to widen the policy; the repository cannot",
		},
		CodePolicyDigestMismatch: {
			Why:  "A pinned policy that changed is indistinguishable from a tampered one, so it is refused. A URL is content someone else controls, so a URL policy without a digest is not loaded at all.",
			Bad:  "A policy whose SHA-256 differs from the pinned `sha256:` value, or `--policy https://policy.example.org/base.toml` with no digest",
			Good: "Review the new policy, then update the pin where it is configured (`@sha256:<hex>`, `--policy-digest`, `AI_RULEZ_POLICY_DIGEST`)",
		},
		CodePolicyUnavailable: {
			Why:  "A policy named by `--policy` or `AI_RULEZ_POLICY` was demanded. Skipping it when it cannot be read would let breaking the path, or the network, switch the policy off, so the run fails instead. A cached copy of a pinned URL policy may stand in for at most max_stale (7d by default).",
			Bad:  "`AI_RULEZ_POLICY=/etc/missing.toml ai-rulez generate`, or a policy URL that is down with no cached copy",
			Good: "Point the variable at a readable policy file, restore the URL, or unset it where no policy is meant to apply",
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
		CodePolicySignature: {
			Why:  "A signature lets an organization publish a policy without every machine pinning its digest, but only if the signer is one the machine trusts and the signature covers exactly this policy. A bad or missing signature is refused, never skipped, so stripping the signature cannot switch the policy off.",
			Bad:  "A policy whose `.sigstore.json` was made by an identity the machine does not trust, or by a key that signed a different file, or `--policy-require-signed` with no signature published",
			Good: "Sign the policy with the organization's signer (`cosign sign-blob --bundle policy.toml.sigstore.json policy.toml`), and configure the trusted signer outside the repository",
		},
		CodeDigestDenied: {
			Why:  "An organization that learns a piece of content is malicious or compromised needs to block it everywhere at once. The deny list names the digest, so renaming the include or moving the file does not help; a denied remote is not fetched.",
			Bad:  "An include whose pinned tree digest is on `sources.deny_digests`, or an authored rule whose digest in `ai-rulez.lock` is",
			Good: "Remove the content, or re-pin to a version that is not denied; ask the policy owners if the digest was listed by mistake",
		},
		CodePolicyBudgetExceeded: {
			Why:  "A ceiling lets an organization say how many findings of a rule it will live with, down to none, without depending on the repository's severity settings. The findings keep their own severity; going over the ceiling is the error, and baselines, [lint.ratchet] and ignore comments cannot absorb it.",
			Bad:  "Three AR703 findings under `[lint.max_findings] AR703 = 0`",
			Good: "Fix the findings; the ceiling is lowered over time by the policy owners, not raised by the repository",
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
	s.addRunCheck((*runner).checkPolicy, AnalyzerConfig)
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
	r.noInline = map[string]bool{}
	for _, code := range out.NoInlineIgnore {
		r.noInline[resolveOrKeep(code)] = true
	}
	for _, code := range policyCodes {
		r.sev[code] = r.policySeverity()
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

// policyWarn reports --policy-mode warn.
func (r *runner) policyWarn() bool {
	return r.cfg != nil && r.cfg.PolicyOutcome != nil && r.cfg.PolicyOutcome.Warn
}

// policySeverity is the severity of the policy's own findings: an error, or a
// warning in --policy-mode warn.
func (r *runner) policySeverity() Severity {
	if r.policyWarn() {
		return SeverityWarning
	}
	return SeverityError
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
// ignore_paths, baselines and [lint.ratchet] do not apply to them. It is nil
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
	for code := range out.MaxFindings {
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
	routeRatchet     = "[lint.ratchet]"
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
	if !r.protected[code] && (route != routeInline || !r.noInline[code]) {
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
		r.findings = append(r.findings, suppressionAttempt(r.display(file), route, codes, r.display(r.rootAbs()), r.policySeverity()))
	}
}

func suppressionAttempt(file, route string, codes []string, root string, sev Severity) Finding {
	rule, _ := lookupRule(CodePolicyLoosened) //nolint:errcheck // registered
	return Finding{
		Code: CodePolicyLoosened, Name: rule.Name, Severity: sev, File: file, Line: 1, Root: root,
		Message: fmt.Sprintf("%s tries to hide %s; the organization policy protects these codes, so the findings are reported anyway",
			route, strings.Join(codes, ", ")),
	}
}

// refuse adds the one AR740 finding per route that tried to
// accept or tolerate a protected code through a baseline or [lint.ratchet].
func (r *Report) refuse(route string, codes []string) {
	if len(codes) == 0 {
		return
	}
	sort.Strings(codes)
	sev := SeverityError
	if r.PolicyWarn {
		sev = SeverityWarning
	}
	f := suppressionAttempt(r.ConfigFile, route, codes, r.Root, sev)
	f.meta().Path = r.ConfigFile
	annotateAnalyzer(&f)
	r.Findings = append(r.Findings, f)
}

// RefuseRatchet reports a [lint.ratchet] entry for protected codes, which the
// caller dropped from the budgets.
func (r *Report) RefuseRatchet(codes []string) { r.refuse(routeRatchet, codes) }

// SizeBudgetKinds lists the content kinds [lint.budgets] may bound, in a stable order.
func SizeBudgetKinds() []string {
	kinds := make([]string, 0, len(defaultBudgets))
	for k := range defaultBudgets {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	return kinds
}

// DefaultSizeBudget is the built-in size budget of a content kind; the zero
// value for an unknown kind.
func DefaultSizeBudget(kind string) config.LintBudget { return defaultBudgets[kind] }

// checkMaxFindings reports AR749 for each code with more findings than the
// policy's ceiling. It appends directly: AR749 is not suppressible, and the
// count is of the findings the run produced, accepted or not.
func (r *runner) checkMaxFindings() {
	if r.cfg == nil || r.cfg.PolicyOutcome == nil || len(r.cfg.PolicyOutcome.MaxFindings) == 0 {
		return
	}
	counts := map[string]int{}
	for i := range r.findings {
		counts[r.findings[i].Code]++
	}
	rule, _ := lookupRule(CodePolicyBudgetExceeded) //nolint:errcheck // registered
	file := r.display(r.configFilePath())
	codes := make([]string, 0, len(r.cfg.PolicyOutcome.MaxFindings))
	for code := range r.cfg.PolicyOutcome.MaxFindings {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	for _, code := range codes {
		limit := r.cfg.PolicyOutcome.MaxFindings[code]
		if code == CodePolicyBudgetExceeded || counts[code] <= limit {
			continue
		}
		r.findings = append(r.findings, Finding{
			Code: CodePolicyBudgetExceeded, Name: rule.Name, Severity: r.sev[CodePolicyBudgetExceeded], File: file, Line: 1, Root: r.display(r.rootAbs()),
			Message: fmt.Sprintf("%s has %d finding%s, over the ceiling of %d set by the organization policy (lint.max_findings); fix them, the repository cannot raise the ceiling", code, counts[code], pluralS(counts[code]), limit),
		})
	}
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
