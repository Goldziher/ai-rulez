package policy

import (
	"strings"
)

// provenance names, for every constrained key of the folded policy, the layer
// that decides it: the first layer holding the winning value for a scalar, and
// every contributing layer (joined with "+") for a set.
func provenance(layers []Layer) map[string]string {
	out := map[string]string{}
	eff := Fold(layers)
	name := func(l Layer) string { return l.Origin }
	firstWith := func(key string, has func(Policy) bool) {
		for _, l := range layers {
			if has(l.Policy) {
				out[key] = name(l)
				return
			}
		}
	}
	all := func(key string, has func(Policy) bool) {
		var names []string
		for _, l := range layers {
			if has(l.Policy) {
				names = append(names, name(l))
			}
		}
		if len(names) > 0 {
			out[key] = strings.Join(names, "+")
		}
	}
	all("sources.allowed_hosts", func(p Policy) bool { return p.Sources.Allowed.Set })
	all("sources.deny_hosts", func(p Policy) bool { return len(p.Sources.Deny) > 0 })
	firstWith("sources.require_pinned", func(p Policy) bool { return p.Sources.RequirePinned })
	all("lint.required_codes", func(p Policy) bool { return len(p.Lint.RequiredCodes) > 0 })
	for code, sev := range eff.Lint.SeverityFloor {
		firstWith("lint.severity_floor."+code, func(p Policy) bool { return p.Lint.SeverityFloor[code] == sev })
	}
	all("lint.security.allowed_hosts", func(p Policy) bool { return p.Lint.Security.AllowedHosts.Set })
	firstWith("lint.security.scan_imports", func(p Policy) bool {
		return p.Lint.Security.ScanImports != "" && p.Lint.Security.ScanImports == eff.Lint.Security.ScanImports
	})
	firstWith("lock.enforce", func(p Policy) bool { return p.Lock.Enforce })
	firstWith("lock.include_outputs", func(p Policy) bool { return p.Lock.IncludeOutputs })
	firstWith("telemetry.allow_network", func(p Policy) bool { return p.Telemetry.Disabled })
	firstWith("llm.allow_network", func(p Policy) bool { return p.LLM.Disabled })
	firstWith("governance.enforce", func(p Policy) bool { return p.Governance.Enforce })
	all("governance.require_approval", func(p Policy) bool { return len(p.Governance.RequireApproval) > 0 })
	firstWith("governance.min_approvers", func(p Policy) bool {
		return p.Governance.MinApprovers > 0 && p.Governance.MinApprovers == eff.Governance.MinApprovers
	})
	all("governance.approvers", func(p Policy) bool { return p.Governance.Approvers.Set })
	firstWith("guard.generated", func(p Policy) bool { return p.Guard.Generated })
	return out
}
