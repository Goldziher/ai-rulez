package exposure

import (
	"fmt"
	"path"
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
	// ImportedBy are the paths of the start-of-session artifacts that import Path with `@path`, in inventory
	// order, for a leak in an imported file; empty otherwise.
	ImportedBy []string `json:"imported_by,omitempty"`
	// ImportChain is every file the leak depends on: the importing artifacts and everything they import. It
	// is not printed; `--changed` keeps the leak when any of them changed.
	ImportChain []string `json:"-"`
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

// UncheckedArtifacts lists, in order, the artifacts of start that were kept unread and the files they import
// that were not read: an import over the read limit, or one the inventory does not follow (an absolute or
// home path, one outside the root, an unreadable target). An imported file is listed under the importing
// artifact's kind and name, with its path as written for the latter.
func UncheckedArtifacts(start []Artifact) []Unchecked {
	var out []Unchecked
	seen := map[Unchecked]bool{}
	add := func(u Unchecked) {
		if !seen[u] {
			seen[u] = true
			out = append(out, u)
		}
	}
	for i := range start {
		a := &start[i]
		if reason := a.UnreadReason(); reason != "" {
			add(Unchecked{Path: a.Path, Kind: a.Kind, Name: a.QualifiedName, Reason: reason})
			continue
		}
		for _, im := range a.Imports {
			if im.Text == "" && im.TextBytes > 0 {
				add(Unchecked{Path: im.Path, Kind: a.Kind, Name: a.QualifiedName, Reason: "imported by " + a.Path + ": the file is over the read limit"})
			}
		}
		for _, p := range a.ImportProblems {
			add(Unchecked{Path: p.Path, Kind: a.Kind, Name: a.QualifiedName, Reason: "imported by " + a.Path + ": " + p.Reason})
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
	leaks, _ := CheckLeaks(start, m)
	return leaks
}

// CheckLeaks is Leaks with the recorded acceptances of the area map applied: a leak an [[accept]] entry covers
// stays in the result with the entry's reason in Reason. stale are the entries that accepted nothing.
//
// The text of each file a start-of-session CLAUDE.md or rule imports with `@path` is scanned as the text the
// session loads, on the lines of the imported file and reported against its path, once per file; a file that is
// itself a start-of-session artifact is scanned as that artifact.
func CheckLeaks(start []Artifact, m *areas.Map) (leaks []Leak, stale []areas.Accept) {
	return CheckLeaksFrom(start, m, "")
}

// CheckLeaksFrom is CheckLeaks for an inventory of the directory rel (slash separated, "" for the project root)
// below the project root: the leak paths are relative to that directory, while the acceptances of the area map
// are relative to the project root, so they are matched against the path under rel.
func CheckLeaksFrom(start []Artifact, m *areas.Map, rel string) (leaks []Leak, stale []areas.Accept) {
	if m == nil {
		return nil, nil
	}
	artifacts := map[string]bool{}
	for i := range start {
		artifacts[start[i].Path] = true
	}
	importers := importerIndex(start, artifacts)
	scanned := map[string]bool{}
	for i := range start {
		a := &start[i]
		leaks = append(leaks, violations(a, m)...)
		for _, im := range a.Imports {
			if im.Text == "" || artifacts[im.Path] || scanned[im.Path] {
				continue
			}
			scanned[im.Path] = true
			found := importViolations(a, &im, m)
			by, chain := importerPaths(start, importers[im.Path])
			for j := range found {
				found[j].ImportedBy, found[j].ImportChain = by, chain
			}
			leaks = append(leaks, found...)
		}
		leaks = append(leaks, warnings(a, m)...)
	}
	used := map[int]bool{}
	for i := range leaks {
		if acc, ok := m.Accepting(leaks[i].Area, leaks[i].Term, path.Join(rel, leaks[i].Path)); ok {
			leaks[i].Reason = acc.Reason
			used[acc.Index] = true
		}
	}
	for _, acc := range m.Accepts() {
		if !used[acc.Index] {
			stale = append(stale, acc)
		}
	}
	return leaks, stale
}

// importerIndex maps each imported file that is scanned as imported text (not itself a start-of-session
// artifact) to the indexes of the artifacts of start that import it.
func importerIndex(start []Artifact, artifacts map[string]bool) map[string][]int {
	importers := map[string][]int{}
	for i := range start {
		for _, im := range start[i].Imports {
			if im.Text != "" && !artifacts[im.Path] {
				importers[im.Path] = append(importers[im.Path], i)
			}
		}
	}
	return importers
}

// importerPaths are the paths of the artifacts at the given indexes of start, and with them every file those
// artifacts import: what a leak found through them depends on.
func importerPaths(start []Artifact, idx []int) (by, chain []string) {
	seen := map[string]bool{}
	add := func(p string) {
		if !seen[p] {
			seen[p] = true
			chain = append(chain, p)
		}
	}
	for _, i := range idx {
		by = append(by, start[i].Path)
		add(start[i].Path)
		for _, im := range start[i].Imports {
			add(im.Path)
		}
	}
	return by, chain
}

func violations(a *Artifact, m *areas.Map) []Leak {
	return lineViolations(a, a.Path, a.Text, a.LineOffset, m)
}

// importViolations scans the text of a file the artifact imports; its lines are the file's own.
func importViolations(a *Artifact, im *Import, m *areas.Map) []Leak {
	return lineViolations(a, im.Path, im.Text, 0, m)
}

func lineViolations(a *Artifact, filePath, text string, offset int, m *areas.Map) []Leak {
	var out []Leak
	for i, line := range splitLines(text) {
		if strings.TrimFunc(line, pyIsSpace) == "" {
			continue
		}
		for _, match := range m.Find(line) {
			if !match.Ambiguous && !m.IsShared(match.Area) {
				l := leak(LeakViolation, match.Area, match.Term, a, offset+i+1, line)
				l.Path = filePath
				out = append(out, l)
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
