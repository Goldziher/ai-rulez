package judge

import (
	"regexp"
	"strings"
)

// Glob matches slash paths against one Claude Code `paths` pattern: relative to the directory that
// holds the `.claude` directory, `**` crosses directories, `*` and `?` do not, `{a,b}` alternates
// (nested groups allowed), `[abc]` is a class. A pattern is anchored at that directory: `*.py`
// matches only a file directly in it, `**/*.py` matches at any depth. There is no negation: a pattern
// starting with `!` matches nothing.
type Glob struct {
	re   *regexp.Regexp
	src  string
	alts []string
}

// CompileGlob compiles a pattern; ok is false for an empty, negated or malformed one.
func CompileGlob(pattern string) (Glob, bool) {
	p := strings.TrimSpace(pattern)
	p = strings.TrimPrefix(p, "./")
	p = strings.TrimPrefix(p, "/")
	if p == "" || strings.HasPrefix(p, "!") {
		return Glob{}, false
	}
	if strings.HasSuffix(p, "/") {
		p += "**"
	}
	alts := expandGlobBraces(p)
	parts := make([]string, 0, len(alts))
	for _, a := range alts {
		parts = append(parts, globRegexp(a))
	}
	re, err := regexp.Compile("^(?:" + strings.Join(parts, "|") + ")$")
	if err != nil {
		return Glob{}, false
	}
	return Glob{re: re, src: pattern, alts: alts}, true
}

// Match reports whether the slash path (relative to the glob's base directory) matches.
func (g Glob) Match(p string) bool {
	return g.re != nil && g.re.MatchString(strings.TrimPrefix(p, "./"))
}

// MayMatchBelow reports whether some path strictly below the slash directory dir (relative to the
// glob's base directory; "" is the base directory itself) could match. It compares the pattern with
// dir one segment at a time: `**` reaches everything below, and a pattern that continues past dir
// may match deeper. A pattern that stops at dir counts only when it names the directory literally;
// a wildcard that merely matches the directory's name (`docs/*` for `docs/api`) matches the
// directory itself, not a path below it.
func (g Glob) MayMatchBelow(dir string) bool {
	if g.re == nil {
		return false
	}
	var dirSegs []string
	if dir != "" {
		dirSegs = strings.Split(dir, "/")
	}
	for _, alt := range g.alts {
		if altMayMatchBelow(strings.Split(alt, "/"), dirSegs) {
			return true
		}
	}
	return false
}

func altMayMatchBelow(pattern, dir []string) bool {
	for i, d := range dir {
		if i >= len(pattern) {
			return false
		}
		seg := pattern[i]
		if before, _, crosses := strings.Cut(seg, "**"); crosses {
			// `**` crosses slashes, so what follows it reaches anything below; only
			// the text before it has to fit the directory name.
			return segmentMatches(before+"**", d)
		}
		if !segmentMatches(seg, d) {
			return false
		}
	}
	// The pattern ran out at the directory. A path strictly below needs it to go
	// on; one that names the directory literally still counts (a directory
	// pattern may cover what it holds), a wildcard that merely matched the
	// directory's own name (`docs/*` for `docs/api`) does not.
	if len(pattern) > len(dir) {
		return true
	}
	return len(dir) > 0 && !strings.ContainsAny(pattern[len(dir)-1], "*?[")
}

func segmentMatches(seg, name string) bool {
	if !strings.ContainsAny(seg, "*?[") {
		return seg == name
	}
	re, err := regexp.Compile("^" + globRegexp(seg) + "$")
	return err != nil || re.MatchString(name)
}

// String returns the source pattern.
func (g Glob) String() string { return g.src }

func globRegexp(p string) string {
	var sb strings.Builder
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
	return sb.String()
}

// expandGlobBraces expands every {a,b} group, recursively.
func expandGlobBraces(p string) []string {
	open := strings.IndexByte(p, '{')
	if open < 0 {
		return []string{p}
	}
	depth, closeIdx := 0, -1
	var cuts []int
	for i := open; i < len(p) && closeIdx < 0; i++ {
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
	}
	if closeIdx < 0 {
		return []string{p}
	}
	prefix, suffix := p[:open], p[closeIdx+1:]
	var out []string
	start := open + 1
	for _, cut := range append(cuts, closeIdx) {
		for _, tail := range expandGlobBraces(p[start:cut] + suffix) {
			out = append(out, prefix+tail)
		}
		start = cut + 1
	}
	return out
}
