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
	memo := map[[2]int]bool{}
	seen := map[[2]int]bool{}
	var f func(i, j int) bool
	f = func(i, j int) bool {
		key := [2]int{i, j}
		if seen[key] {
			return memo[key]
		}
		seen[key] = true
		var res bool
		switch {
		case i == len(p) && j == len(q):
			res = true
		case i < len(p) && p[i] == '*':
			res = f(i+1, j) || (j < len(q) && f(i, j+1))
		case j < len(q) && q[j] == '*':
			res = f(i, j+1) || (i < len(p) && f(i+1, j))
		case i < len(p) && j < len(q):
			res = (p[i] == q[j] || p[i] == '?' || q[j] == '?') && f(i+1, j+1)
		}
		memo[key] = res
		return res
	}
	return f(0, 0)
}

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
