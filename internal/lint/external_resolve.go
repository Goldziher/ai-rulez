package lint

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint/scanners"
	cmdrun "github.com/Goldziher/ai-rulez/v5/internal/runner"
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
	// OfflineNotices are the profile's "lookup unreachable" results, dropped when egress is denied.
	OfflineNotices []scanners.OfflineNotice
	// Problems are profile-level configuration errors (AR9E0).
	Problems []string
	// timeoutCeiling is the longest timeout the policy allows (0: the default maximum).
	timeoutCeiling time.Duration
}

// scannerPolicy is [lint.scanner_policy] with the preset's defaults applied.
type scannerPolicy struct {
	preset     string
	required   map[string]bool
	failOn     string
	maxMessage int
	// maxTimeout is the ceiling on a scanner's timeout (0: the runner's default).
	maxTimeout  time.Duration
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
	p.maxMessage = sp.MaxMessage
	if d, ok := parseMaxTimeout(sp.MaxTimeout); ok {
		p.maxTimeout = d
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
	if !validScannerFailOn(sp.FailOn) {
		problems = append(problems, fmt.Sprintf("lint.scanner_policy.fail_on %q is not error, warning or info", sp.FailOn))
	}
	if !validMaxMessage(sp.MaxMessage) {
		problems = append(problems, fmt.Sprintf("lint.scanner_policy.max_message %d is not between 1 and %d", sp.MaxMessage, maxScannerMessageLimit))
	}
	if _, ok := parseMaxTimeout(sp.MaxTimeout); !ok && strings.TrimSpace(sp.MaxTimeout) != "" {
		problems = append(problems, fmt.Sprintf("lint.scanner_policy.max_timeout %q is not a duration between 1s and %s", sp.MaxTimeout, cmdrun.HardMaxTimeout))
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

// validMaxMessage reports whether n is unset (0) or a usable max_message.
func validMaxMessage(n int) bool { return n == 0 || (n >= 1 && n <= maxScannerMessageLimit) }

// versionTimeout is the parsed version_timeout (0 when unset or invalid; an
// invalid value is reported through allProblems).
func (s resolvedScanner) versionTimeout() time.Duration {
	d, _ := time.ParseDuration(s.VersionTimeout) //nolint:errcheck // an invalid value is in Problems
	return d
}

// messageLimit is the number of characters of a finding message kept in text
// output: the scanner's max_message, else the policy's, else the default.
func (s resolvedScanner) messageLimit(pol scannerPolicy) int {
	switch {
	case s.MaxMessage > 0:
		return s.MaxMessage
	case pol.maxMessage > 0:
		return pol.maxMessage
	}
	return maxScannerMessage
}

// validScannerFailOn reports whether v is empty or a fail_on threshold.
func validScannerFailOn(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", levelError, levelWarning, "info":
		return true
	}
	return false
}

// failOnOf is the failure threshold of a scanner's findings: its own fail_on,
// else the policy's (the preset implies one), else "" for the run's threshold.
func (s resolvedScanner) failOnOf(pol scannerPolicy) string {
	if own := strings.ToLower(strings.TrimSpace(s.FailOn)); own != "" {
		return own
	}
	return pol.failOn
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
	taken := takenScannerNames(lc.External)
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
	for i := range lc.External {
		ex := lc.External[i]
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
		out[i].timeoutCeiling = pol.maxTimeout
		if p := out[i].Profile; p != "" {
			out[i].Presets = scanners.PresetsOf(p)
		}
		if pol.required[out[i].Name] {
			out[i].Required = true
		}
	}
	return out
}

// takenScannerNames is the set of names and profiles the [[lint.external]]
// entries use; a preset member with one of them is replaced by the entry.
func takenScannerNames(entries []config.LintExternal) map[string]bool {
	taken := map[string]bool{}
	for i := range entries {
		if entries[i].Name != "" {
			taken[entries[i].Name] = true
		}
		if entries[i].Profile != "" {
			taken[entries[i].Profile] = true
		}
	}
	return taken
}

// fillProfileDefaults copies the profile's command, format, inputs, severity cap,
// batch size and version into the fields the entry left unset.
func fillProfileDefaults(rs *resolvedScanner, p scanners.Profile) {
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
	if rs.BatchSize == 0 {
		rs.BatchSize = p.BatchSize
	}
	if rs.Version == "" {
		rs.Version = p.Version
	}
	if len(rs.MessageProperties) == 0 {
		rs.MessageProperties = append([]string(nil), p.MessageProperties...)
	}
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
	fillProfileDefaults(&rs, p)
	switch {
	case ex.Egress == nil:
		egress := p.Egress
		rs.Egress = &egress
	case p.Egress && !*ex.Egress:
		rs.Problems = append(rs.Problems, fmt.Sprintf("egress = false contradicts profile %q, which sends content off the machine", p.Name))
	}
	if len(p.SeverityMap) > 0 {
		rs.SeverityMap = mergeSeverityMaps(p.SeverityMap, ex.SeverityMap)
	}
	if rs.Egress != nil && !*rs.Egress {
		for _, flag := range p.RequiredFlags {
			if !slices.Contains(rs.Command, flag) {
				rs.Problems = append(rs.Problems, fmt.Sprintf("profile %q requires %s in its command: without it the scanner can send content off the machine", p.Name, flag))
			}
		}
	}
	rs.Layout, rs.EgressFlags, rs.DataSent, rs.RequiresEnv = p.Layout, p.EgressFlags, p.DataSent, p.RequiresEnv
	rs.OfflineNotices = p.OfflineNotices
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

// mergeSeverityMaps returns the profile's map overlaid with the entry's.
func mergeSeverityMaps(profile, entry map[string]string) map[string]string {
	merged := map[string]string{}
	for k, v := range profile {
		merged[k] = v
	}
	for k, v := range entry {
		merged[k] = v
	}
	return merged
}

// allProblems lists every configuration problem of the entry (AR9E0).
func (s resolvedScanner) allProblems() []string {
	return append(append([]string(nil), s.Problems...), externalProblems(s.LintExternal, s.timeoutCeiling)...)
}

// egressFlag returns the argument that turns on egress in a scanner declared
// egress = false: a known flag, or one of the profile's.
func (s resolvedScanner) egressFlag() string {
	if s.Egress == nil || *s.Egress {
		return ""
	}
	return egressFlagViolationWith(s.Command, s.EgressFlags)
}

// parseMaxTimeout reads [lint.scanner_policy] max_timeout; ok is false for an
// empty, malformed or out-of-range value (1s to the runner's hard maximum).
func parseMaxTimeout(v string) (time.Duration, bool) {
	d, err := time.ParseDuration(strings.TrimSpace(v))
	if err != nil || d < time.Second || d > cmdrun.HardMaxTimeout {
		return 0, false
	}
	return d, true
}
