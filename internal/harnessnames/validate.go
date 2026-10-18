package harnessnames

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	datePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
)

// validName reports whether n can be a recorded name: trimmed, no path separator, no
// control character (display names with inner spaces exist).
func validName(n string) bool {
	if n == "" || n != strings.TrimSpace(n) {
		return false
	}
	return !strings.ContainsFunc(n, func(r rune) bool { return r < ' ' || r == '/' || r == '\\' })
}

func validKind(k string, reserved bool) bool {
	return k == KindCommand || k == KindSkill || k == KindAgent || (reserved && k == KindReserved)
}

// Validate checks an embedded table: the structure, the sources, the groups and their
// names. file is the base name of the file without extension ("" skips the preset
// check); today is the current date (YYYY-MM-DD), which no date may be after. It
// returns every problem found.
func (t *Table) Validate(file, today string) error {
	var errs []error
	fail := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }
	if t.Schema != SchemaVersion {
		fail("schema = %d, want %d", t.Schema, SchemaVersion)
	}
	if t.Preset == "" || (file != "" && t.Preset != file) {
		fail("preset = %q, want %q", t.Preset, file)
	}
	if !datePattern.MatchString(t.VerifiedOn) || t.VerifiedOn > today {
		fail("verified_on = %q is not a past date", t.VerifiedOn)
	}
	if t.HarnessVersion == "" {
		fail("harness_version is empty")
	}
	errs = append(errs, t.validateHeader()...)
	errs = append(errs, t.validateSources(today)...)
	errs = append(errs, t.validateGroups()...)
	if t.Completeness == None && len(t.Group) > 0 {
		fail("completeness = none but the table has groups")
	}
	return errors.Join(errs...)
}

func (t *Table) validateHeader() []error {
	var errs []error
	switch t.Completeness {
	case Complete, Partial, DocsOnly, None:
	default:
		errs = append(errs, fmt.Errorf("completeness = %q", t.Completeness))
	}
	if t.Severity != SeverityError && t.Severity != SeverityWarning {
		errs = append(errs, fmt.Errorf("severity = %q", t.Severity))
	}
	for _, space := range t.InvocationSpaces {
		for _, k := range space {
			if !validKind(k, false) {
				errs = append(errs, fmt.Errorf("invocation_spaces has kind %q", k))
			}
		}
	}
	return errs
}

func (t *Table) validateSources(today string) []error {
	var errs []error
	seen := map[string]bool{}
	for _, s := range t.Source {
		switch {
		case s.ID == "" || seen[s.ID]:
			errs = append(errs, fmt.Errorf("source id %q is empty or repeated", s.ID))
		case !strings.HasPrefix(s.URL, "https://"):
			errs = append(errs, fmt.Errorf("source %s: url %q is not https", s.ID, s.URL))
		case !datePattern.MatchString(s.VerifiedOn) || s.VerifiedOn > today:
			errs = append(errs, fmt.Errorf("source %s: verified_on = %q is not a past date", s.ID, s.VerifiedOn))
		}
		seen[s.ID] = true
	}
	return errs
}

func (t *Table) validateGroups() []error {
	var errs []error
	seen := map[string]bool{}
	for i := range t.Group {
		g := &t.Group[i]
		if err := validateGroup(g, t, fmt.Sprintf("group %d (%s)", i+1, g.Kind), true); err != nil {
			errs = append(errs, err)
			continue
		}
		for _, n := range g.Names {
			key := g.Kind + "\x00" + g.Match + "\x00" + Normalize(n)
			if seen[key] {
				errs = append(errs, fmt.Errorf("group %d: %s %q is listed twice", i+1, g.Kind, n))
			}
			seen[key] = true
		}
	}
	return errs
}

// validateGroup checks one group; strict also demands a source that resolves.
func validateGroup(g *Group, t *Table, label string, strict bool) error {
	var errs []error
	fail := func(format string, args ...any) { errs = append(errs, fmt.Errorf(label+": "+format, args...)) }
	if !validKind(g.Kind, true) {
		fail("kind = %q", g.Kind)
	}
	switch g.UserCollision {
	case UserWins, BuiltinWins, BothListed, Error, NotApplied, Unknown:
	default:
		fail("user_collision = %q", g.UserCollision)
	}
	if g.Match != MatchExact && g.Match != MatchPrefix {
		fail("match = %q", g.Match)
	}
	if g.Kind == KindReserved && g.UserCollision != Error {
		fail("a reserved group must have user_collision = %q", Error)
	}
	if len(g.Names) == 0 {
		fail("no names")
	}
	if _, ok := t.sources[g.Source]; !ok && (strict || g.Source != "") {
		fail("source %q is not declared", g.Source)
	}
	if _, ok := t.sources[g.Evidence]; !ok && g.Evidence != "" {
		fail("evidence %q is not declared", g.Evidence)
	}
	errs = append(errs, validateNames(g, label)...)
	return errors.Join(errs...)
}

func validateNames(g *Group, label string) []error {
	var errs []error
	for _, n := range g.Names {
		if !validName(n) {
			errs = append(errs, fmt.Errorf("%s: name %q is not a valid name", label, n))
		}
	}
	for owner, aliases := range g.Aliases {
		if !contains(g.Names, owner) {
			errs = append(errs, fmt.Errorf("%s: aliases of %q, which is not in names", label, owner))
		}
		for _, a := range aliases {
			if !validName(a) {
				errs = append(errs, fmt.Errorf("%s: alias %q of %q is not a valid name", label, a, owner))
			}
		}
	}
	return errs
}
