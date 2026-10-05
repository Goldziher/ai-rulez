package lint

import (
	"fmt"
	"sort"
	"strings"
)

// Lint profiles are presets for severities and the failure threshold. They are
// a starting point: an explicit [lint.severity] entry, [lint] fail_on or
// --fail-on always wins over the preset. Security rules (AR0xx) are never
// weakened by a profile.

// Profile names.
const (
	ProfileDefault    = "default"
	ProfileStrict     = "strict"
	ProfilePermissive = "permissive"
)

// Profile is one preset.
type Profile struct {
	Name string
	// FailOn is the preset's failure threshold ("" keeps error).
	FailOn string
	// Severity maps a code to the severity the preset gives it.
	Severity map[string]Severity
	// Summary describes the preset for docs and the resolved-policy line.
	Summary string
}

var profiles = map[string]Profile{
	ProfileDefault: {Name: ProfileDefault, Summary: "every rule at its registry severity; fail on errors"},
	ProfileStrict: {
		Name: ProfileStrict, FailOn: string(SeverityWarning),
		Summary: "fail on warnings; description style (AR803) and evals (AR962) on; " +
			"anchors, unknown keys, missing paths, description and name quality and size budgets become errors",
		Severity: map[string]Severity{
			CodeDescriptionStyle: SeverityWarning, CodeEvalsMissing: SeverityWarning,
			CodeAnchorUnresolved: SeverityError, CodeFrontmatterKey: SeverityError, CodePathMissing: SeverityError,
			CodeDescriptionMissing: SeverityError, CodeDescriptionLength: SeverityError, CodeSkillNameInvalid: SeverityError,
			CodeSizeLines: SeverityError, CodeSizeTokens: SeverityError,
		},
	},
	ProfilePermissive: {
		Name: ProfilePermissive, FailOn: string(SeverityError),
		Summary: "fail on errors; broken globs, links, references, resources and metadata are warnings; " +
			"anchors, missing paths, duplicates and size budgets are info; security rules unchanged",
		Severity: map[string]Severity{
			CodeGlobNoMatch: SeverityWarning, CodeLinkUnresolved: SeverityWarning, CodeReferenceUnknown: SeverityWarning,
			CodeFrontmatterSkill: SeverityWarning, CodeSkillResourceMissing: SeverityWarning,
			CodeMetadataMissing: SeverityWarning, CodeMetadataInvalid: SeverityWarning, CodeSupersededMissing: SeverityWarning,
			CodeAnchorUnresolved: SeverityInfo, CodePathMissing: SeverityInfo,
			CodeDescriptionDup: SeverityInfo, CodeDescriptionNearDup: SeverityInfo, CodeDuplicateCollapsed: SeverityInfo,
			CodeSizeLines: SeverityInfo, CodeSizeTokens: SeverityInfo,
		},
	},
}

// ProfileNames lists the preset names.
func ProfileNames() []string {
	out := make([]string, 0, len(profiles))
	for n := range profiles {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// LookupProfile resolves a preset name; "" is the default profile.
func LookupProfile(name string) (Profile, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		name = ProfileDefault
	}
	p, ok := profiles[name]
	return p, ok
}

// ProfileFailOn returns the preset's failure threshold, or "".
func ProfileFailOn(name string) string {
	p, _ := LookupProfile(name) //nolint:errcheck // an unknown name is reported by ValidateSettings
	return p.FailOn
}

// DescribeProfile renders the preset's deltas against the default profile, one
// per line; docs and `--help` use it.
func DescribeProfile(name string) []string {
	p, ok := LookupProfile(name)
	if !ok {
		return nil
	}
	var out []string
	if p.FailOn != "" {
		out = append(out, "fail_on = "+p.FailOn)
	}
	codes := make([]string, 0, len(p.Severity))
	for c := range p.Severity {
		codes = append(codes, c)
	}
	sort.Strings(codes)
	for _, c := range codes {
		rule, _ := lookupRule(c) //nolint:errcheck // table codes are registered (tested)
		out = append(out, fmt.Sprintf("%s %s: %s -> %s", c, rule.Name, rule.Default, p.Severity[c]))
	}
	return out
}

func (r *runner) applyProfile() {
	p, ok := LookupProfile(r.lc.Profile)
	if !ok {
		return
	}
	for code, sev := range p.Severity {
		if _, known := r.sev[code]; known {
			r.sev[code] = sev
		}
	}
}
