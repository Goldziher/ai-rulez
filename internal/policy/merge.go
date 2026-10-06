package policy

import (
	"slices"
	"sort"
)

// Merge combines two policies into the tighter of the two, key by key:
// allowlists intersect, denylists and required sets union, severities and
// scan levels take the stricter value, and switches turn on. It is
// commutative, idempotent and associative, and the zero Policy is its
// identity, so layers fold in any order and a layer can only add restrictions.
func Merge(a, b Policy) Policy {
	return Policy{
		Sources: Sources{
			Allowed:       intersect(a.Sources.Allowed, b.Sources.Allowed),
			Deny:          union(a.Sources.Deny, b.Sources.Deny),
			RequirePinned: a.Sources.RequirePinned || b.Sources.RequirePinned,
			MinReleaseAge: max(a.Sources.MinReleaseAge, b.Sources.MinReleaseAge),
			DenyDigests:   union(a.Sources.DenyDigests, b.Sources.DenyDigests),
		},
		Lint: Lint{
			RequiredCodes: union(a.Lint.RequiredCodes, b.Lint.RequiredCodes),
			SeverityFloor: mergeFloor(a.Lint.SeverityFloor, b.Lint.SeverityFloor),
			Security: Security{
				AllowedHosts:  intersect(a.Lint.Security.AllowedHosts, b.Lint.Security.AllowedHosts),
				ScanImports:   stricterScan(a.Lint.Security.ScanImports, b.Lint.Security.ScanImports),
				DirectiveTags: union(a.Lint.Security.DirectiveTags, b.Lint.Security.DirectiveTags),
				TrustedOrgs:   intersectExact(a.Lint.Security.TrustedOrgs, b.Lint.Security.TrustedOrgs),
			},
			Capability:     Capability{MaxNetworkCommands: lowerLimit(a.Lint.Capability.MaxNetworkCommands, b.Lint.Capability.MaxNetworkCommands)},
			LoadBudgets:    lowerLimits(a.Lint.LoadBudgets, b.Lint.LoadBudgets),
			ScannerPolicy:  mergeScannerPolicy(a.Lint.ScannerPolicy, b.Lint.ScannerPolicy),
			SizeBudgets:    mergeSizeBudgets(a.Lint.SizeBudgets, b.Lint.SizeBudgets),
			MaxFindings:    lowerLimits(a.Lint.MaxFindings, b.Lint.MaxFindings),
			NoInlineIgnore: union(a.Lint.NoInlineIgnore, b.Lint.NoInlineIgnore),
		},
		Lock: Lock{
			Enforce:        a.Lock.Enforce || b.Lock.Enforce,
			IncludeOutputs: a.Lock.IncludeOutputs || b.Lock.IncludeOutputs,
		},
		Telemetry: Network{Disabled: a.Telemetry.Disabled || b.Telemetry.Disabled},
		LLM:       Network{Disabled: a.LLM.Disabled || b.LLM.Disabled},
		Guard:     Guard{Generated: a.Guard.Generated || b.Guard.Generated},
		Governance: Governance{
			Enforce:         a.Governance.Enforce || b.Governance.Enforce,
			RequireApproval: union(a.Governance.RequireApproval, b.Governance.RequireApproval),
			MinApprovers:    max(a.Governance.MinApprovers, b.Governance.MinApprovers),
			Approvers:       intersectExact(a.Governance.Approvers, b.Governance.Approvers),
		},
		MCP:     mergeMCP(a.MCP, b.MCP),
		Signing: mergeSigning(a.Signing, b.Signing),
		Hooks:   Hooks{Forbidden: a.Hooks.Forbidden || b.Hooks.Forbidden},
	}
}

// Fold merges layers; no layers is the empty policy.
func Fold(layers []Layer) Policy {
	var out Policy
	for _, l := range layers {
		out = Merge(out, l.Policy)
	}
	return out
}

func union(a, b []string) []string {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	return sortedUnique(append(append([]string(nil), a...), b...))
}

// intersect is the allowlist meet. An unset list constrains nothing, so the
// other side wins. Otherwise an entry survives when the other list has an
// entry that covers it; entries the algebra cannot compare are dropped, which
// only ever narrows (a false reject is safe, a false accept is not).
func intersect(a, b List) List {
	if !a.Set {
		return b
	}
	if !b.Set {
		return a
	}
	items := append(coveredBy(a.Items, b.Items), coveredBy(b.Items, a.Items)...)
	return List{Set: true, Items: sortedUnique(items)}
}

// intersectExact is the allowlist meet for lists of plain names (no patterns):
// the entries both sides list. An unset list constrains nothing.
func intersectExact(a, b List) List {
	if !a.Set {
		return b
	}
	if !b.Set {
		return a
	}
	var items []string
	for _, x := range a.Items {
		if slices.Contains(b.Items, x) {
			items = append(items, x)
		}
	}
	return List{Set: true, Items: sortedUnique(items)}
}

// coveredBy returns the entries of xs that some entry of by covers.
func coveredBy(xs, by []string) []string {
	var out []string
	for _, x := range xs {
		for _, y := range by {
			if Covers(y, x) {
				out = append(out, x)
				break
			}
		}
	}
	return out
}

func mergeFloor(a, b map[string]string) map[string]string {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	out := make(map[string]string, len(a)+len(b))
	for _, m := range []map[string]string{a, b} {
		for code, sev := range m {
			if cur, ok := out[code]; !ok || severityRank[sev] > severityRank[cur] {
				out[code] = sev
			}
		}
	}
	return out
}

// lowerLimit is the meet of two upper bounds: the lower one; nil is no bound.
func lowerLimit(a, b *int) *int {
	switch {
	case a == nil && b == nil:
		return nil
	case a == nil:
		v := *b
		return &v
	case b == nil || *a <= *b:
		v := *a
		return &v
	}
	v := *b
	return &v
}

// lowerLimits merges per-key upper bounds, keeping the lower value of each key.
func lowerLimits(a, b map[string]int) map[string]int {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	out := make(map[string]int, len(a)+len(b))
	for _, m := range []map[string]int{a, b} {
		for k, v := range m {
			if cur, ok := out[k]; !ok || v < cur {
				out[k] = v
			}
		}
	}
	return out
}

// policyScanRank orders the values a policy holds: "" is no constraint.
var policyScanRank = map[string]int{"": 0, "warn": 1, "error": 2}

func stricterScan(a, b string) string {
	if policyScanRank[b] > policyScanRank[a] {
		return b
	}
	return a
}

// sortedKeys returns the keys of m in order.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
