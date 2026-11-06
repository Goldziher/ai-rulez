// Package pathglob matches slash-separated repository-relative paths against the glob dialect of the lint
// path settings ([[lint.accept]], ignore_paths and friends), so every table that names a path reads it alike.
package pathglob

import (
	"regexp"
	"strings"
)

// Matcher matches slash-separated repo-relative paths against one glob.
// Semantics follow the way assistants read `paths`/`globs`: ** crosses
// directories, * does not, {a,b} alternates, a pattern with no slash matches at
// any depth, and a pattern naming a directory matches everything below it.
type Matcher struct{ re *regexp.Regexp }

// New compiles pattern; ok is false for an empty or uncompilable one.
func New(pattern string) (Matcher, bool) {
	p := strings.TrimSpace(pattern)
	p = strings.TrimPrefix(p, "./")
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return Matcher{}, false
	}
	if strings.HasSuffix(p, "/") {
		p += "**"
	}
	var alts []string
	for _, expanded := range ExpandBraces(p) {
		alts = append(alts, globBody(expanded))
	}
	re, err := regexp.Compile("^(?:" + strings.Join(alts, "|") + ")$")
	if err != nil {
		return Matcher{}, false
	}
	return Matcher{re: re}, true
}

// Malformed reports whether pattern opens a character class or a brace group it never closes,
// or closes a brace group it never opened: such a pattern would silently match nothing.
func Malformed(pattern string) bool {
	depth := 0
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '\\':
			i++
		case '[':
			end := strings.IndexByte(pattern[i+1:], ']')
			if end < 0 {
				return true
			}
			i += end + 1
		case '{':
			depth++
		case '}':
			if depth == 0 {
				return true
			}
			depth--
		}
	}
	return depth != 0
}

// Match reports whether path is covered by the glob.
func (g Matcher) Match(path string) bool { return g.re != nil && g.re.MatchString(path) }

func globBody(p string) string {
	var sb strings.Builder
	if !strings.Contains(p, "/") {
		sb.WriteString("(?:.*/)?")
	}
	for i := 0; i < len(p); i++ {
		switch c := p[i]; c {
		case '*':
			if i+1 < len(p) && p[i+1] == '*' {
				i++
				if i+1 < len(p) && p[i+1] == '/' {
					i++
					sb.WriteString("(?:.*/)?")
				} else {
					sb.WriteString(".*")
				}
			} else {
				sb.WriteString("[^/]*")
			}
		case '?':
			sb.WriteString("[^/]")
		case '[':
			end := strings.IndexByte(p[i:], ']')
			if end < 2 {
				sb.WriteString(`\[`)
				continue
			}
			class := p[i+1 : i+end]
			if strings.HasPrefix(class, "!") {
				class = "^" + class[1:]
			}
			sb.WriteString("[" + strings.ReplaceAll(class, `\`, `\\`) + "]")
			i += end
		default:
			sb.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	sb.WriteString("(?:/.*)?")
	return sb.String()
}

// ExpandBraces expands the first {a,b} group recursively.
func ExpandBraces(p string) []string {
	open := strings.IndexByte(p, '{')
	if open < 0 {
		return []string{p}
	}
	depth := 0
	closeIdx := -1
	var cuts []int
	for i := open; i < len(p); i++ {
		switch p[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				closeIdx = i
			}
		case ',':
			if depth == 1 {
				cuts = append(cuts, i)
			}
		}
		if closeIdx >= 0 {
			break
		}
	}
	if closeIdx < 0 {
		return []string{p}
	}
	prefix, suffix := p[:open], p[closeIdx+1:]
	var out []string
	start := open + 1
	for _, cut := range append(cuts, closeIdx) {
		for _, tail := range ExpandBraces(p[start:cut] + suffix) {
			out = append(out, prefix+tail)
		}
		start = cut + 1
	}
	return out
}
