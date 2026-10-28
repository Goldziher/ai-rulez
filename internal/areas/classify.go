package areas

import "strings"

// RootScope is the repository root as a directory scope. Every function that takes a scope also accepts ".",
// the root of the repository index and the exposure inventory, so the two conventions cannot be mixed up.
const RootScope = ""

// normScope maps "." to RootScope.
func normScope(scope string) string {
	if scope == "." {
		return RootScope
	}
	return scope
}

// IsUnder reports whether the repository-relative directory scope is prefix or inside it; every scope is under
// the repository root ("" or ".").
func IsUnder(scope, prefix string) bool {
	scope, prefix = normScope(scope), normScope(prefix)
	return prefix == RootScope || scope == prefix || strings.HasPrefix(scope, prefix+"/")
}

// patternLength is the length of pattern when it covers scope, else 0. A trailing "*" makes it a name prefix.
func patternLength(scope, pattern string) int {
	if stem, ok := strings.CutSuffix(pattern, namePrefix); ok {
		if strings.HasPrefix(scope, stem) {
			return runeLen(stem)
		}
		return 0
	}
	if IsUnder(scope, pattern) {
		return runeLen(pattern)
	}
	return 0
}

// MatchLength is the specificity of the area's claim on the directory scope (the root is "" or "."): the length of its longest
// matching path, 0 when none matches or an exclude ("!" path) covers scope.
func (a Area) MatchLength(scope string) int {
	scope = normScope(scope)
	for _, p := range a.Paths {
		if stem, ok := strings.CutPrefix(p, excludePrefix); ok && patternLength(scope, stem) > 0 {
			return 0
		}
	}
	best := 0
	for _, p := range a.Paths {
		if !strings.HasPrefix(p, excludePrefix) {
			best = max(best, patternLength(scope, p))
		}
	}
	return best
}

// Owns reports whether the directory scope lies in one of the area's claims.
func (a Area) Owns(scope string) bool { return a.MatchLength(scope) > 0 }

// Claim returns the area with the most specific path claim on the directory scope (the first in map order on
// a tie), or "" when no area claims it.
func (m *Map) Claim(scope string) string {
	if m == nil {
		return ""
	}
	best, length := "", 0
	for i := range m.areas {
		if n := m.areas[i].MatchLength(scope); n > length {
			best, length = m.areas[i].Name, n
		}
	}
	return best
}

// Classify returns the area of an artifact in the directory scope (the root is "" or ".") with the given text:
// the most specific path claim, else the area with the most distinct non-ambiguous terms in text, else
// Unclassified (also on a tie).
func (m *Map) Classify(scope, text string) string {
	area, _ := m.ClassifyWithSource(scope, text)
	return area
}

// ClassifyWithSource is Classify that also says whether a path claim, the terms or nothing decided.
func (m *Map) ClassifyWithSource(scope, text string) (string, Source) {
	if area := m.Claim(scope); area != "" {
		return area, SourcePath
	}
	scores := map[string]int{}
	for _, match := range m.Find(text) {
		if !match.Ambiguous {
			scores[match.Area]++
		}
	}
	top, leaders := 0, []string(nil)
	for area, score := range scores {
		switch {
		case score > top:
			top, leaders = score, []string{area}
		case score == top:
			leaders = append(leaders, area)
		}
	}
	if len(leaders) != 1 {
		return Unclassified, SourceNone
	}
	return leaders[0], SourceTerms
}

// Skill identifies a skill for owner resolution. Build it from an inventory artifact with
// exposure.Artifact.AreaSkill; the fields differ between repository and plugin skills.
type Skill struct {
	// Name is the skill's name; for a plugin skill, without the plugin prefix.
	Name string
	// Plugin is the plugin that provides the skill, "" for a repository skill.
	Plugin string
	// Scope is the directory whose path claim owns the skill: for a repository skill the directory that holds
	// its .claude directory (RootScope, "" or ".", at the root), for a plugin skill the plugin's directory.
	Scope string
	// Text is what term classification reads: a repository skill's listing text (`name: description`), a plugin
	// skill's qualified name (`plugin:skill`), so a plugin skill is never placed by its description.
	Text string
}

// QualifiedName is plugin:name for a plugin skill and the name for a repository skill.
func (s Skill) QualifiedName() string {
	if s.Plugin == "" {
		return s.Name
	}
	return s.Plugin + namespaceSep + s.Name
}

// Owner resolves the owning area of a skill: an explicit skill_areas entry (for a plugin skill, its qualified
// name, then its bare name), then the path claim on its scope, then the terms of its text, else Unclassified.
func (m *Map) Owner(s Skill) Owner {
	names := []string{s.QualifiedName()}
	if s.Plugin != "" {
		names = append(names, s.Name)
	}
	if m != nil {
		for _, name := range names {
			if area := m.skillAreas[name]; area != "" {
				return Owner{Area: area, Source: SourceSkillAreas}
			}
		}
	}
	area, source := m.ClassifyWithSource(s.Scope, s.Text)
	return Owner{Area: area, Source: source}
}
