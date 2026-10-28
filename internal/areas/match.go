package areas

import (
	"slices"
	"strings"
	"unicode/utf8"
)

// literalChars make a term match as a plain substring; any other term needs word boundaries.
const literalChars = "_/."

// pathLiteralStop are the characters, besides white space, that end a path literal.
const pathLiteralStop = "`'\")],;"

// pathLiteralGuard are the characters, besides word characters, that may not precede a path literal.
const pathLiteralGuard = "/.@-"

// term is one entry of the term alternation.
type term struct {
	runes   []rune
	bounded bool
}

// matcher finds area terms in text the way these regular expressions do under Python's re module:
//
//	terms:          (?i) alternatives longest first, each (?<![A-Za-z0-9])term(?![A-Za-z0-9]) unless it holds _ / .
//	path literals:  (?<![\w/.@-])(?:root|...)/[^\s`'")\],;]*  (root = first segment of a path claim, "*" removed)
//
// RE2 has no lookbehind, so both are scanners over code points with the boundaries as explicit checks.
type matcher struct {
	terms   []term
	byFirst map[rune][]int
	index   map[string][]TermMatch
	roots   map[string]bool
	maxRoot int
}

func newMatcher(areas []Area) *matcher {
	m := &matcher{byFirst: map[rune][]int{}, index: map[string][]TermMatch{}, roots: map[string]bool{}}
	var order []string
	add := func(area, word string, ambiguous bool) {
		if _, ok := m.index[word]; !ok {
			order = append(order, word)
		}
		m.index[word] = append(m.index[word], TermMatch{Area: area, Term: word, Ambiguous: ambiguous})
	}
	for i := range areas {
		a := &areas[i]
		for _, t := range a.Terms {
			add(a.Name, t, false)
		}
		for _, t := range a.Ambiguous {
			add(a.Name, t, true)
		}
		for _, p := range a.Paths {
			root, _, _ := strings.Cut(strings.TrimPrefix(p, excludePrefix), "/")
			root = strings.TrimSuffix(root, namePrefix)
			m.roots[root] = true
			m.maxRoot = max(m.maxRoot, runeLen(root))
		}
	}
	// Longest first; a stable sort keeps first-seen order among equal lengths, as Python's sorted() does.
	slices.SortStableFunc(order, func(a, b string) int { return runeLen(b) - runeLen(a) })
	for i, word := range order {
		runes := []rune(word)
		m.terms = append(m.terms, term{runes: runes, bounded: !strings.ContainsAny(word, literalChars)})
		for _, key := range pyFoldKeys(runes[0]) {
			m.byFirst[key] = append(m.byFirst[key], i)
		}
	}
	for key, list := range m.byFirst {
		slices.Sort(list)
		m.byFirst[key] = slices.Compact(list)
	}
	return m
}

func runeLen(s string) int { return utf8.RuneCountInString(s) }

// Find returns every distinct (area, word) pair whose word occurs in text outside path literals, ordered by
// word and then by map order.
func (m *Map) Find(text string) []TermMatch {
	if m == nil || len(m.matcher.terms) == 0 {
		return nil
	}
	return m.matcher.find(m.matcher.stripPathLiterals(text))
}

// StripPathLiterals blanks out repository path literals (a path claim's first segment followed by "/"):
// naming a location is not naming an area.
func (m *Map) StripPathLiterals(text string) string {
	if m == nil {
		return text
	}
	return m.matcher.stripPathLiterals(text)
}

func (m *matcher) find(text string) []TermMatch {
	runes := []rune(text)
	found := map[string]bool{}
	for i := 0; i < len(runes); {
		n := m.matchAt(runes, i)
		if n == 0 {
			i++
			continue
		}
		found[pyStrLower(string(runes[i:i+n]))] = true
		i += n
	}
	var out []TermMatch
	for _, word := range sortedKeys(found) {
		out = append(out, m.index[word]...)
	}
	return out
}

// matchAt returns the length of the first alternative that matches at runes[i], 0 when none does.
func (m *matcher) matchAt(runes []rune, i int) int {
	candidates := m.byFirst[pyLower(runes[i])]
	if len(candidates) == 0 {
		return 0
	}
	behind := i > 0 && pyIsAlnumFold(runes[i-1])
	for _, idx := range candidates {
		t := m.terms[idx]
		end := i + len(t.runes)
		if (t.bounded && behind) || end > len(runes) {
			continue
		}
		if !matchesFold(t.runes, runes[i:end]) {
			continue
		}
		if t.bounded && end < len(runes) && pyIsAlnumFold(runes[end]) {
			continue
		}
		return len(t.runes)
	}
	return 0
}

func matchesFold(pattern, text []rune) bool {
	for k, r := range pattern {
		if !pyLiteralEqual(r, text[k]) {
			return false
		}
	}
	return true
}

func (m *matcher) stripPathLiterals(text string) string {
	if len(m.roots) == 0 || !strings.Contains(text, "/") {
		return text
	}
	runes := []rune(text)
	nextSlash := make([]int, len(runes)+1)
	nextSlash[len(runes)] = -1
	for i := len(runes) - 1; i >= 0; i-- {
		if runes[i] == '/' {
			nextSlash[i] = i
		} else {
			nextSlash[i] = nextSlash[i+1]
		}
	}
	var b strings.Builder
	b.Grow(len(text))
	for i := 0; i < len(runes); {
		if end := m.pathLiteralAt(runes, nextSlash, i); end > 0 {
			b.WriteByte(' ')
			i = end
			continue
		}
		b.WriteRune(runes[i])
		i++
	}
	return b.String()
}

// pathLiteralAt returns the end of a path literal that starts at runes[i], 0 when none does. A root holds no
// "/", so the only alternative that can match is the text up to the next slash.
func (m *matcher) pathLiteralAt(runes []rune, nextSlash []int, i int) int {
	slash := nextSlash[i]
	if slash < 0 || slash-i > m.maxRoot {
		return 0
	}
	if i > 0 && (pyIsWord(runes[i-1]) || strings.ContainsRune(pathLiteralGuard, runes[i-1])) {
		return 0
	}
	if !m.roots[string(runes[i:slash])] {
		return 0
	}
	end := slash + 1
	for end < len(runes) && !pyIsSpace(runes[end]) && !strings.ContainsRune(pathLiteralStop, runes[end]) {
		end++
	}
	return end
}
