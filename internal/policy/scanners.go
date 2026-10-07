package policy

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/lint/scanners"
	"github.com/Goldziher/ai-rulez/v5/internal/sandbox"
)

// ScannerPolicy governs [lint.scanner_policy]. Every key can only tighten:
// the preset and isolation are floors, the required list unions, fail_on is
// the most permissive threshold the repository may use, and allow_egress
// bounds which scanners --allow-egress may enable.
type ScannerPolicy struct {
	// Preset is the weakest preset the repository may use ("" for none).
	Preset string
	// Required lists scanners that are always required.
	Required []string
	// FailOn is the highest threshold the repository may use: "error" (the
	// repository may use any), "warning" or "info" (it must be at least as strict).
	FailOn string
	// Isolation is the weakest isolation the repository may use: "auto" or "require".
	Isolation string
	// AllowEgress bounds the repository's allow_egress; an empty set list allows no
	// scanner to send content away.
	AllowEgress List
}

type fileScannerPolicy struct {
	Preset      string    `toml:"preset"`
	Required    []string  `toml:"required"`
	FailOn      string    `toml:"fail_on"`
	Isolation   string    `toml:"isolation"`
	AllowEgress *[]string `toml:"allow_egress"`
}

func (s *ScannerPolicy) fromDoc(d *fileScannerPolicy) error {
	if d == nil {
		return nil
	}
	if p := strings.ToLower(strings.TrimSpace(d.Preset)); p != "" && p != levelOff {
		if _, ok := scanners.LookupPreset(p); !ok {
			return fmt.Errorf("lint.scanner_policy.preset: %q is not one of %s", d.Preset, strings.Join(scanners.PresetNames(), ", "))
		}
		s.Preset = p
	}
	for _, name := range d.Required {
		if name = strings.TrimSpace(name); name == "" {
			return fmt.Errorf("lint.scanner_policy.required: an entry is empty")
		}
		s.Required = append(s.Required, name)
	}
	s.Required = sortedUnique(s.Required)
	switch f := strings.ToLower(strings.TrimSpace(d.FailOn)); f {
	case "", levelError, levelWarning, levelInfo:
		s.FailOn = f
	default:
		return fmt.Errorf("lint.scanner_policy.fail_on: %q is not error, warning or info", d.FailOn)
	}
	mode, err := sandbox.ParseMode(d.Isolation)
	if err != nil {
		return fmt.Errorf("lint.scanner_policy.%w", err)
	}
	if strings.TrimSpace(d.Isolation) != "" && mode != sandbox.ModeNone {
		s.Isolation = string(mode)
	}
	if d.AllowEgress != nil {
		names := make([]string, 0, len(*d.AllowEgress))
		for _, n := range *d.AllowEgress {
			if n = strings.TrimSpace(n); n == "" {
				return fmt.Errorf("lint.scanner_policy.allow_egress: an entry is empty")
			}
			names = append(names, n)
		}
		s.AllowEgress = List{Set: true, Items: sortedUnique(names)}
	}
	return nil
}

// presetRank orders presets by how much they run (strict contains baseline).
func presetRank(name string) int {
	if p, ok := scanners.LookupPreset(name); ok {
		return len(p.Profiles)
	}
	return 0
}

// failOnRank orders thresholds by strictness: info fails on the most.
func failOnRank(v string) int {
	switch v {
	case levelError:
		return 1
	case levelWarning:
		return 2
	case levelInfo:
		return 3
	}
	return 0
}

func isolationRank(v string) int {
	switch v {
	case "auto":
		return 1
	case "require":
		return 2
	}
	return 0
}

func stricterPreset(a, b string) string {
	if presetRank(b) > presetRank(a) {
		return b
	}
	return a
}

func stricterFailOn(a, b string) string {
	if failOnRank(b) > failOnRank(a) {
		return b
	}
	return a
}

func stricterIsolation(a, b string) string {
	if isolationRank(b) > isolationRank(a) {
		return b
	}
	return a
}

func mergeScannerPolicy(a, b ScannerPolicy) ScannerPolicy {
	return ScannerPolicy{
		Preset:      stricterPreset(a.Preset, b.Preset),
		Required:    union(a.Required, b.Required),
		FailOn:      stricterFailOn(a.FailOn, b.FailOn),
		Isolation:   stricterIsolation(a.Isolation, b.Isolation),
		AllowEgress: intersectExact(a.AllowEgress, b.AllowEgress),
	}
}

// addScanner adds the scanner policy keys to the tree.
func (s ScannerPolicy) addTo(table func(path ...string) map[string]any) {
	if s.Preset != "" {
		table("lint", "scanner_policy")["preset"] = s.Preset
	}
	if len(s.Required) > 0 {
		table("lint", "scanner_policy")["required"] = s.Required
	}
	if s.FailOn != "" {
		table("lint", "scanner_policy")["fail_on"] = s.FailOn
	}
	if s.Isolation != "" {
		table("lint", "scanner_policy")["isolation"] = s.Isolation
	}
	if s.AllowEgress.Set {
		table("lint", "scanner_policy")["allow_egress"] = nonNil(s.AllowEgress.Items)
	}
}

// scannerPolicy applies the policy to the repository's [lint.scanner_policy].
func (a *applier) scannerPolicy() {
	pol := a.res.Policy.Lint.ScannerPolicy
	if pol.Preset == "" && len(pol.Required) == 0 && pol.FailOn == "" && pol.Isolation == "" && !pol.AllowEgress.Set {
		return
	}
	if a.cfg.Lint == nil {
		a.cfg.Lint = &config.LintConfig{}
	}
	if a.cfg.Lint.ScannerPolicy == nil {
		a.cfg.Lint.ScannerPolicy = &config.LintScannerPolicy{}
	}
	sp := a.cfg.Lint.ScannerPolicy
	a.scannerPreset(sp, pol)
	if len(pol.Required) > 0 {
		sp.Required = sortedUnique(append(append([]string(nil), pol.Required...), sp.Required...))
	}
	a.scannerFailOn(sp, pol)
	a.scannerIsolation(sp, pol)
	a.scannerAllowEgress(sp, pol)
}

func (a *applier) scannerPreset(sp *config.LintScannerPolicy, pol ScannerPolicy) {
	if pol.Preset == "" {
		return
	}
	have := strings.ToLower(strings.TrimSpace(sp.Preset))
	switch {
	case have == "":
	case presetRank(have) < presetRank(pol.Preset):
		a.violate(lint.CodePolicyLoosened, "lint.scanner_policy.preset", "preset",
			"[lint.scanner_policy] preset = %q runs less than the policy preset %q (origin: %s); %q is enforced", have, pol.Preset, a.origin("lint.scanner_policy.preset"), pol.Preset)
	case presetRank(have) > presetRank(pol.Preset):
		a.accept = append(a.accept, fmt.Sprintf("lint.scanner_policy.preset (stricter: %s)", have))
		return
	default:
		return
	}
	sp.Preset = pol.Preset
}

func (a *applier) scannerFailOn(sp *config.LintScannerPolicy, pol ScannerPolicy) {
	if pol.FailOn == "" {
		return
	}
	have := strings.ToLower(strings.TrimSpace(sp.FailOn))
	switch {
	case have == "":
	case failOnRank(have) < failOnRank(pol.FailOn):
		a.violate(lint.CodePolicyLoosened, "lint.scanner_policy.fail_on", "fail_on",
			"[lint.scanner_policy] fail_on = %q is looser than the policy threshold %q (origin: %s); %q is enforced", have, pol.FailOn, a.origin("lint.scanner_policy.fail_on"), pol.FailOn)
	case failOnRank(have) > failOnRank(pol.FailOn):
		a.accept = append(a.accept, fmt.Sprintf("lint.scanner_policy.fail_on (stricter: %s)", have))
		return
	default:
		return
	}
	sp.FailOn = pol.FailOn
}

func (a *applier) scannerIsolation(sp *config.LintScannerPolicy, pol ScannerPolicy) {
	if pol.Isolation == "" {
		return
	}
	have := strings.ToLower(strings.TrimSpace(sp.Isolation))
	switch {
	case have == "":
	case isolationRank(have) < isolationRank(pol.Isolation):
		a.violate(lint.CodePolicyLoosened, "lint.scanner_policy.isolation", "isolation",
			"[lint.scanner_policy] isolation = %q is weaker than the policy level %q (origin: %s); %q is enforced", have, pol.Isolation, a.origin("lint.scanner_policy.isolation"), pol.Isolation)
	case isolationRank(have) > isolationRank(pol.Isolation):
		a.accept = append(a.accept, fmt.Sprintf("lint.scanner_policy.isolation (stricter: %s)", have))
		return
	default:
		return
	}
	sp.Isolation = pol.Isolation
}

// scannerAllowEgress keeps the repository's allow_egress entries the policy names.
// Unset, the policy list applies; with an empty policy list no scanner may send
// content away. A repository list is the intersection.
func (a *applier) scannerAllowEgress(sp *config.LintScannerPolicy, pol ScannerPolicy) {
	if !pol.AllowEgress.Set {
		return
	}
	if sp.AllowEgress == nil {
		sp.AllowEgress = append([]string{}, pol.AllowEgress.Items...)
		return
	}
	kept := []string{}
	for _, name := range sp.AllowEgress {
		n := strings.TrimSpace(name)
		if slices.Contains(pol.AllowEgress.Items, n) {
			kept = append(kept, n)
			continue
		}
		a.violate(lint.CodePolicyLoosened, "lint.scanner_policy.allow_egress", name,
			"[lint.scanner_policy] allow_egress entry %q is not in the policy list %s (origin: %s); it is dropped", name, quoteList(pol.AllowEgress.Items), a.origin("lint.scanner_policy.allow_egress"))
	}
	if len(kept) > 0 && !sameSet(kept, pol.AllowEgress.Items) {
		a.accept = append(a.accept, fmt.Sprintf("lint.scanner_policy.allow_egress (narrowed to %s)", quoteList(sortedUnique(kept))))
	}
	sp.AllowEgress = sortedUnique(kept)
	if sp.AllowEgress == nil {
		sp.AllowEgress = []string{}
	}
}
