package builtins

import (
	"slices"
	"sort"
	"strings"
)

// UnknownExclusion describes an entry in the user's builtins list that excludes
// something that does not exist: either a whole domain ("!domain") that is not a
// registered builtin, or a per-rule exclusion ("!domain/name") whose domain or
// content file is absent. Such an entry suppresses nothing, so the content the
// author meant to drop is still emitted.
type UnknownExclusion struct {
	// Spec is the identifier as written, without the leading "!".
	Spec string
	// Suggestion is the closest real identifier, or "" when no candidate is
	// close enough for a guess to be useful.
	Suggestion string
}

// UnknownExclusions reports the exclusions in the user's builtins list that do
// not resolve against the builtin registry and its embedded content.
//
// Exclusions are the only lever for suppressing an always-loaded builtin rule, so
// an unresolved one is a silent cost: the author believes a rule was dropped while
// every request still pays for it. Callers surface these as warnings rather than
// errors — a builtin legitimately renamed or removed upstream must not break
// projects that still exclude it.
func UnknownExclusions(list []string) []UnknownExclusion {
	var unknown []UnknownExclusion
	seen := make(map[string]bool, len(list))
	for _, entry := range list {
		if !strings.HasPrefix(entry, "!") {
			continue
		}
		spec := strings.TrimPrefix(entry, "!")
		if spec == "" || seen[spec] || exclusionResolves(spec) {
			continue
		}
		seen[spec] = true
		unknown = append(unknown, UnknownExclusion{
			Spec:       spec,
			Suggestion: nearestExclusion(spec),
		})
	}
	return unknown
}

// exclusionResolves reports whether an exclusion spec ("domain" or "domain/name")
// names something that actually exists. The qualified form is matched the same way
// loadBuiltins matches it — against a single content entry name within the domain —
// so a spec with extra path segments never resolves.
func exclusionResolves(spec string) bool {
	domain, name, qualified := strings.Cut(spec, "/")
	if _, ok := registry[domain]; !ok {
		return false
	}
	if !qualified {
		return true
	}
	return slices.Contains(domainContentNames(domain), name)
}

// domainContentNames returns the content entry names (rules, context, skills,
// agents, commands) of a builtin domain — the set a "!domain/name" exclusion can
// match.
func domainContentNames(domain string) []string {
	entries, err := LoadDomainContent(domain)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for i := range entries {
		names = append(names, entries[i].Name)
	}
	return names
}

// nearestExclusion returns the real identifier closest to an unresolved spec, or
// "" when nothing is close. The dominant failure mode is a typo, so naming the
// intended identifier is usually the whole fix.
func nearestExclusion(spec string) string {
	budget := suggestionBudget(spec)
	best := ""
	bestDistance := -1
	for _, candidate := range exclusionCandidates(spec) {
		distance := levenshtein(spec, candidate)
		if distance > budget {
			continue
		}
		if bestDistance == -1 || distance < bestDistance {
			best = candidate
			bestDistance = distance
		}
	}
	return best
}

// exclusionCandidates returns the identifiers an unresolved spec is compared
// against: domain names for a bare spec, the domain's own content identifiers when
// the domain exists (the typo is in the content name), and every qualified
// identifier when even the domain is unknown. Sorted, so ties resolve
// deterministically.
func exclusionCandidates(spec string) []string {
	domain, _, qualified := strings.Cut(spec, "/")
	if !qualified {
		return ResolveAll()
	}
	if _, ok := registry[domain]; ok {
		return qualifiedContentNames(domain)
	}
	var all []string
	for _, name := range ResolveAll() {
		all = append(all, qualifiedContentNames(name)...)
	}
	sort.Strings(all)
	return all
}

// qualifiedContentNames returns the domain's content entries as "domain/name"
// exclusion identifiers, sorted.
func qualifiedContentNames(domain string) []string {
	names := domainContentNames(domain)
	qualified := make([]string, 0, len(names))
	for _, name := range names {
		qualified = append(qualified, domain+"/"+name)
	}
	sort.Strings(qualified)
	return qualified
}

// suggestionBudget is the largest edit distance still treated as a plausible typo,
// scaled to the length of the spec so short names are not matched to unrelated
// ones and long names tolerate a couple of slips.
func suggestionBudget(spec string) int {
	budget := len([]rune(spec)) / 3
	if budget < 2 {
		return 2
	}
	return budget
}

// levenshtein returns the edit distance between a and b, comparing runes so
// multi-byte identifiers are measured by character rather than by byte.
func levenshtein(a, b string) int {
	ar := []rune(a)
	br := []rune(b)
	if len(ar) == 0 {
		return len(br)
	}
	if len(br) == 0 {
		return len(ar)
	}

	previous := make([]int, len(br)+1)
	current := make([]int, len(br)+1)
	for j := range previous {
		previous[j] = j
	}

	for i := 1; i <= len(ar); i++ {
		current[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			current[j] = min(previous[j]+1, current[j-1]+1, previous[j-1]+cost)
		}
		previous, current = current, previous
	}
	return previous[len(br)]
}
