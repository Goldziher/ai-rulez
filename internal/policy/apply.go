package policy

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
)

// noHostSentinel stands for "no host is allowed" in lint.security.allowed_hosts,
// where an empty list means the check is off. .invalid never resolves (RFC 2606).
const noHostSentinel = "policy.invalid"

// Resolved is a policy ready to apply: the folded rules, the layers they came
// from, and which layer decided each key.
type Resolved struct {
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
		SeverityFloor: map[string]string{},
		RequiredCodes: append([]string(nil), r.Policy.Lint.RequiredCodes...),
	}}
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
	a.severities()
	a.securityHosts()
	a.scanImports()
	a.lock()
	a.networks()
	a.guard()
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
