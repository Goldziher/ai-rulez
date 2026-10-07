package lint

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint/scanners"
	"github.com/Goldziher/ai-rulez/v5/internal/sandbox"
)

// Scanner policy values.
const (
	presetOff = "off"
)

// resolvedScanner is one scanner to inspect or run: a [[lint.external]] entry
// with its profile applied, or a member of the [lint.scanner_policy] preset.
type resolvedScanner struct {
	// LintExternal is the effective entry (profile defaults filled in).
	config.LintExternal
	// Layout is the stage layout of the profile ("" keeps repository paths).
	Layout string
	// Presets are the presets whose members include this scanner's profile.
	Presets []string
	// FromPreset is set for a scanner that exists only because of the preset.
	FromPreset bool
	// EgressFlags are the profile's extra deny-listed flags.
	EgressFlags []string
	// DataSent is what the vendor documents receiving (egress profiles).
	DataSent []string
	// RequiresEnv names the variables the scanner needs (egress profiles).
	RequiresEnv []string
	// Problems are profile-level configuration errors (AR9E0).
	Problems []string
}

// scannerPolicy is [lint.scanner_policy] with the preset's defaults applied.
type scannerPolicy struct {
	preset      string
	required    map[string]bool
	failOn      string
	baseline    string
	isolation   sandbox.Mode
	allowEgress []string
	// allowSet is true when allow_egress was written (even empty).
	allowSet bool
}

func policyOf(lc *config.LintConfig) scannerPolicy {
	p := scannerPolicy{isolation: sandbox.ModeAuto, required: map[string]bool{}}
	if lc == nil || lc.ScannerPolicy == nil {
		return p
	}
	sp := lc.ScannerPolicy
	p.preset = strings.ToLower(strings.TrimSpace(sp.Preset))
	if p.preset == presetOff {
		p.preset = ""
	}
	for _, name := range sp.Required {
		p.required[strings.TrimSpace(name)] = true
	}
	p.failOn = strings.ToLower(strings.TrimSpace(sp.FailOn))
	if preset, ok := scanners.LookupPreset(p.preset); ok && p.failOn == "" {
		p.failOn = preset.FailOn
	}
	p.baseline = strings.TrimSpace(sp.Baseline)
	if m, err := sandbox.ParseMode(sp.Isolation); err == nil {
		p.isolation = m
	}
	p.allowEgress, p.allowSet = sp.AllowEgress, sp.AllowEgress != nil
	return p
}

// validateScannerPolicy lists invalid [lint.scanner_policy] values.
func validateScannerPolicy(lc *config.LintConfig) []string {
	if lc == nil || lc.ScannerPolicy == nil {
		return nil
	}
	sp := lc.ScannerPolicy
	var problems []string
	if p := strings.ToLower(strings.TrimSpace(sp.Preset)); p != "" && p != presetOff {
		if _, ok := scanners.LookupPreset(p); !ok {
			problems = append(problems, fmt.Sprintf("lint.scanner_policy.preset %q is not off or one of %s", sp.Preset, strings.Join(scanners.PresetNames(), ", ")))
		}
	}
	switch strings.ToLower(strings.TrimSpace(sp.FailOn)) {
	case "", levelError, levelWarning, "info":
	default:
		problems = append(problems, fmt.Sprintf("lint.scanner_policy.fail_on %q is not error, warning or info", sp.FailOn))
	}
	if _, err := sandbox.ParseMode(sp.Isolation); err != nil {
		problems = append(problems, "lint.scanner_policy."+err.Error())
	}
	for _, name := range append(append([]string(nil), sp.Required...), sp.AllowEgress...) {
		if strings.TrimSpace(name) == "" {
			problems = append(problems, "lint.scanner_policy: a scanner name in required or allow_egress is empty")
		}
	}
	return problems
}

// resolveScanners returns the scanners of lc in run order: the preset's members
// first, then the [[lint.external]] entries in config order. hasPlugin says
// whether [plugin] or [marketplace] is configured (a preset member may need it).
// An entry with the name (or profile) of a preset member replaces that member.
func resolveScanners(lc *config.LintConfig, hasPlugin bool) []resolvedScanner {
	if lc == nil {
		return nil
	}
	var out []resolvedScanner
	taken := map[string]bool{}
	for _, ex := range lc.External {
		if ex.Name != "" {
			taken[ex.Name] = true
		}
		if ex.Profile != "" {
			taken[ex.Profile] = true
		}
	}
	pol := policyOf(lc)
	if preset, ok := scanners.LookupPreset(pol.preset); ok {
		for _, member := range preset.Profiles {
			p, found := scanners.Lookup(member)
			if !found || p.Egress || taken[member] || (p.When == scanners.WhenPlugin && !hasPlugin) {
				continue
			}
			rs := fromProfile(config.LintExternal{Name: p.Name, Profile: p.Name})
			rs.FromPreset = true
			out = append(out, rs)
		}
	}
	for _, ex := range lc.External {
		if strings.TrimSpace(ex.Name) == "" {
			continue
		}
		rs := fromProfile(ex)
		if len(rs.Command) == 0 {
			continue
		}
		out = append(out, rs)
	}
	for i := range out {
		if p := out[i].Profile; p != "" {
			out[i].Presets = scanners.PresetsOf(p)
		}
		if pol.required[out[i].Name] {
			out[i].Required = true
		}
	}
	return out
}

// fromProfile fills ex from its profile: command, format, inputs and egress
// when unset, the severity map merged (the entry wins), the flag deny-list.
// Declaring less egress than the profile does is a problem, not a fill-in.
func fromProfile(ex config.LintExternal) resolvedScanner {
	rs := resolvedScanner{LintExternal: ex}
	if ex.Profile == "" {
		return rs
	}
	p, ok := scanners.Lookup(ex.Profile)
	if !ok {
		rs.Problems = append(rs.Problems, fmt.Sprintf("profile %q is not one of %s", ex.Profile, strings.Join(scanners.Names(), ", ")))
		return rs
	}
	if len(rs.Command) == 0 {
		rs.Command = append([]string(nil), p.Command...)
	}
	if rs.Format == "" {
		rs.Format = p.Format
	}
	if len(rs.Inputs) == 0 {
		rs.Inputs = append([]string(nil), p.Inputs...)
	}
	if rs.MaxSeverity == "" {
		rs.MaxSeverity = p.MaxSeverity
	}
	switch {
	case ex.Egress == nil:
		egress := p.Egress
		rs.Egress = &egress
	case p.Egress && !*ex.Egress:
		rs.Problems = append(rs.Problems, fmt.Sprintf("egress = false contradicts profile %q, which sends content off the machine", p.Name))
	}
	if len(p.SeverityMap) > 0 {
		merged := map[string]string{}
		for k, v := range p.SeverityMap {
			merged[k] = v
		}
		for k, v := range ex.SeverityMap {
			merged[k] = v
		}
		rs.SeverityMap = merged
	}
	rs.Layout, rs.EgressFlags, rs.DataSent, rs.RequiresEnv = p.Layout, p.EgressFlags, p.DataSent, p.RequiresEnv
	if rs.Egress != nil && *rs.Egress {
		// An egress scanner needs its credential; names the profile lists reach it.
		for _, name := range p.RequiresEnv {
			if !slices.Contains(rs.EnvPass, name) {
				rs.EnvPass = append(append([]string(nil), rs.EnvPass...), name)
			}
		}
	}
	return rs
}

// allProblems lists every configuration problem of the entry (AR9E0).
func (s resolvedScanner) allProblems() []string {
	return append(append([]string(nil), s.Problems...), externalProblems(s.LintExternal)...)
}

// egressFlag returns the argument that turns on egress in a scanner declared
// egress = false: a known flag, or one of the profile's.
func (s resolvedScanner) egressFlag() string {
	if s.Egress == nil || *s.Egress {
		return ""
	}
	return egressFlagViolationWith(s.Command, s.EgressFlags)
}
