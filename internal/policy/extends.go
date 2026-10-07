package policy

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

const (
	// OriginExtends is the origin of a layer pulled in by another layer's extends.
	OriginExtends = "extends"
	// maxExtendsDepth bounds a chain of extends: a policy may extend a policy that
	// extends a policy, five hops deep at most.
	maxExtendsDepth = 5
	// maxExtendsPerFile bounds the entries of one extends list.
	maxExtendsPerFile = 8
	// maxLayers bounds every layer one load pulls in, so a chain cannot fan out.
	maxLayers = 32
)

// identity is how a reference is compared for cycles and duplicates: the URL, or
// the absolute path of a file.
func (r Ref) identity() string { return r.Location }

// expand resolves the extends of a loaded layer. It returns the layer followed by
// its ancestors (nearest first) and the effective policy of the whole chain.
//
// Every hop is tighten-only: a policy that extends another must itself tighten
// it, so a team policy cannot loosen the organization's. Merge already makes the
// result at least as strict as the parent; a child that states a weaker value is
// reported as invalid (AR743) because it misstates its own effect.
func (l *loader) expand(layer Layer, ref Ref, stack []string) ([]Layer, Policy, error) {
	if len(layer.extendsRaw) == 0 {
		return []Layer{layer}, layer.Policy, nil
	}
	if len(stack) >= maxExtendsDepth {
		return nil, Policy{}, &ParseError{Path: layer.Path, Msg: fmt.Sprintf("extends is nested deeper than %d policies", maxExtendsDepth)}
	}
	stack = append(slices.Clone(stack), ref.identity())
	out := []Layer{{}}
	var parentEff Policy
	for _, raw := range layer.extendsRaw {
		pref, err := parentRef(layer, ref, raw)
		if err != nil {
			return nil, Policy{}, err
		}
		if slices.Contains(stack, pref.identity()) {
			return nil, Policy{}, &ParseError{Path: layer.Path, Msg: fmt.Sprintf("extends forms a cycle: %s -> %s", strings.Join(displayChain(stack), " -> "), pref.Display())}
		}
		parent, err := l.loadOne(OriginExtends, pref)
		if err != nil {
			return nil, Policy{}, err
		}
		chain, eff, err := l.expand(parent, pref, stack)
		if err != nil {
			return nil, Policy{}, err
		}
		layer.Extends = append(layer.Extends, parent.Path)
		out = append(out, chain...)
		parentEff = Merge(parentEff, eff)
	}
	if bad := Loosens(parentEff, layer.Policy); len(bad) > 0 {
		return nil, Policy{}, &ParseError{Path: layer.Path, Msg: "this policy extends another but loosens it: " + strings.Join(bad, "; ") +
			"; a policy may only add restrictions to what it extends"}
	}
	out[0] = layer
	return out, Merge(parentEff, layer.Policy), nil
}

// Keys of the switches a policy turns on, with how a Policy answers for each.
const (
	switchRequirePinned      = "sources.require_pinned"
	switchLockEnforce        = "lock.enforce"
	switchLockIncludeOutputs = "lock.include_outputs"
	switchTelemetryNetwork   = "telemetry.allow_network"
	switchLLMNetwork         = "llm.allow_network"
	switchGuardGenerated     = "guard.generated"
	switchGovernanceEnforce  = "governance.enforce"
	switchForbidSelfApproval = "governance.forbid_self_approval"
	switchHooksAllow         = "hooks.allow"
)

// switchValues reports, per switch key, whether a policy has the restriction on.
var switchValues = map[string]func(Policy) bool{
	switchRequirePinned:      func(p Policy) bool { return p.Sources.RequirePinned },
	switchLockEnforce:        func(p Policy) bool { return p.Lock.Enforce },
	switchLockIncludeOutputs: func(p Policy) bool { return p.Lock.IncludeOutputs },
	switchTelemetryNetwork:   func(p Policy) bool { return p.Telemetry.Disabled },
	switchLLMNetwork:         func(p Policy) bool { return p.LLM.Disabled },
	switchGuardGenerated:     func(p Policy) bool { return p.Guard.Generated },
	switchGovernanceEnforce:  func(p Policy) bool { return p.Governance.Enforce },
	switchForbidSelfApproval: func(p Policy) bool { return p.Governance.ForbidSelfApproval },
	switchHooksAllow:         func(p Policy) bool { return p.Hooks.Forbidden },
}

func displayChain(stack []string) []string {
	out := make([]string, len(stack))
	for i, s := range stack {
		out[i] = redactURL(s)
		if !strings.Contains(s, "://") {
			out[i] = s
		}
	}
	return out
}

// parentRef resolves one extends entry of a layer. A URL parent must be pinned
// with @sha256:<hex> (no trust-on-first-use for a chain); a file parent is
// resolved against the directory of the policy that names it, and a policy that
// came from a URL cannot reach for a file on the machine.
func parentRef(child Layer, childRef Ref, raw string) (Ref, error) {
	ref, err := ParseRef(raw, "")
	if err != nil {
		return Ref{}, wrapExtendsError(child.Path, err)
	}
	if ref.Remote {
		if ref.Digest == "" {
			return Ref{}, &DigestError{Path: ref.Display()}
		}
		return ref, nil
	}
	if childRef.Remote {
		return Ref{}, &ParseError{Path: child.Path, Msg: fmt.Sprintf("a policy fetched from a URL cannot extend the local file %q", raw)}
	}
	if !filepath.IsAbs(ref.Location) {
		ref.Location = filepath.Join(filepath.Dir(childRef.Location), ref.Location)
	}
	ref.Location = filepath.Clean(ref.Location)
	return ref, nil
}

func wrapExtendsError(path string, err error) error {
	if pe, ok := err.(*ParseError); ok { //nolint:errorlint // our own type, one level
		return &ParseError{Path: path, Msg: "extends: " + pe.Msg}
	}
	return err
}

// Loosens lists the keys where child states a value weaker than parent's. It
// compares only values child states: a key child leaves alone is not a loosening
// (the fold uses the parent's). A switch counts when the child wrote it in the
// weaker direction (`require_pinned = false`, `allow_network = true`) and the
// parent turns it on; a switch the child never wrote is not compared. Merge(parent,
// child) is never weaker than parent on any key, whatever this reports; the
// report is what lets an author learn that the value they wrote has no effect.
func Loosens(parent, child Policy) []string {
	r := &loosenings{}
	for _, key := range child.statedLoose {
		if on := switchValues[key]; on != nil && on(parent) {
			r.add(key, "is written as the weaker value, but the parent turns the restriction on")
		}
	}
	r.lists(parent, child)
	r.sources(parent, child)
	r.lint(parent.Lint, child.Lint)
	r.governance(parent.Governance, child.Governance)
	r.signing(parent.Signing, child.Signing)
	return r.out
}

// loosenings collects what Loosens reports, in the order the keys are checked.
type loosenings struct{ out []string }

func (r *loosenings) add(key, format string, args ...any) {
	r.out = append(r.out, fmt.Sprintf("%s: %s", key, fmt.Sprintf(format, args...)))
}

// listPatterns reports child items no parent pattern covers.
func (r *loosenings) listPatterns(key string, p, c List) {
	if p.Set && c.Set {
		for _, item := range c.Items {
			if !anyCovers(p.Items, item) {
				r.add(key, "%q is not covered by the parent's %s", item, quoteList(p.Items))
			}
		}
	}
}

// listExact reports child items the parent's list does not hold.
func (r *loosenings) listExact(key string, p, c List) {
	if p.Set && c.Set {
		for _, item := range c.Items {
			if !slices.Contains(p.Items, item) {
				r.add(key, "%q is not in the parent's %s", item, quoteList(p.Items))
			}
		}
	}
}

func (r *loosenings) lists(parent, child Policy) {
	r.listPatterns("sources.allowed_hosts", parent.Sources.Allowed, child.Sources.Allowed)
	r.listPatterns("lint.security.allowed_hosts", parent.Lint.Security.AllowedHosts, child.Lint.Security.AllowedHosts)
	r.listExact("lint.security.trusted_orgs", parent.Lint.Security.TrustedOrgs, child.Lint.Security.TrustedOrgs)
	r.listExact("governance.approvers", parent.Governance.Approvers, child.Governance.Approvers)
	r.listExact("mcp.allowed_commands", parent.MCP.AllowedCommands, child.MCP.AllowedCommands)
	r.listExact("signing.trust", parent.Signing.Trust, child.Signing.Trust)
	r.listExact("lint.scanner_policy.allow_egress", parent.Lint.ScannerPolicy.AllowEgress, child.Lint.ScannerPolicy.AllowEgress)
}

func (r *loosenings) sources(parent, child Policy) {
	if c, p := child.Sources.MinReleaseAge, parent.Sources.MinReleaseAge; c > 0 && c < p {
		r.add("sources.min_release_age", "%s is shorter than the parent's %s", formatAge(c), formatAge(p))
	}
	if c, p := child.Sources.MinReleaseAgeSource, parent.Sources.MinReleaseAgeSource; c != "" && ageSourceRank[c] < ageSourceRank[p] {
		r.add("sources.min_release_age_source", "%q is weaker than the parent's %q", c, p)
	}
}

func (r *loosenings) lint(parent, child Lint) {
	for _, code := range sortedKeys(child.SeverityFloor) {
		if p, ok := parent.SeverityFloor[code]; ok && severityRank[child.SeverityFloor[code]] < severityRank[p] {
			r.add("lint.severity_floor."+code, "%q is below the parent's %q", child.SeverityFloor[code], p)
		}
	}
	if c, p := child.Security.ScanImports, parent.Security.ScanImports; c != "" && policyScanRank[c] < policyScanRank[p] {
		r.add("lint.security.scan_imports", "%q is weaker than the parent's %q", c, p)
	}
	if c, p := child.Capability.MaxNetworkCommands, parent.Capability.MaxNetworkCommands; c != nil && p != nil && *c > *p {
		r.add("lint.capability.max_network_commands", "%d is above the parent's %d", *c, *p)
	}
	r.limits("lint.load_budgets.", parent.LoadBudgets, child.LoadBudgets)
	r.limits("lint.max_findings.", parent.MaxFindings, child.MaxFindings)
	r.sizeBudgets(parent, child)
	child.ScannerPolicy.loosens(parent.ScannerPolicy, r.add)
}

// limits reports child maxima above the parent's.
func (r *loosenings) limits(prefix string, p, c map[string]int) {
	for _, key := range sortedIntKeys(c) {
		if pv, ok := p[key]; ok && c[key] > pv {
			r.add(prefix+key, "%d is above the parent's %d", c[key], pv)
		}
	}
}

func (r *loosenings) sizeBudgets(parent, child Lint) {
	for _, kind := range sortedBudgetKinds(child.SizeBudgets) {
		pb, cb := parent.SizeBudgets[kind], child.SizeBudgets[kind]
		if pb.MaxLines > 0 && cb.MaxLines > pb.MaxLines {
			r.add("lint.budgets."+kind+".max_lines", "%d is above the parent's %d", cb.MaxLines, pb.MaxLines)
		}
		if pb.MaxTokens > 0 && cb.MaxTokens > pb.MaxTokens {
			r.add("lint.budgets."+kind+".max_tokens", "%d is above the parent's %d", cb.MaxTokens, pb.MaxTokens)
		}
	}
}

func (r *loosenings) governance(parent, child Governance) {
	if c, p := child.MinApprovers, parent.MinApprovers; c > 0 && c < p {
		r.add("governance.min_approvers", "%d is below the parent's %d", c, p)
	}
	if c, p := child.MinAssurance, parent.MinAssurance; c != "" && lockfile.AssuranceRank(c) < lockfile.AssuranceRank(p) {
		r.add("governance.min_assurance", "%q is weaker than the parent's %q", c, p)
	}
}

func (r *loosenings) signing(parent, child Signing) {
	for _, key := range slices.Sorted(maps.Keys(parent.Thresholds)) {
		if c, ok := child.Thresholds[key]; ok && c < parent.Thresholds[key] {
			r.add("signing.thresholds."+key, "%d is below the parent's %d", c, parent.Thresholds[key])
		}
	}
	if c, p := child.TLog, parent.TLog; c != "" && tlogRank[c] < tlogRank[p] {
		r.add("signing.tlog", "%q is weaker than the parent's %q", c, p)
	}
	if c, p := child.MaxAge, parent.MaxAge; c > 0 && p > 0 && c > p {
		r.add("signing.max_age", "%s accepts older signatures than the parent's %s", formatAge(c), formatAge(p))
	}
	if c, p := child.MinHashVersion, parent.MinHashVersion; c > 0 && c < p {
		r.add("signing.min_hash_version", "%d is below the parent's %d", c, p)
	}
}

// loosens reports the scanner-policy keys child states weaker than parent.
func (s ScannerPolicy) loosens(parent ScannerPolicy, add func(key, format string, args ...any)) {
	if s.Preset != "" && presetRank(s.Preset) < presetRank(parent.Preset) {
		add("lint.scanner_policy.preset", "%q runs less than the parent's %q", s.Preset, parent.Preset)
	}
	if s.FailOn != "" && failOnRank(s.FailOn) < failOnRank(parent.FailOn) {
		add("lint.scanner_policy.fail_on", "%q is looser than the parent's %q", s.FailOn, parent.FailOn)
	}
	if s.Isolation != "" && isolationRank(s.Isolation) < isolationRank(parent.Isolation) {
		add("lint.scanner_policy.isolation", "%q is weaker than the parent's %q", s.Isolation, parent.Isolation)
	}
}

func sortedIntKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// anyCovers reports whether some pattern covers item.
func anyCovers(patterns []string, item string) bool {
	for _, p := range patterns {
		if Covers(p, item) {
			return true
		}
	}
	return false
}
