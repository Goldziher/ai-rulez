// Package areas is the area map of a repository: which part of the tree each area claims, who owns it, and the
// words that name it. It loads and validates the map ([exposure] areas or areas_file, rule AR9W5), finds area
// terms in text, classifies an artifact by its directory and text, and resolves the owning area of a skill.
//
// Term matching follows the semantics of Python's re module with re.IGNORECASE on str patterns (CPython 3.11)
// exactly; a differential test holds the matcher to recorded vectors.
package areas

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// Unclassified is the area of an artifact that no path claim, term majority or explicit assignment places.
const Unclassified = "unclassified"

// Source says how an owning area was determined.
type Source string

// Sources of an owning area, in the order they are tried.
const (
	SourceSkillAreas Source = "skill_areas"
	SourcePath       Source = "path"
	SourceTerms      Source = "terms"
	SourceNone       Source = "none"
)

const (
	excludePrefix = "!"
	namePrefix    = "*"
	namespaceSep  = ":"
)

// Area is one validated area: terms and ambiguous words are lowercased, trimmed, unique and sorted, and a word
// that is both a term and ambiguous is a term.
type Area struct {
	Name        string
	Description string
	Owner       string
	Paths       []string
	Terms       []string
	Ambiguous   []string
	Shared      bool
	Declare     string
}

// TermMatch is one area word found in a text.
type TermMatch struct {
	Area      string
	Term      string
	Ambiguous bool
}

// Owner is the owning area of a skill and how it was determined.
type Owner struct {
	Area   string
	Source Source
}

// Problem is one reason the area map cannot be used (AR9W5).
type Problem struct {
	// File is the areas file the problem is in; "" means the [exposure] table of the config.
	File string
	// Line is the 1-based line in File, 0 when unknown.
	Line int
	// Message describes the problem.
	Message string
	// Key is text of a scalar [exposure] key that locates its line ("" when none does); an entry of the area
	// map is located by LineIn.
	Key string
	// area is the 1-based area table the problem is in, 0 for none; needle is text in that table that locates
	// the line (the table's header when it is ""), preferably on the line assigning field.
	area   int
	field  string
	needle string
	// skill is the [skill_areas] key the problem is about.
	skill string
}

// Map is a validated area map. Its methods are safe for concurrent use; a nil *Map has no areas and classifies
// everything as Unclassified.
type Map struct {
	areas      []Area
	skillAreas map[string]string
	matcher    *matcher
}

// New validates and normalizes an area map. It returns the map, or nil and every problem found; a map with
// problems is never returned, so the rules that read it stay quiet until it is fixed.
func New(defs []config.ExposureArea, skillAreas map[string]string) (*Map, []Problem) {
	areas := make([]Area, 0, len(defs))
	var problems []Problem
	seen := map[string]bool{}
	for i := range defs {
		area, areaProblems := normalizeArea(i, &defs[i])
		problems = append(problems, areaProblems...)
		if area.Name != "" && seen[area.Name] {
			problems = append(problems, Problem{Message: fmt.Sprintf("duplicate area name %q", area.Name), area: i + 1, field: keyName, needle: quoted(area.Name)})
		}
		seen[area.Name] = true
		areas = append(areas, area)
	}
	for _, skill := range sortedKeys(skillAreas) {
		area := skillAreas[skill]
		switch {
		case strings.TrimSpace(skill) == "":
			problems = append(problems, Problem{Message: "skill_areas has an empty skill name", skill: skill})
		case !seen[area]:
			problems = append(problems, Problem{Message: fmt.Sprintf("skill_areas assigns %q to %q, which names no area", skill, area), skill: skill})
		}
	}
	if len(problems) > 0 {
		return nil, problems
	}
	m := &Map{areas: areas, skillAreas: copyMap(skillAreas)}
	m.matcher = newMatcher(areas)
	return m, nil
}

func normalizeArea(index int, def *config.ExposureArea) (Area, []Problem) {
	area := Area{
		Name:        def.Name,
		Description: def.Description,
		Owner:       def.Owner,
		Paths:       slices.Clone(def.Paths),
		Terms:       normalizeTerms(def.Terms),
		Shared:      def.Shared,
		Declare:     def.Declare,
	}
	terms := map[string]bool{}
	for _, t := range area.Terms {
		terms[t] = true
	}
	for _, a := range normalizeTerms(def.Ambiguous) {
		if !terms[a] {
			area.Ambiguous = append(area.Ambiguous, a)
		}
	}
	var problems []Problem
	add := func(field, needle, format string, args ...any) {
		problems = append(problems, Problem{Message: fmt.Sprintf(format, args...), area: index + 1, field: field, needle: needle})
	}
	switch {
	case def.Name == "":
		add(keyName, "", "area %d has no name", index+1)
	case def.Name == Unclassified:
		add(keyName, quoted(def.Name), "area name %q is reserved", def.Name)
	case !isKebab(def.Name):
		add(keyName, quoted(def.Name), "area name %q must be lower-kebab (a-z, 0-9 and single hyphens)", def.Name)
	}
	for _, p := range def.Paths {
		if why := badPath(p); why != "" {
			add(keyPaths, quoted(p), "area %q path %q %s", def.Name, p, why)
		}
	}
	if why := badDeclare(def.Declare); why != "" {
		add(keyDeclare, quoted(def.Declare), "area %q declare %q %s", def.Name, def.Declare, why)
	}
	return area, problems
}

// badPath says why a path claim is invalid, or "" when it is valid: a non-empty repository-relative prefix
// with no leading or trailing slash, no empty, "." or ".." segment, no backslash, and "*" only at the end.
// "dir/*" is a name prefix like any other: everything below dir, not dir itself.
func badPath(p string) string {
	stem := strings.TrimPrefix(p, excludePrefix)
	switch {
	case stem == "":
		return "is empty"
	case strings.HasPrefix(stem, "/") || strings.HasSuffix(stem, "/"):
		return "must be a repository-relative prefix without a leading or trailing slash"
	case strings.Contains(stem, `\`):
		return "must use forward slashes"
	case strings.Contains(strings.TrimSuffix(stem, namePrefix), namePrefix):
		return `may have "*" only at the end`
	}
	for _, segment := range strings.Split(strings.TrimSuffix(strings.TrimSuffix(stem, namePrefix), "/"), "/") {
		if segment == "." || segment == ".." || (segment == "" && stem != namePrefix) {
			return `must not have an empty, "." or ".." segment`
		}
	}
	return ""
}

// badDeclare says why a declare path is invalid, or "" when it is valid or unset.
func badDeclare(p string) string {
	if p == "" {
		return ""
	}
	if strings.HasPrefix(p, "/") || strings.Contains(p, `\`) || slices.Contains(strings.Split(p, "/"), "..") {
		return "must be a repository-relative path inside the repository"
	}
	return ""
}

// normalizeTerms trims and lowercases words (Python's str.strip().lower()), drops empty ones, and returns them
// unique and sorted.
func normalizeTerms(values []string) []string {
	set := map[string]bool{}
	for _, v := range values {
		if t := strings.TrimFunc(v, pyIsSpace); t != "" {
			set[pyStrLower(t)] = true
		}
	}
	return sortedKeys(set)
}

// isKebab reports whether name is lower-kebab: runs of a-z and 0-9 joined by single hyphens.
func isKebab(name string) bool {
	for _, part := range strings.Split(name, "-") {
		if part == "" || strings.Trim(part, "abcdefghijklmnopqrstuvwxyz0123456789") != "" {
			return false
		}
	}
	return true
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func copyMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// The keys of an area table whose lines a problem points at.
const (
	keyName    = "name"
	keyPaths   = "paths"
	keyDeclare = "declare"
)

func quoted(s string) string { return `"` + s + `"` }

// Areas returns the areas in map order.
func (m *Map) Areas() []Area {
	if m == nil {
		return nil
	}
	out := make([]Area, len(m.areas))
	for i := range m.areas {
		out[i] = m.areas[i].clone()
	}
	return out
}

// Area looks an area up by name.
func (m *Map) Area(name string) (Area, bool) {
	if m != nil {
		for i := range m.areas {
			if m.areas[i].Name == name {
				return m.areas[i].clone(), true
			}
		}
	}
	return Area{}, false
}

// IsShared reports whether name is a shared area: its terms classify but never leak.
func (m *Map) IsShared(name string) bool {
	a, ok := m.Area(name)
	return ok && a.Shared
}

// SkillAreas returns the explicit skill assignments.
func (m *Map) SkillAreas() map[string]string {
	if m == nil {
		return nil
	}
	return copyMap(m.skillAreas)
}

func (a *Area) clone() Area {
	out := *a
	out.Paths, out.Terms, out.Ambiguous = slices.Clone(a.Paths), slices.Clone(a.Terms), slices.Clone(a.Ambiguous)
	return out
}
