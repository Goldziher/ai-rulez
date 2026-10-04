package lint

import (
	"regexp"
	"strings"
)

// globMatcher matches slash-separated repo-relative paths against one glob.
// Semantics follow the way assistants read `paths`/`globs`: ** crosses
// directories, * does not, {a,b} alternates, a pattern with no slash matches at
// any depth, and a pattern naming a directory matches everything below it.
type globMatcher struct{ re *regexp.Regexp }

func newGlob(pattern string) (globMatcher, bool) {
	p := strings.TrimSpace(pattern)
	p = strings.TrimPrefix(p, "./")
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return globMatcher{}, false
	}
	if strings.HasSuffix(p, "/") {
		p += "**"
	}
	var alts []string
	for _, expanded := range expandBraces(p) {
		alts = append(alts, globBody(expanded))
	}
	re, err := regexp.Compile("^(?:" + strings.Join(alts, "|") + ")$")
	if err != nil {
		return globMatcher{}, false
	}
	return globMatcher{re: re}, true
}

func (g globMatcher) match(path string) bool { return g.re != nil && g.re.MatchString(path) }

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

// expandBraces expands the first {a,b} group recursively.
func expandBraces(p string) []string {
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
		for _, tail := range expandBraces(p[start:cut] + suffix) {
			out = append(out, prefix+tail)
		}
		start = cut + 1
	}
	return out
}
