package lint

import (
	"sort"
	"strings"
)

// editDistance is the Levenshtein distance between a and b.
func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}

// closest returns the candidate nearest to word within maxDist (case-insensitive),
// or "" when none is close.
func closest(word string, candidates []string, maxDist int) string {
	best, bestD := "", maxDist+1
	sorted := append([]string(nil), candidates...)
	sort.Strings(sorted)
	for _, c := range sorted {
		if d := editDistance(strings.ToLower(word), strings.ToLower(c)); d < bestD {
			best, bestD = c, d
		}
	}
	return best
}

func inSet(set []string, v string) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}

// presetNames lists the configured preset names.
func (r *runner) presetNames() map[string]bool {
	out := map[string]bool{}
	for _, p := range r.cfg.Presets {
		if p.BuiltIn != "" {
			out[p.BuiltIn] = true
		}
		if p.Name != "" {
			out[p.Name] = true
		}
	}
	return out
}

// targetsClaude reports whether Claude Code reads the content: the claude preset
// is configured, or no preset is (the default).
func (r *runner) targetsClaude() bool {
	names := r.presetNames()
	return len(names) == 0 || names[presetClaude]
}

// fileLines returns the lines of the file at abs, registering it for inline
// ignores, or nil when unreadable.
func (r *runner) fileLines(abs string) []string {
	if d, ok := r.docs[abs]; ok {
		return d.lines
	}
	data, err := readSmallFile(abs)
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	r.docs[abs] = doc{lines: lines}
	return lines
}

// lineContaining is the 1-based line of the first line that holds needle, or 1.
func lineContaining(lines []string, needle string) int {
	if needle == "" {
		return 1
	}
	for i, l := range lines {
		if strings.Contains(l, needle) {
			return i + 1
		}
	}
	return 1
}
