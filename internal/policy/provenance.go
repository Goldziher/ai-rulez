package policy

import (
	"strings"
)

// provenance names, for every constrained key of the folded policy, the layer
// that decides it: the first layer holding the winning value for a scalar, and
// every contributing layer (joined with "+") for a set.
func provenance(layers []Layer) map[string]string {
	pv := &provenanceOf{layers: layers, out: map[string]string{}}
	eff := Fold(layers)
	pv.sources(eff)
	pv.lintLimits(eff)
	pv.lintScanners(eff)
	pv.switches(eff)
	pv.signing(eff)
	return pv.out
}

// provenanceOf collects the deciding layers of each key.
type provenanceOf struct {
	layers []Layer
	out    map[string]string
}

// firstWith names the first layer holding the winning value of a scalar key.
func (pv *provenanceOf) firstWith(key string, has func(Policy) bool) {
	for i := range pv.layers {
		if has(pv.layers[i].Policy) {
			pv.out[key] = pv.layers[i].Origin
			return
		}
	}
}

// all names every layer that contributes to a set key, joined with "+".
func (pv *provenanceOf) all(key string, has func(Policy) bool) {
	var names []string
	for i := range pv.layers {
		if has(pv.layers[i].Policy) {
			names = append(names, pv.layers[i].Origin)
		}
	}
	if len(names) > 0 {
		pv.out[key] = strings.Join(names, "+")
	}
}

// sources names the deciding layers of [sources].
func (pv *provenanceOf) sources(eff Policy) {
	pv.all("sources.allowed_hosts", func(p Policy) bool { return p.Sources.Allowed.Set })
	pv.all("sources.deny_hosts", func(p Policy) bool { return len(p.Sources.Deny) > 0 })
	pv.firstWith("sources.require_pinned", func(p Policy) bool { return p.Sources.RequirePinned })
	pv.all("sources.deny_digests", func(p Policy) bool { return len(p.Sources.DenyDigests) > 0 })
	pv.firstWith("sources.min_release_age", func(p Policy) bool {
		return p.Sources.MinReleaseAge > 0 && p.Sources.MinReleaseAge == eff.Sources.MinReleaseAge
	})
	pv.firstWith("sources.min_release_age_source", func(p Policy) bool {
		return p.Sources.MinReleaseAgeSource != "" && p.Sources.MinReleaseAgeSource == eff.Sources.MinReleaseAgeSource
	})
}

// lintLimits names the deciding layers of the [lint] limits and floors.
func (pv *provenanceOf) lintLimits(eff Policy) {
	for kind, sb := range eff.Lint.SizeBudgets {
		if sb.MaxLines > 0 {
			pv.firstWith("lint.budgets."+kind+".max_lines", func(p Policy) bool { return p.Lint.SizeBudgets[kind].MaxLines == sb.MaxLines })
		}
		if sb.MaxTokens > 0 {
			pv.firstWith("lint.budgets."+kind+".max_tokens", func(p Policy) bool { return p.Lint.SizeBudgets[kind].MaxTokens == sb.MaxTokens })
		}
	}
	for code, limit := range eff.Lint.MaxFindings {
		pv.firstWith("lint.max_findings."+code, func(p Policy) bool { v, ok := p.Lint.MaxFindings[code]; return ok && v == limit })
	}
	pv.all("lint.no_inline_ignore", func(p Policy) bool { return len(p.Lint.NoInlineIgnore) > 0 })
	pv.all("lint.required_codes", func(p Policy) bool { return len(p.Lint.RequiredCodes) > 0 })
	for code, sev := range eff.Lint.SeverityFloor {
		pv.firstWith("lint.severity_floor."+code, func(p Policy) bool { return p.Lint.SeverityFloor[code] == sev })
	}
}

// lintScanners names the deciding layers of the [lint] security and scanner keys.
func (pv *provenanceOf) lintScanners(eff Policy) {
	pv.all("lint.security.allowed_hosts", func(p Policy) bool { return p.Lint.Security.AllowedHosts.Set })
	pv.firstWith("lint.security.scan_imports", func(p Policy) bool {
		return p.Lint.Security.ScanImports != "" && p.Lint.Security.ScanImports == eff.Lint.Security.ScanImports
	})
	pv.all("lint.security.directive_tags", func(p Policy) bool { return len(p.Lint.Security.DirectiveTags) > 0 })
	pv.all("lint.security.trusted_orgs", func(p Policy) bool { return p.Lint.Security.TrustedOrgs.Set })
	pv.firstWith("lint.capability.max_network_commands", func(p Policy) bool {
		v := p.Lint.Capability.MaxNetworkCommands
		return v != nil && eff.Lint.Capability.MaxNetworkCommands != nil && *v == *eff.Lint.Capability.MaxNetworkCommands
	})
	for id, limit := range eff.Lint.LoadBudgets {
		pv.firstWith("lint.load_budgets."+id, func(p Policy) bool { v, ok := p.Lint.LoadBudgets[id]; return ok && v == limit })
	}
	pv.firstWith("lint.scanner_policy.preset", func(p Policy) bool {
		return p.Lint.ScannerPolicy.Preset != "" && p.Lint.ScannerPolicy.Preset == eff.Lint.ScannerPolicy.Preset
	})
	pv.all("lint.scanner_policy.required", func(p Policy) bool { return len(p.Lint.ScannerPolicy.Required) > 0 })
	pv.firstWith("lint.scanner_policy.fail_on", func(p Policy) bool {
		return p.Lint.ScannerPolicy.FailOn != "" && p.Lint.ScannerPolicy.FailOn == eff.Lint.ScannerPolicy.FailOn
	})
	pv.firstWith("lint.scanner_policy.isolation", func(p Policy) bool {
		return p.Lint.ScannerPolicy.Isolation != "" && p.Lint.ScannerPolicy.Isolation == eff.Lint.ScannerPolicy.Isolation
	})
	pv.all("lint.scanner_policy.allow_egress", func(p Policy) bool { return p.Lint.ScannerPolicy.AllowEgress.Set })
}

// switches names the deciding layers of [lock], [telemetry], [llm] and [governance].
func (pv *provenanceOf) switches(eff Policy) {
	pv.firstWith("lock.enforce", func(p Policy) bool { return p.Lock.Enforce })
	pv.firstWith("lock.include_outputs", func(p Policy) bool { return p.Lock.IncludeOutputs })
	pv.firstWith("telemetry.allow_network", func(p Policy) bool { return p.Telemetry.Disabled })
	pv.firstWith("llm.allow_network", func(p Policy) bool { return p.LLM.Disabled })
	pv.firstWith("governance.enforce", func(p Policy) bool { return p.Governance.Enforce })
	pv.all("governance.require_approval", func(p Policy) bool { return len(p.Governance.RequireApproval) > 0 })
	pv.firstWith("governance.min_approvers", func(p Policy) bool {
		return p.Governance.MinApprovers > 0 && p.Governance.MinApprovers == eff.Governance.MinApprovers
	})
	pv.all("governance.approvers", func(p Policy) bool { return p.Governance.Approvers.Set })
	pv.firstWith("governance.min_assurance", func(p Policy) bool {
		return p.Governance.MinAssurance != "" && p.Governance.MinAssurance == eff.Governance.MinAssurance
	})
	pv.firstWith("governance.forbid_self_approval", func(p Policy) bool { return p.Governance.ForbidSelfApproval })
	pv.firstWith("governance.approvers_from", func(p Policy) bool { return p.Governance.ApproversFrom != "" })
}

// signing names the deciding layers of [signing], [mcp], [hooks] and [guard].
func (pv *provenanceOf) signing(eff Policy) {
	pv.all("signing.require_verified", func(p Policy) bool { return len(p.Signing.RequireVerified) > 0 })
	pv.firstWith("signing.tlog", func(p Policy) bool { return p.Signing.TLog != "" && p.Signing.TLog == eff.Signing.TLog })
	pv.firstWith("signing.max_age", func(p Policy) bool { return p.Signing.MaxAge > 0 && p.Signing.MaxAge == eff.Signing.MaxAge })
	pv.firstWith("signing.min_hash_version", func(p Policy) bool {
		return p.Signing.MinHashVersion > 0 && p.Signing.MinHashVersion == eff.Signing.MinHashVersion
	})
	pv.all("signing.trust", func(p Policy) bool { return p.Signing.Trust.Set })
	pv.all("signing.thresholds", func(p Policy) bool { return len(p.Signing.Thresholds) > 0 })
	pv.all("mcp.allowed_commands", func(p Policy) bool { return p.MCP.AllowedCommands.Set })
	pv.all("mcp.deny_transports", func(p Policy) bool { return len(p.MCP.DenyTransports) > 0 })
	pv.firstWith("hooks.allow", func(p Policy) bool { return p.Hooks.Forbidden })
	pv.firstWith("guard.generated", func(p Policy) bool { return p.Guard.Generated })
}
