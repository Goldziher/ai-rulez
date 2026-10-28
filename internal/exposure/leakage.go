package exposure

import (
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/Goldziher/ai-rulez/v5/internal/areas"
)

// SchemaLeakage is the schema_version of a LeakageReport
// (schema/exposure-leakage.v1.schema.json).
const SchemaLeakage = 1

// excerptWidth is the most characters of a line a Leak quotes.
const excerptWidth = 140

// LeakSeverity says what a Leak does to the gate.
type LeakSeverity string

// The leak severities.
const (
	// LeakViolation is a term of an area named at session start; it fails the gate.
	LeakViolation LeakSeverity = "violation"
	// LeakWarning is a listing that names only ambiguous words; it is advisory.
	LeakWarning LeakSeverity = "warning"
	// LeakAccepted is a violation or warning a [[lint.accept]] entry accepts, with its reason.
	LeakAccepted LeakSeverity = "accepted"
)

// Leak is a word of an area found in the text a session sees at its start.
type Leak struct {
	Severity LeakSeverity `json:"severity"`
	// Code is the rule that reports the leak (AR9W0 or AR9W1); Leaks leaves it empty.
	Code string `json:"code"`
	// Area is the area the term names; for a warning, every area that uses the word, sorted and joined by "/".
	Area string `json:"area"`
	Term string `json:"term"`
	Kind Kind   `json:"kind"`
	// Name is the qualified name of the artifact.
	Name  string `json:"name"`
	Path  string `json:"path"`
	Scope string `json:"scope"`
	// Line is the file line of a body line (the text line plus the artifact's LineOffset); 1 for a listing
	// line, a server name or a plugin name, and for every warning.
	Line int `json:"line"`
	// Excerpt is the line (for a warning, the whole text), whitespace squashed, cut to 140 characters.
	Excerpt string `json:"excerpt"`
	// Reason is the reason of the acceptance of an accepted leak.
	Reason string `json:"reason"`
}

// Message is the finding message of the leak. It is a public contract: a
// [[lint.accept]] match is a substring of it.
func (l *Leak) Message() string {
	return fmt.Sprintf("%s %q at session start names term %q of area %q", l.Kind, l.Name, l.Term, l.Area)
}

// LeakageReport is what `ai-rulez exposure leakage` prints.
type LeakageReport struct {
	SchemaVersion int `json:"schema_version"`
	// Root is the absolute repository root inspected, slash separated.
	Root string `json:"root"`
	// Plugins is the plugin mode of the inventory.
	Plugins PluginMode `json:"plugins"`
	// AreasFile is the areas file the map was read from, "" for the inline [exposure] form.
	AreasFile string `json:"areas_file"`
	// ArtifactsChecked counts the start-of-session artifacts, an unchecked one included.
	ArtifactsChecked int    `json:"artifacts_checked"`
	Findings         []Leak `json:"findings"`
	// Unchecked are the start-of-session artifacts whose text could not be read, so no leak in them can be
	// found. They are not findings: the inventory lists each as a problem.
	Unchecked []Unchecked `json:"unchecked"`
	// StaleAcceptances describes each [[lint.accept]] entry for a leakage rule that accepted nothing (AR9I0).
	StaleAcceptances []string `json:"stale_acceptances"`
}

// Unchecked is a start-of-session artifact kept unread (over the read limit, or a symlink to outside the
// root): it loads in every session, but its text is not known. Path is relative to the root; Name is the
// qualified name.
type Unchecked struct {
	Path   string `json:"path"`
	Kind   Kind   `json:"kind"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// UncheckedArtifacts lists the artifacts of start that were kept unread, in order.
func UncheckedArtifacts(start []Artifact) []Unchecked {
	var out []Unchecked
	for i := range start {
		a := &start[i]
		if reason := a.UnreadReason(); reason != "" {
			out = append(out, Unchecked{Path: a.Path, Kind: a.Kind, Name: a.QualifiedName, Reason: reason})
		}
	}
	return out
}

// OK reports whether the report passes: no violation and no stale acceptance.
func (r *LeakageReport) OK() bool {
	if len(r.StaleAcceptances) > 0 {
		return false
	}
	return !slices.ContainsFunc(r.Findings, func(l Leak) bool { return l.Severity == LeakViolation })
}

// StartInputs are the root files, besides the artifacts themselves, that
// decide what a session sees at its start: the project settings
// (skillOverrides, claudeMdExcludes, enabledPlugins) and the repository
// marketplace. They are relative to the root and slash separated.
func StartInputs() []string {
	return []string{claudeDir + "/" + settingsShared, claudeDir + "/" + settingsLocal, MarketplaceFile}
}

// Leaks finds the area words in the texts of start, the artifacts a session
// sees at its start (Inventory.Start, which fails closed):
//   - a violation for each term of an area that is not shared, outside path
//     literals, on each non-blank line of each text, in line order and, within
//     a line, by term and then map order;
//   - for a skill or agent listing that names no term at all (a shared one
//     included) but some ambiguous words, a warning per word, after the
//     artifact's violations.
//
// A nil map finds nothing.
func Leaks(start []Artifact, m *areas.Map) []Leak {
	if m == nil {
		return nil
	}
	var out []Leak
	for i := range start {
		a := &start[i]
		out = append(out, violations(a, m)...)
		out = append(out, warnings(a, m)...)
	}
	return out
}

func violations(a *Artifact, m *areas.Map) []Leak {
	var out []Leak
	for i, line := range splitLines(a.Text) {
		if strings.TrimFunc(line, pyIsSpace) == "" {
			continue
		}
		for _, match := range m.Find(line) {
			if !match.Ambiguous && !m.IsShared(match.Area) {
				out = append(out, leak(LeakViolation, match.Area, match.Term, a, a.LineOffset+i+1, line))
			}
		}
	}
	return out
}

func warnings(a *Artifact, m *areas.Map) []Leak {
	if a.Kind != KindSkill && a.Kind != KindAgent {
		return nil
	}
	matches := m.Find(a.Text)
	if slices.ContainsFunc(matches, func(t areas.TermMatch) bool { return !t.Ambiguous }) {
		return nil
	}
	owners := map[string][]string{}
	var terms []string
	for _, t := range matches {
		if _, seen := owners[t.Term]; !seen {
			terms = append(terms, t.Term)
		}
		owners[t.Term] = append(owners[t.Term], t.Area)
	}
	out := make([]Leak, 0, len(terms))
	for _, term := range terms {
		slices.Sort(owners[term])
		out = append(out, leak(LeakWarning, strings.Join(owners[term], "/"), term, a, 1, a.Text))
	}
	return out
}

func leak(sev LeakSeverity, area, term string, a *Artifact, line int, text string) Leak {
	return Leak{Severity: sev, Area: area, Term: term, Kind: a.Kind, Name: a.QualifiedName, Path: a.Path, Scope: a.Scope, Line: line, Excerpt: excerpt(text)}
}

// excerpt is text with its whitespace squashed, cut to excerptWidth characters.
func excerpt(text string) string {
	s := []rune(strings.Join(strings.FieldsFunc(text, pyIsSpace), " "))
	return string(s[:min(len(s), excerptWidth)])
}

// pyIsSpace reports whether Python's str.isspace() accepts r: Go's white space
// plus the information separators U+001C to U+001F.
func pyIsSpace(r rune) bool {
	return unicode.IsSpace(r) || (r >= 0x1c && r <= 0x1f)
}

// UnknownSkillAreas lists, sorted, the keys of skillAreas that name no skill of
// inv: neither the qualified name of a skill nor the bare name of a plugin
// skill, which is how areas.Map.Owner looks a key up (AR9W6).
func UnknownSkillAreas(skillAreas map[string]string, inv *Inventory) []string {
	known := map[string]bool{}
	for i := range inv.Artifacts {
		a := &inv.Artifacts[i]
		if a.Kind != KindSkill {
			continue
		}
		known[a.QualifiedName] = true
		if a.Source == SourcePlugin {
			known[a.Name] = true
		}
	}
	var out []string
	for key := range skillAreas {
		if !known[key] {
			out = append(out, key)
		}
	}
	slices.Sort(out)
	return out
}
