package rulefiles

import (
	"strconv"
	"strings"
)

// ExpandBraces expands {a,b} groups (nested allowed) into separate globs:
// "*.{ts,tsx}" becomes ["*.ts", "*.tsx"]. Character classes ([a,b]) and
// backslash-escaped characters are left untouched, as is an unbalanced "{".
// A glob without braces is returned as a single element.
//
// Expansion is capped at maxExpansion alternatives; a glob that would exceed
// it is returned unexpanded (use expandBraces to learn whether that happened).
func ExpandBraces(glob string) []string {
	out, _ := expandBraces(glob)
	return out
}

// maxExpansion bounds the number of alternatives one glob may expand to.
const maxExpansion = 256

// expandBraces is ExpandBraces plus a flag that is false when the expansion
// exceeded the cap and the glob was returned as is.
func expandBraces(glob string) (expanded []string, ok bool) {
	out, ok := expandLimited(glob)
	if !ok {
		return []string{glob}, false
	}
	return out, true
}

func expandLimited(glob string) ([]string, bool) {
	open, closeIdx := findBraceGroup(glob)
	if open < 0 {
		return []string{glob}, true
	}
	prefix, suffix := glob[:open], glob[closeIdx+1:]
	var out []string
	for _, alt := range splitTopLevel(glob[open+1 : closeIdx]) {
		sub, ok := expandLimited(prefix + alt + suffix)
		if !ok {
			return nil, false
		}
		out = append(out, sub...)
		if len(out) > maxExpansion {
			return nil, false
		}
	}
	return out, true
}

// findBraceGroup returns the indexes of the first top-level "{" and its
// matching "}", or -1, -1 when there is none.
func findBraceGroup(s string) (open, closeIdx int) {
	open = -1
	depth, brackets := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '[':
			brackets++
		case ']':
			if brackets > 0 {
				brackets--
			}
		case '{':
			if brackets > 0 {
				continue
			}
			if depth == 0 {
				open = i
			}
			depth++
		case '}':
			if brackets > 0 || depth == 0 {
				continue
			}
			depth--
			if depth == 0 {
				return open, i
			}
		}
	}
	return -1, -1
}

// splitTopLevel splits on commas outside nested braces and brackets and not
// escaped.
func splitTopLevel(s string) []string {
	var parts []string
	depth, brackets, start := 0, 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '[':
			brackets++
		case ']':
			if brackets > 0 {
				brackets--
			}
		case '{':
			if brackets == 0 {
				depth++
			}
		case '}':
			if brackets == 0 {
				depth--
			}
		case ',':
			if depth == 0 && brackets == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	return append(parts, s[start:])
}

// joinExpanded expands braces in every glob and joins the distinct results
// with commas, for dialects that carry all globs in one string. The returned
// notes name globs whose expansion hit the cap and were kept as written.
func joinExpanded(rule string, globs []string) (joined string, notes []string) {
	var all []string
	seen := map[string]struct{}{}
	for _, g := range globs {
		exp, ok := expandBraces(g)
		if !ok {
			notes = append(notes, "rule \""+rule+"\": glob "+strconv.Quote(g)+
				" expands to more than "+strconv.Itoa(maxExpansion)+" alternatives; kept unexpanded")
		}
		for _, e := range exp {
			if _, dup := seen[e]; dup {
				continue
			}
			seen[e] = struct{}{}
			all = append(all, e)
		}
	}
	return strings.Join(all, ","), notes
}
