package lint

import (
	"regexp"
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

// gatedRe is a regular expression behind a literal pre-check. The pattern is
// only run on text that contains at least one of the stems (compared without
// case when fold is set), which skips the backtracking search on the great
// majority of lines. The stems must be literals that every match contains: an
// over-broad stem costs speed, a missing one would lose matches.
type gatedRe struct {
	re    *regexp.Regexp
	stems []string
	fold  bool
	// wordStart requires each ASCII stem to begin a word (the pattern opens
	// with \b), which rejects "whenever" for "never".
	wordStart bool
}

func newGatedRe(pattern string, fold bool, stems ...string) gatedRe {
	for i, s := range stems {
		if fold {
			stems[i] = strings.ToLower(s)
		}
	}
	return gatedRe{re: regexp.MustCompile(pattern), stems: stems, fold: fold}
}

// newWordGatedRe is newGatedRe for a pattern whose every alternative opens with
// a word boundary, so each ASCII stem must begin a word.
func newWordGatedRe(pattern string, fold bool, stems ...string) gatedRe {
	g := newGatedRe(pattern, fold, stems...)
	g.wordStart = true
	return g
}

// mayMatch reports whether s holds one of the stems.
func (g gatedRe) mayMatch(s string) bool {
	if len(g.stems) == 0 {
		return true
	}
	if g.wordStart {
		return containsWordStartStem(s, g.fold, g.stems) || (g.fold && foldsToASCII(s))
	}
	return containsAnyStem(s, g.fold, g.stems)
}

// foldsToASCII reports whether s holds a character the pattern's case folding
// equates with an ASCII letter although lower-casing it does not: U+017F (long s)
// folds to s. The Kelvin sign U+212A lower-cases to k, so it needs no case here.
func foldsToASCII(s string) bool { return strings.Contains(s, "\u017f") }

func containsWordStartStem(s string, fold bool, stems []string) bool {
	if fold {
		s = strings.ToLower(s)
	}
	for _, st := range stems {
		for from := 0; ; {
			i := strings.Index(s[from:], st)
			if i < 0 {
				break
			}
			i += from
			if i == 0 || st[0] >= 0x80 || !isWordByte(s[i-1]) {
				return true
			}
			from = i + 1
		}
	}
	return false
}

func isWordByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_'
}

func containsAnyStem(s string, fold bool, stems []string) bool {
	if fold {
		return containsAnyFold(s, stems)
	}
	for _, st := range stems {
		if strings.Contains(s, st) {
			return true
		}
	}
	return false
}

// containsAnyFold is containsAnyStem without case. It compares ASCII stems in
// place, so it allocates nothing for the common line; a stem with other
// characters is compared against the lower-cased text. Like the patterns it
// guards, it treats U+017F (long s) as an s.
func containsAnyFold(s string, stems []string) bool {
	var lowered string
	for _, st := range stems {
		if isASCII(st) {
			if indexFoldASCII(s, st) >= 0 {
				return true
			}
			continue
		}
		if lowered == "" {
			lowered = strings.ToLower(s)
		}
		if strings.Contains(lowered, st) {
			return true
		}
	}
	return foldsToASCII(s) || strings.Contains(s, "\u212a")
}

// indexFoldASCII is the index of the first occurrence of stem (lower-case
// ASCII) in s, ignoring ASCII case; -1 when there is none.
func indexFoldASCII(s, stem string) int {
	lower := func(c byte) byte {
		if c >= 'A' && c <= 'Z' {
			return c + 'a' - 'A'
		}
		return c
	}
	for i := 0; i+len(stem) <= len(s); i++ {
		if lower(s[i]) != stem[0] {
			continue
		}
		j := 1
		for j < len(stem) && lower(s[i+j]) == stem[j] {
			j++
		}
		if j == len(stem) {
			return i
		}
	}
	return -1
}

func (g gatedRe) MatchString(s string) bool { return g.mayMatch(s) && g.re.MatchString(s) }

func (g gatedRe) FindAllStringIndex(s string, n int) [][]int {
	if !g.mayMatch(s) {
		return nil
	}
	return g.re.FindAllStringIndex(s, n)
}

func (g gatedRe) FindString(s string) string {
	if !g.mayMatch(s) {
		return ""
	}
	return g.re.FindString(s)
}

// hasCmdWord reports whether s holds one of words as a whole word (the
// characters next to it are not letters, digits, "_" or "-"). It is a cheap,
// slightly loose pre-check for a shell command name; the real pattern decides.
func hasCmdWord(s string, words ...string) bool {
	isWord := func(c byte) bool { return isWordByte(c) || c == '-' }
	for _, w := range words {
		for from := 0; ; {
			i := strings.Index(s[from:], w)
			if i < 0 {
				break
			}
			i += from
			end := i + len(w)
			if (i == 0 || !isWord(s[i-1])) && (end == len(s) || !isWord(s[end])) {
				return true
			}
			from = i + 1
		}
	}
	return false
}
