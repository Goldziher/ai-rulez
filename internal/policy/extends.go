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
// (the fold uses the parent's), and a switch left off cannot be told from one
// never written, so booleans are not compared. Merge(parent, child) is never
// weaker than parent on any key, whatever this reports; the report is what lets
// an author learn that the value they wrote has no effect.
func Loosens(parent, child Policy) []string {
	var out []string
	add := func(key, format string, args ...any) {
		out = append(out, fmt.Sprintf("%s: %s", key, fmt.Sprintf(format, args...)))
	}
	listPatterns := func(key string, p, c List) {
		if p.Set && c.Set {
			for _, item := range c.Items {
				if !anyCovers(p.Items, item) {
					add(key, "%q is not covered by the parent's %s", item, quoteList(p.Items))
				}
			}
		}
	}
	listExact := func(key string, p, c List) {
		if p.Set && c.Set {
			for _, item := range c.Items {
				if !slices.Contains(p.Items, item) {
					add(key, "%q is not in the parent's %s", item, quoteList(p.Items))
				}
			}
		}
	}
	listPatterns("sources.allowed_hosts", parent.Sources.Allowed, child.Sources.Allowed)
	listPatterns("lint.security.allowed_hosts", parent.Lint.Security.AllowedHosts, child.Lint.Security.AllowedHosts)
	listExact("lint.security.trusted_orgs", parent.Lint.Security.TrustedOrgs, child.Lint.Security.TrustedOrgs)
	listExact("governance.approvers", parent.Governance.Approvers, child.Governance.Approvers)
	listExact("mcp.allowed_commands", parent.MCP.AllowedCommands, child.MCP.AllowedCommands)
	listExact("signing.trust", parent.Signing.Trust, child.Signing.Trust)
	listExact("lint.scanner_policy.allow_egress", parent.Lint.ScannerPolicy.AllowEgress, child.Lint.ScannerPolicy.AllowEgress)

	if c, p := child.Sources.MinReleaseAge, parent.Sources.MinReleaseAge; c > 0 && c < p {
		add("sources.min_release_age", "%s is shorter than the parent's %s", formatAge(c), formatAge(p))
	}
	for _, code := range sortedKeys(child.Lint.SeverityFloor) {
		if p, ok := parent.Lint.SeverityFloor[code]; ok && severityRank[child.Lint.SeverityFloor[code]] < severityRank[p] {
			add("lint.severity_floor."+code, "%q is below the parent's %q", child.Lint.SeverityFloor[code], p)
		}
	}
	if c, p := child.Lint.Security.ScanImports, parent.Lint.Security.ScanImports; c != "" && policyScanRank[c] < policyScanRank[p] {
		add("lint.security.scan_imports", "%q is weaker than the parent's %q", c, p)
	}
	if c, p := child.Lint.Capability.MaxNetworkCommands, parent.Lint.Capability.MaxNetworkCommands; c != nil && p != nil && *c > *p {
		add("lint.capability.max_network_commands", "%d is above the parent's %d", *c, *p)
	}
	limits := func(prefix string, p, c map[string]int) {
		for _, key := range sortedIntKeys(c) {
			if pv, ok := p[key]; ok && c[key] > pv {
				add(prefix+key, "%d is above the parent's %d", c[key], pv)
			}
		}
	}
	limits("lint.load_budgets.", parent.Lint.LoadBudgets, child.Lint.LoadBudgets)
	limits("lint.max_findings.", parent.Lint.MaxFindings, child.Lint.MaxFindings)
	for _, kind := range sortedBudgetKinds(child.Lint.SizeBudgets) {
		pb, cb := parent.Lint.SizeBudgets[kind], child.Lint.SizeBudgets[kind]
		if pb.MaxLines > 0 && cb.MaxLines > pb.MaxLines {
			add("lint.budgets."+kind+".max_lines", "%d is above the parent's %d", cb.MaxLines, pb.MaxLines)
		}
		if pb.MaxTokens > 0 && cb.MaxTokens > pb.MaxTokens {
			add("lint.budgets."+kind+".max_tokens", "%d is above the parent's %d", cb.MaxTokens, pb.MaxTokens)
		}
	}
	child.Lint.ScannerPolicy.loosens(parent.Lint.ScannerPolicy, add)
	if c, p := child.Governance.MinApprovers, parent.Governance.MinApprovers; c > 0 && c < p {
		add("governance.min_approvers", "%d is below the parent's %d", c, p)
	}
	if c, p := child.Governance.MinAssurance, parent.Governance.MinAssurance; c != "" && lockfile.AssuranceRank(c) < lockfile.AssuranceRank(p) {
		add("governance.min_assurance", "%q is weaker than the parent's %q", c, p)
	}
	for _, key := range slices.Sorted(maps.Keys(parent.Signing.Thresholds)) {
		if c, ok := child.Signing.Thresholds[key]; ok && c < parent.Signing.Thresholds[key] {
			add("signing.thresholds."+key, "%d is below the parent's %d", c, parent.Signing.Thresholds[key])
		}
	}
	if c, p := child.Signing.TLog, parent.Signing.TLog; c != "" && tlogRank[c] < tlogRank[p] {
		add("signing.tlog", "%q is weaker than the parent's %q", c, p)
	}
	if c, p := child.Signing.MaxAge, parent.Signing.MaxAge; c > 0 && p > 0 && c > p {
		add("signing.max_age", "%s accepts older signatures than the parent's %s", formatAge(c), formatAge(p))
	}
	if c, p := child.Signing.MinHashVersion, parent.Signing.MinHashVersion; c > 0 && c < p {
		add("signing.min_hash_version", "%d is below the parent's %d", c, p)
	}
	return out
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
