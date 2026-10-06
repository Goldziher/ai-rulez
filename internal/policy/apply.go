package policy

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/approval"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// noHostSentinel stands for "no host is allowed" in lint.security.allowed_hosts,
// where an empty list means the check is off. .invalid never resolves (RFC 2606).
const noHostSentinel = "policy.invalid"

// Resolved is a policy ready to apply: the folded rules, the layers they came
// from, and which layer decided each key.
type Resolved struct {
	// Warn is --policy-mode warn: violations are reported as warnings; the policy
	// values are still enforced.
	Warn       bool
	Layers     []Layer
	Policy     Policy
	Provenance map[string]string
}

// Resolve folds layers into the effective policy.
func Resolve(layers []Layer) *Resolved {
	if len(layers) == 0 {
		return nil
	}
	return &Resolved{Layers: layers, Policy: Fold(layers), Provenance: provenance(layers)}
}

// Result is what applying a policy to one configuration did.
type Result struct {
	Outcome *config.PolicyOutcome
	// Accepted lists repository overrides the policy allowed (a narrowing).
	Accepted []string
}

// applier carries the state of one Apply call.
type applier struct {
	res    *Resolved
	cfg    *config.Config
	file   string
	text   []string
	out    *config.PolicyOutcome
	accept []string
}

// Apply clamps cfg to the policy in place and reports every attempt to loosen
// it. The clamped value is what runs; the report is what the author sees. A
// repository that does not touch a policy key is not reported: the policy value
// is simply used.
func (r *Resolved) Apply(cfg *config.Config) Result {
	a := &applier{res: r, cfg: cfg, out: &config.PolicyOutcome{
		Warn:           r.Warn,
		SeverityFloor:  map[string]string{},
		RequiredCodes:  append([]string(nil), r.Policy.Lint.RequiredCodes...),
		NoInlineIgnore: append([]string(nil), r.Policy.Lint.NoInlineIgnore...),
	}}
	if len(r.Policy.Lint.MaxFindings) > 0 {
		a.out.MaxFindings = map[string]int{}
		for code, limit := range r.Policy.Lint.MaxFindings {
			a.out.MaxFindings[code] = limit
		}
	}
	for code, sev := range r.Policy.Lint.SeverityFloor {
		a.out.SeverityFloor[code] = sev
	}
	a.file = filepath.Join(cfg.ConfigDir, cfg.ConfigFile)
	if cfg.ConfigDir != "" && cfg.ConfigFile != "" {
		if data, err := os.ReadFile(a.file); err == nil { //nolint:gosec // the project's own config file
			a.text = strings.Split(string(data), "\n")
		}
	}
	a.sources()
	a.denyDigests()
	a.severities()
	a.securityHosts()
	a.scanImports()
	a.directiveTags()
	a.trustedOrgs()
	a.maxNetworkCommands()
	a.loadBudgets()
	a.sizeBudgets()
	a.scannerPolicy()
	a.lock()
	a.minReleaseAge()
	a.minReleaseAgeSource()
	a.networks()
	a.guard()
	a.governance()
	a.mcpServers()
	a.hooks()
	a.signing()
	sort.SliceStable(a.out.Violations, func(i, j int) bool {
		x, y := a.out.Violations[i], a.out.Violations[j]
		if x.Code != y.Code {
			return x.Code < y.Code
		}
		return x.Key < y.Key
	})
	sort.Strings(a.accept)
	a.out.Accepted = a.accept
	return Result{Outcome: a.out, Accepted: a.accept}
}

func (a *applier) origin(key string) string { return a.res.Provenance[key] }

// violate records an attempt to loosen key. needle locates the line.
func (a *applier) violate(code, key, needle, format string, args ...any) {
	a.out.Violations = append(a.out.Violations, config.PolicyViolation{
		Code: code, Key: key, File: a.file, Line: a.line(needle),
		Message: fmt.Sprintf(format, args...), Origin: a.origin(policyKeyOf(key)),
	})
}

// line finds the first line that mentions needle, 1 when the file or needle is unknown.
func (a *applier) line(needle string) int {
	if needle == "" {
		return 1
	}
	for i, l := range a.text {
		if strings.Contains(l, needle) {
			return i + 1
		}
	}
	return 1
}

// policyKeyOf maps a repository key to the policy key that bounds it.
func policyKeyOf(key string) string {
	switch {
	case strings.HasPrefix(key, "lint.severity.") || strings.HasPrefix(key, "lint.ignore"):
		return "lint.severity_floor." + strings.TrimPrefix(strings.TrimPrefix(key, "lint.severity."), "lint.ignore.")
	case key == "lock.enforce":
		return "lock.enforce"
	case strings.HasSuffix(key, ".min_release_age"):
		return "sources.min_release_age"
	case key == "lock.min_release_age_source":
		return "sources.min_release_age_source"
	}
	return key
}

func (a *applier) sources() {
	p := a.res.Policy.Sources
	if !p.Allowed.Set && len(p.Deny) == 0 {
		return
	}
	var allow, deny []pattern
	for _, s := range p.Allowed.Items {
		if pt, err := parsePattern(s); err == nil {
			allow = append(allow, pt)
		}
	}
	for _, s := range p.Deny {
		if pt, err := parsePattern(s); err == nil {
			deny = append(deny, pt)
		}
	}
	check := func(kind, name, source string) bool {
		host, segs, remote := splitLocation(source)
		if !remote {
			return true
		}
		loc := host
		if len(segs) > 0 {
			loc += "/" + strings.Join(segs, "/")
		}
		for _, d := range deny {
			if d.matches(host, segs) {
				a.violate(lint.CodeSourceNotAllowed, "sources.deny_hosts", source,
					"%s %q: %s is denied by sources.deny_hosts %q (origin: %s); the source is not loaded", kind, name, loc, d, a.origin("sources.deny_hosts"))
				return false
			}
		}
		if p.Allowed.Set {
			ok := false
			for _, al := range allow {
				if al.matches(host, segs) {
					ok = true
					break
				}
			}
			if !ok {
				a.violate(lint.CodeSourceNotAllowed, "sources.allowed_hosts", source,
					"%s %q: %s is not covered by sources.allowed_hosts %s (origin: %s); the source is not loaded", kind, name, loc, quoteList(p.Allowed.Items), a.origin("sources.allowed_hosts"))
				return false
			}
		}
		return true
	}
	cfg := a.cfg
	cfg.Includes = filterSlice(cfg.Includes, func(i config.IncludeConfig) bool { return check("include", i.Name, i.Source) })
	cfg.InstalledSkills = filterSlice(cfg.InstalledSkills, func(s config.InstalledSkillConfig) bool {
		return check("installed skill", s.Name, s.Source)
	})
	cfg.SkillSources = filterSlice(cfg.SkillSources, func(s config.SkillSourceConfig) bool {
		return check("skill source", s.Name, s.URL)
	})
}

func filterSlice[T any](in []T, keep func(T) bool) []T {
	if len(in) == 0 {
		return in
	}
	out := make([]T, 0, len(in))
	for _, v := range in {
		if keep(v) {
			out = append(out, v)
		}
	}
	return out
}

func quoteList(items []string) string {
	q := make([]string, len(items))
	for i, s := range items {
		q[i] = fmt.Sprintf("%q", s)
	}
	return "[" + strings.Join(q, ", ") + "]"
}

func (a *applier) severities() {
	pol := a.res.Policy.Lint
	if len(pol.RequiredCodes) == 0 && len(pol.SeverityFloor) == 0 {
		return
	}
	required := map[string]bool{}
	for _, c := range pol.RequiredCodes {
		required[c] = true
	}
	lc := a.cfg.Lint
	if lc == nil {
		return
	}
	for _, key := range sortedKeys(lc.Severity) {
		code, ok := lint.ResolveCode(key)
		if !ok {
			continue
		}
		val := strings.ToLower(strings.TrimSpace(lc.Severity[key]))
		floor, floored := pol.SeverityFloor[code]
		switch {
		case val == "off" && required[code]:
			a.violate(lint.CodePolicyRequiredMissing, "lint.severity."+key, key,
				"[lint.severity] %s = \"off\" turns off %s, which the policy requires (origin: %s); it stays on", key, code, a.origin("lint.required_codes"))
			delete(lc.Severity, key)
		case floored && severityRank[val] < severityRank[floor]:
			a.violate(lint.CodePolicyLoosened, "lint.severity."+key, key,
				"[lint.severity] %s = %q is below the policy floor %q (origin: %s); %q is enforced", key, val, floor, a.origin("lint.severity_floor."+code), floor)
			lc.Severity[key] = floor
		}
	}
	kept := lc.Ignore[:0:0]
	for _, key := range lc.Ignore {
		code, ok := lint.ResolveCode(key)
		switch {
		case ok && required[code]:
			a.violate(lint.CodePolicyRequiredMissing, "lint.ignore", key,
				"[lint] ignore lists %s, which the policy requires (origin: %s); it is not ignored", key, a.origin("lint.required_codes"))
		case ok && pol.SeverityFloor[code] != "":
			a.violate(lint.CodePolicyLoosened, "lint.ignore", key,
				"[lint] ignore lists %s, which has the policy floor %q (origin: %s); it is not ignored", key, pol.SeverityFloor[code], a.origin("lint.severity_floor."+code))
		default:
			kept = append(kept, key)
		}
	}
	lc.Ignore = kept
}

func (a *applier) securityHosts() {
	pol := a.res.Policy.Lint.Security.AllowedHosts
	if !pol.Set {
		return
	}
	if a.cfg.Lint == nil {
		a.cfg.Lint = &config.LintConfig{}
	}
	if a.cfg.Lint.Security == nil {
		a.cfg.Lint.Security = &config.LintSecurity{}
	}
	sec := a.cfg.Lint.Security
	var kept []string
	for _, h := range sec.AllowedHosts {
		covered := false
		for _, p := range pol.Items {
			if Covers(p, h) {
				covered = true
				break
			}
		}
		if covered {
			kept = append(kept, h)
			continue
		}
		a.violate(lint.CodePolicyLoosened, "lint.security.allowed_hosts", h,
			"[lint.security] allowed_hosts entry %q is not provably within the policy list %s (origin: %s); it is dropped", h, quoteList(pol.Items), a.origin("lint.security.allowed_hosts"))
	}
	if len(kept) > 0 {
		if len(kept) < len(pol.Items) || !sameSet(kept, pol.Items) {
			a.accept = append(a.accept, fmt.Sprintf("lint.security.allowed_hosts (narrowed to %s)", quoteList(sortedUnique(kept))))
		}
		sec.AllowedHosts = kept
		return
	}
	if len(pol.Items) == 0 {
		sec.AllowedHosts = []string{noHostSentinel}
		return
	}
	sec.AllowedHosts = append([]string(nil), pol.Items...)
}

func sameSet(a, b []string) bool {
	x, y := sortedUnique(a), sortedUnique(b)
	if len(x) != len(y) {
		return false
	}
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

func (a *applier) scanImports() {
	want := a.res.Policy.Lint.Security.ScanImports
	if want == "" {
		return
	}
	if a.cfg.Lint == nil {
		a.cfg.Lint = &config.LintConfig{}
	}
	if a.cfg.Lint.Security == nil {
		a.cfg.Lint.Security = &config.LintSecurity{}
	}
	sec := a.cfg.Lint.Security
	have := strings.ToLower(strings.TrimSpace(sec.ScanImports))
	if scanImportsRank[have] >= scanImportsRank[want] {
		return
	}
	if have != "" {
		a.violate(lint.CodePolicyLoosened, "lint.security.scan_imports", "scan_imports",
			"[lint.security] scan_imports = %q is weaker than the policy level %q (origin: %s); %q is enforced", have, want, a.origin("lint.security.scan_imports"), want)
	}
	sec.ScanImports = want
}

// lintSecurity returns the repository's [lint.security], created when absent.
func (a *applier) lintSecurity() *config.LintSecurity {
	if a.cfg.Lint == nil {
		a.cfg.Lint = &config.LintConfig{}
	}
	if a.cfg.Lint.Security == nil {
		a.cfg.Lint.Security = &config.LintSecurity{}
	}
	return a.cfg.Lint.Security
}

// directiveTags adds the policy's tags to the repository's. The repository's own
// list only ever adds, so there is nothing to loosen and nothing to report.
func (a *applier) directiveTags() {
	pol := a.res.Policy.Lint.Security.DirectiveTags
	if len(pol) == 0 {
		return
	}
	sec := a.lintSecurity()
	sec.DirectiveTags = sortedUnique(append(append([]string(nil), pol...), sec.DirectiveTags...))
}

// trustedOrgs keeps the repository's trusted_orgs the policy list names, like
// governance approvers: the intersection, the policy list when none is left,
// and a sentinel when the policy trusts nobody (an empty list means built-in).
func (a *applier) trustedOrgs() {
	pol := a.res.Policy.Lint.Security.TrustedOrgs
	if !pol.Set {
		return
	}
	sec := a.lintSecurity()
	var kept []string
	for _, org := range sec.TrustedOrgs {
		n := strings.ToLower(strings.TrimSpace(org))
		if slices.Contains(pol.Items, n) {
			kept = append(kept, n)
			continue
		}
		a.violate(lint.CodePolicyLoosened, "lint.security.trusted_orgs", org,
			"[lint.security] trusted_orgs entry %q is not in the policy list %s (origin: %s); it is dropped", org, quoteList(pol.Items), a.origin("lint.security.trusted_orgs"))
	}
	switch {
	case len(kept) > 0:
		if !sameSet(kept, pol.Items) {
			a.accept = append(a.accept, fmt.Sprintf("lint.security.trusted_orgs (narrowed to %s)", quoteList(sortedUnique(kept))))
		}
		sec.TrustedOrgs = sortedUnique(kept)
	case len(pol.Items) == 0:
		sec.TrustedOrgs = []string{noHostSentinel}
	default:
		sec.TrustedOrgs = append([]string(nil), pol.Items...)
	}
}

// maxNetworkCommands clamps [lint.capability] max_network_commands to the
// policy's bound; a lower value is a narrowing and is accepted.
func (a *applier) maxNetworkCommands() {
	pol := a.res.Policy.Lint.Capability.MaxNetworkCommands
	if pol == nil {
		return
	}
	if a.cfg.Lint == nil {
		a.cfg.Lint = &config.LintConfig{}
	}
	if a.cfg.Lint.Capability == nil {
		a.cfg.Lint.Capability = &config.LintCapability{}
	}
	c := a.cfg.Lint.Capability
	// An unset repository value runs at the built-in default; a policy bound
	// above it must not raise it.
	limit := *pol
	unsetLimit := min(limit, lint.DefaultMaxNetworkCommands)
	switch have := c.MaxNetworkCommands; {
	case have == nil:
		limit = unsetLimit
	case *have > limit:
		a.violate(lint.CodePolicyLoosened, "lint.capability.max_network_commands", "max_network_commands",
			"[lint.capability] max_network_commands = %d is above the policy limit %d (origin: %s); %d is enforced", *have, limit, a.origin("lint.capability.max_network_commands"), limit)
	case *have < limit:
		a.accept = append(a.accept, fmt.Sprintf("lint.capability.max_network_commands (lowered to %d)", *have))
		return
	default:
		return
	}
	c.MaxNetworkCommands = &limit
}

// loadBudgets clamps [lint.load_budgets] to the policy's per-limit bounds.
func (a *applier) loadBudgets() {
	pol := a.res.Policy.Lint.LoadBudgets
	if len(pol) == 0 {
		return
	}
	if a.cfg.Lint == nil {
		a.cfg.Lint = &config.LintConfig{}
	}
	lc := a.cfg.Lint
	ids := make([]string, 0, len(pol))
	for id := range pol {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		limit := pol[id]
		key := "lint.load_budgets." + id
		have, set := lc.LoadBudgets[id]
		switch {
		case set && have > limit:
			a.violate(lint.CodePolicyLoosened, key, id,
				"[lint.load_budgets] %s = %d is above the policy limit %d (origin: %s); %d is enforced", id, have, limit, a.origin(key), limit)
		case set && have > 0 && have < limit:
			a.accept = append(a.accept, fmt.Sprintf("%s (lowered to %d)", key, have))
			continue
		case set && have == limit:
			continue
		}
		if lc.LoadBudgets == nil {
			lc.LoadBudgets = map[string]int{}
		}
		if !set || have <= 0 {
			limit = min(limit, lint.LoadBudgetDefault(id)) // unset runs at the built-in; a bound above it must not raise it
		}
		lc.LoadBudgets[id] = limit
	}
}

func (a *applier) lock() {
	pol := a.res.Policy
	enforce := pol.Lock.Enforce || pol.Sources.RequirePinned
	if enforce {
		key := "lock.enforce"
		why := "lock.enforce"
		if !pol.Lock.Enforce {
			why = "sources.require_pinned"
		}
		if a.cfg.Lock != nil && a.cfg.Lock.Enforce != nil && !*a.cfg.Lock.Enforce {
			a.violate(lint.CodePolicyLoosened, key, "enforce",
				"[lock] enforce = false switches the lock off, which the policy forbids (%s, origin: %s); the lock is enforced", why, a.origin(why))
		}
		if a.cfg.Lock == nil {
			a.cfg.Lock = &config.LockConfig{}
		}
		on := true
		a.cfg.Lock.Enforce = &on
	}
	if pol.Lock.IncludeOutputs {
		if a.cfg.Lock != nil && a.cfg.Lock.IncludeOutputs != nil && !*a.cfg.Lock.IncludeOutputs {
			a.violate(lint.CodePolicyLoosened, "lock.include_outputs", "include_outputs",
				"[lock] include_outputs = false drops output pins, which the policy forbids (origin: %s); outputs are pinned", a.origin("lock.include_outputs"))
		}
		if a.cfg.Lock == nil {
			a.cfg.Lock = &config.LockConfig{}
		}
		on := true
		a.cfg.Lock.IncludeOutputs = &on
	}
}

func (a *applier) networks() {
	if a.res.Policy.Telemetry.Disabled && a.cfg.Telemetry != nil && a.cfg.Telemetry.AllowNetwork {
		a.violate(lint.CodePolicyLoosened, "telemetry.allow_network", "allow_network",
			"[telemetry] allow_network = true enables export, which the policy forbids (origin: %s); it stays off", a.origin("telemetry.allow_network"))
		a.cfg.Telemetry.AllowNetwork = false
	}
	if a.res.Policy.LLM.Disabled && a.cfg.LLM != nil && a.cfg.LLM.AllowNetwork {
		a.violate(lint.CodePolicyLoosened, "llm.allow_network", "allow_network",
			"[llm] allow_network = true enables model calls, which the policy forbids (origin: %s); it stays off", a.origin("llm.allow_network"))
		a.cfg.LLM.AllowNetwork = false
	}
}

func (a *applier) guard() {
	if !a.res.Policy.Guard.Generated {
		return
	}
	if a.cfg.Guard == nil {
		a.cfg.Guard = &config.GuardConfig{}
	}
	if !a.cfg.Guard.Generated && a.guardTablePresent() {
		a.violate(lint.CodePolicyLoosened, "guard.generated", "[guard]",
			"[guard] does not enable generated, which the policy requires (origin: %s); the guard hook is on", a.origin("guard.generated"))
	}
	a.cfg.Guard.Generated = true
}

// guardTablePresent reports whether the repository wrote a [guard] table: the
// config type cannot tell an explicit false from an unset bool.
func (a *applier) guardTablePresent() bool {
	for _, l := range a.text {
		if strings.HasPrefix(strings.TrimSpace(l), "[guard]") {
			return true
		}
	}
	return false
}

// governance clamps [governance] to the policy floor: enforce on, the policy's
// selectors always required (PolicyFloor, which exempt cannot narrow), at least
// min_approvers, and only the policy's approvers counting. A repository that
// writes a [governance] table loosening one of them is reported; one without
// the table is forced silently.
func (a *applier) governance() {
	pol := a.res.Policy.Governance
	if !pol.Enforce && len(pol.RequireApproval) == 0 && pol.MinApprovers == 0 && !pol.Approvers.Set &&
		pol.MinAssurance == "" && !pol.ForbidSelfApproval && pol.ApproversFrom == "" {
		return
	}
	repoHas := a.cfg.Governance != nil
	if !repoHas {
		a.cfg.Governance = &config.GovernanceConfig{}
	}
	g := a.cfg.Governance
	if pol.Enforce {
		if repoHas && !g.Enforce {
			a.violate(lint.CodePolicyLoosened, "governance.enforce", "[governance]",
				"[governance] does not enable enforce, which the policy requires (origin: %s); approvals are enforced", a.origin("governance.enforce"))
		}
		g.Enforce = true
	}
	g.PolicyFloor = append([]string(nil), pol.RequireApproval...)
	if pol.MinApprovers > g.MinApprovers {
		if g.MinApprovers > 0 {
			a.violate(lint.CodePolicyLoosened, "governance.min_approvers", "min_approvers",
				"[governance] min_approvers = %d is below the policy minimum %d (origin: %s); %d is enforced", g.MinApprovers, pol.MinApprovers, a.origin("governance.min_approvers"), pol.MinApprovers)
		}
		g.MinApprovers = pol.MinApprovers
	}
	if pol.Approvers.Set {
		a.governanceApprovers(g, pol.Approvers)
	}
	a.governanceAssurance(g, pol, repoHas)
}

// governanceAssurance applies min_assurance, forbid_self_approval and
// approvers_from. The first two only tighten; approvers_from only has to be
// set, because the repository picks which CODEOWNERS file (a policy cannot name
// a path in a repository it has not seen).
func (a *applier) governanceAssurance(g *config.GovernanceConfig, pol Governance, repoHas bool) {
	if pol.MinAssurance != "" && lockfile.AssuranceRank(g.MinAssurance) < lockfile.AssuranceRank(pol.MinAssurance) {
		if g.MinAssurance != "" {
			a.violate(lint.CodePolicyLoosened, "governance.min_assurance", "min_assurance",
				"[governance] min_assurance = %q is weaker than the policy level %q (origin: %s); %q is enforced", g.MinAssurance, pol.MinAssurance, a.origin("governance.min_assurance"), pol.MinAssurance)
		}
		g.MinAssurance = pol.MinAssurance
	}
	if pol.ForbidSelfApproval && !g.ForbidSelfApproval {
		if repoHas {
			a.violate(lint.CodePolicyLoosened, "governance.forbid_self_approval", "[governance]",
				"[governance] does not set forbid_self_approval, which the policy requires (origin: %s); it is enforced", a.origin("governance.forbid_self_approval"))
		}
		g.ForbidSelfApproval = true
	}
	if pol.ApproversFrom != "" && g.ApproversFrom == "" {
		if repoHas {
			a.violate(lint.CodePolicyLoosened, "governance.approvers_from", "[governance]",
				"[governance] has no approvers_from, which the policy requires (origin: %s); %q is enforced", a.origin("governance.approvers_from"), pol.ApproversFrom)
		}
		g.ApproversFrom = pol.ApproversFrom
	}
}

// governanceApprovers keeps the repository's approvers that the policy list
// also names; a repository without any left (or none) gets the policy list. An
// empty policy list means nobody may approve, which approval.NobodyMayApprove stands for.
func (a *applier) governanceApprovers(g *config.GovernanceConfig, pol List) {
	var kept []string
	for _, r := range g.Approvers {
		n := approval.NormalizeReviewer(r)
		if slices.Contains(pol.Items, n) {
			kept = append(kept, n)
			continue
		}
		a.violate(lint.CodePolicyLoosened, "governance.approvers", r,
			"[governance] approvers entry %q is not in the policy list %s (origin: %s); it is dropped", r, quoteList(pol.Items), a.origin("governance.approvers"))
	}
	switch {
	case len(kept) > 0:
		if !sameSet(kept, pol.Items) {
			a.accept = append(a.accept, fmt.Sprintf("governance.approvers (narrowed to %s)", quoteList(sortedUnique(kept))))
		}
		g.Approvers = sortedUnique(kept)
	case len(pol.Items) == 0:
		g.Approvers = []string{approval.NobodyMayApprove}
	default:
		g.Approvers = append([]string(nil), pol.Items...)
	}
}
