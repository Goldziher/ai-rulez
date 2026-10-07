package settings

import "strings"

// globsOverlap reports whether some string matches both patterns, where `*`
// matches any run of characters and `?` any single one. It is conservative for
// patterns whose real matcher is richer: callers use it to decide whether two
// rules can disagree about the same input.
func globsOverlap(a, b string) bool {
	for _, x := range globVariants(a) {
		for _, y := range globVariants(b) {
			if globIntersect(x, y) {
				return true
			}
		}
	}
	return false
}

// globVariants adds the form without a trailing " *": OpenCode and its forks
// match `git *` against the bare command `git` too.
func globVariants(p string) []string {
	if head, ok := strings.CutSuffix(p, " *"); ok {
		return []string{p, head}
	}
	return []string{p}
}

func globIntersect(p, q string) bool {
	m := globMatcher{p: p, q: q, memo: map[[2]int]bool{}, seen: map[[2]int]bool{}}
	return m.from(0, 0)
}

// globMatcher decides whether two patterns share a string, memoising the
// (position in p, position in q) pairs it has settled.
type globMatcher struct {
	p, q       string
	memo, seen map[[2]int]bool
}

// from reports whether p[i:] and q[j:] share a string.
func (m *globMatcher) from(i, j int) bool {
	key := [2]int{i, j}
	if m.seen[key] {
		return m.memo[key]
	}
	m.seen[key] = true
	res := m.step(i, j)
	m.memo[key] = res
	return res
}

// step consumes one position of either pattern: a star may match nothing or one
// more character, a literal or `?` must meet its counterpart.
func (m *globMatcher) step(i, j int) bool {
	pLeft, qLeft := i < len(m.p), j < len(m.q)
	switch {
	case !pLeft && !qLeft:
		return true
	case pLeft && m.p[i] == '*':
		return m.from(i+1, j) || (qLeft && m.from(i, j+1))
	case qLeft && m.q[j] == '*':
		return m.from(i, j+1) || (pLeft && m.from(i+1, j))
	case pLeft && qLeft:
		return charsMeet(m.p[i], m.q[j]) && m.from(i+1, j+1)
	}
	return false
}

// charsMeet reports whether two pattern characters can match the same character.
func charsMeet(a, b byte) bool { return a == b || a == '?' || b == '?' }

// strictness orders the actions: a later match of a weaker action than an
// earlier overlapping stricter one would relax it.
func strictness(a PermAction) int {
	switch a {
	case ActionDeny:
		return 2
	case ActionAsk:
		return 1
	}
	return 0
}
