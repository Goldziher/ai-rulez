package lint

import "path/filepath"

// Codes of the organization policy (internal/policy, docs/policy.md). AR74x is
// the policy block of the allocation table in docs/strict-validation.md.
const (
	CodePolicyLoosened        = "AR740"
	CodePolicyDigestMismatch  = "AR741"
	CodePolicyUnavailable     = "AR742"
	CodePolicyInvalid         = "AR743"
	CodePolicyRequiredMissing = "AR744"
	CodeSourceNotAllowed      = "AR745"
)

var policyCodes = []string{
	CodePolicyLoosened, CodePolicyDigestMismatch, CodePolicyUnavailable,
	CodePolicyInvalid, CodePolicyRequiredMissing, CodeSourceNotAllowed,
}

func init() {
	registerRules(
		RuleInfo{CodePolicyLoosened, "policy-loosened", SeverityError, "the repository configuration weakens a key the organization policy only lets it tighten; the policy value is enforced and the attempt reported (always an error)"},
		RuleInfo{CodePolicyDigestMismatch, "policy-digest-mismatch", SeverityError, "a policy file does not match the digest it was pinned to (reserved for pinned policies)"},
		RuleInfo{CodePolicyUnavailable, "policy-unavailable", SeverityError, "a policy that was demanded by --policy or AI_RULEZ_POLICY cannot be read; ai-rulez fails closed instead of running without it"},
		RuleInfo{CodePolicyInvalid, "policy-invalid", SeverityError, "a policy file is unusable: not TOML, an unknown key, a bad pattern, or newer than this ai-rulez"},
		RuleInfo{CodePolicyRequiredMissing, "policy-required-missing", SeverityError, "the repository turns off or ignores a rule code the organization policy requires"},
		RuleInfo{CodeSourceNotAllowed, "source-not-allowed", SeverityError, "an include, installed skill or skill source comes from a host the organization policy does not allow, or one it denies"},
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
