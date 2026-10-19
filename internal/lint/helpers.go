package lint

import (
	"bytes"
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
	// A line that fits the buffer is lower-cased once (ASCII letters only, so
	// offsets and other bytes are kept) and each ASCII stem is found with the
	// vectorized bytes.Contains instead of visiting every hit of its first letter.
	var buf [foldBufLen]byte
	var low []byte
	if len(s) <= foldBufLen {
		low = lowerASCIIInto(buf[:0], s)
	}
	var lowered string
	for _, st := range stems {
		if isASCII(st) {
			if low != nil && bytes.Contains(low, []byte(st)) || low == nil && indexFoldASCII(s, st) >= 0 {
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

// foldedText is a text lower-cased once for several containsAnyFold checks
// against lower-case stems. A long s does not lower-case to s, and a Kelvin sign
// lower-cases to a k that would end a word, so either keeps every check true, as
// containsAnyFold does.
type foldedText struct {
	low   string
	folds bool
}

func foldText(s string) foldedText {
	return foldedText{low: strings.ToLower(s), folds: foldsToASCII(s) || strings.Contains(s, "\u212a")}
}

// stemIndex finds which of a fixed set of lower-case stems occur in a text in
// one pass over it, instead of one search per stem: a table of every stem's
// first two bytes says where a stem may start. Stems shorter than two bytes
// are compared by strings.Contains. With wordStart set, a stem that opens with
// a letter or digit counts only where it begins a word, for patterns whose
// stems all follow \b or a space ("agree" is not found in "disagree").
type stemIndex struct {
	stems     []string
	pair      map[uint16][]int
	starts    [1 << 16]bool
	short     []int
	wordStart bool
}

func newStemIndex(stems []string) *stemIndex {
	x := &stemIndex{stems: stems, pair: map[uint16][]int{}}
	for i, st := range stems {
		if len(st) < 2 {
			x.short = append(x.short, i)
			continue
		}
		k := uint16(st[0])<<8 | uint16(st[1])
		x.starts[k] = true
		x.pair[k] = append(x.pair[k], i)
	}
	return x
}

// find marks found[i] for every stem i that occurs in folded's text; found must
// be at least as long as the stem list. A long s marks every stem (see foldedText).
func (x *stemIndex) find(folded foldedText, found []bool) {
	if folded.folds {
		for i := range x.stems {
			found[i] = true
		}
		return
	}
	s := folded.low
	for i := 0; i+1 < len(s); i++ {
		k := uint16(s[i])<<8 | uint16(s[i+1])
		if !x.starts[k] {
			continue
		}
		midWord := x.wordStart && i > 0 && isWordByte(s[i-1]) && isWordByte(s[i])
		for _, j := range x.pair[k] {
			if !found[j] && !midWord && strings.HasPrefix(s[i:], x.stems[j]) {
				found[j] = true
			}
		}
	}
	for _, j := range x.short {
		if strings.Contains(s, x.stems[j]) {
			found[j] = true
		}
	}
}

// containsAny reports whether one of the stems occurs in folded's text.
func (x *stemIndex) containsAny(folded foldedText) bool {
	if folded.folds {
		return true
	}
	s := folded.low
	for i := 0; i+1 < len(s); i++ {
		k := uint16(s[i])<<8 | uint16(s[i+1])
		if !x.starts[k] || x.wordStart && i > 0 && isWordByte(s[i-1]) && isWordByte(s[i]) {
			continue
		}
		for _, j := range x.pair[k] {
			if strings.HasPrefix(s[i:], x.stems[j]) {
				return true
			}
		}
	}
	for _, j := range x.short {
		if strings.Contains(s, x.stems[j]) {
			return true
		}
	}
	return false
}

// anyFound reports whether found marks one of the stem indexes; an empty list
// matches everything, like a gatedRe without stems.
func anyFound(found []bool, idx []int) bool {
	if len(idx) == 0 {
		return true
	}
	for _, i := range idx {
		if found[i] {
			return true
		}
	}
	return false
}

// stemGroups is a stemIndex over the union of several stem lists, with each
// list's stems as indexes into the union.
type stemGroups struct {
	index  *stemIndex
	groups [][]int
}

// newWordStemGroups is newStemGroups whose stems must begin a word.
func newWordStemGroups(lists ...[]string) *stemGroups {
	sg := newStemGroups(lists...)
	sg.index.wordStart = true
	return sg
}

func newStemGroups(lists ...[]string) *stemGroups {
	var union []string
	pos := map[string]int{}
	groups := make([][]int, len(lists))
	for g, list := range lists {
		for _, st := range list {
			i, ok := pos[st]
			if !ok {
				i = len(union)
				pos[st] = i
				union = append(union, st)
			}
			groups[g] = append(groups[g], i)
		}
	}
	return &stemGroups{index: newStemIndex(union), groups: groups}
}

// match reports, for each list, whether one of its stems occurs in folded's text.
func (sg *stemGroups) match(folded foldedText) []bool {
	found := make([]bool, len(sg.index.stems))
	sg.index.find(folded, found)
	hit := make([]bool, len(sg.groups))
	for g, idx := range sg.groups {
		hit[g] = anyFound(found, idx)
	}
	return hit
}

// foldBufLen is the longest text containsAnyFold lower-cases on the stack.
const foldBufLen = 1024

// lowerASCIIInto appends s to dst with ASCII letters lower-cased and every
// other byte unchanged.
func lowerASCIIInto(dst []byte, s string) []byte {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		dst = append(dst, c)
	}
	return dst
}

// indexFoldASCII is the index of the first occurrence of stem (lower-case
// ASCII) in s, ignoring ASCII case; -1 when there is none. It jumps between the
// occurrences of the stem's first letter (either case) with IndexByte instead of
// comparing every byte.
func indexFoldASCII(s, stem string) int {
	f := newFoldFinder(s, stem[0])
	for i := f.next(0); i >= 0 && i+len(stem) <= len(s); i = f.next(i + 1) {
		if hasFoldPrefix(s[i+1:], stem[1:]) {
			return i
		}
	}
	return -1
}

// foldFinder finds the positions of one letter in either case, searching each
// case only as far as the previous hit.
type foldFinder struct {
	s      string
	lo, up byte
	nl, nu int
}

func newFoldFinder(s string, lo byte) foldFinder {
	up := lo
	if lo >= 'a' && lo <= 'z' {
		up = lo - 'a' + 'A'
	}
	f := foldFinder{s: s, lo: lo, up: up, nl: -1, nu: -1}
	f.nl = f.index(lo, 0)
	if up != lo {
		f.nu = f.index(up, 0)
	}
	return f
}

// next is the position of the first occurrence of the letter at or after from, or -1.
func (f *foldFinder) next(from int) int {
	if f.nl >= 0 && f.nl < from {
		f.nl = f.index(f.lo, from)
	}
	if f.nu >= 0 && f.nu < from {
		f.nu = f.index(f.up, from)
	}
	if f.nl < 0 || f.nu >= 0 && f.nu < f.nl {
		return f.nu
	}
	return f.nl
}

func (f *foldFinder) index(c byte, from int) int {
	if from > len(f.s) {
		return -1
	}
	i := strings.IndexByte(f.s[from:], c)
	if i < 0 {
		return -1
	}
	return from + i
}

// hasFoldPrefix reports whether s starts with stem (lower-case ASCII), ignoring ASCII case.
func hasFoldPrefix(s, stem string) bool {
	if len(s) < len(stem) {
		return false
	}
	for j := range len(stem) {
		c := s[j]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != stem[j] {
			return false
		}
	}
	return true
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
