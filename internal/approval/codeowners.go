package approval

import (
	"regexp"
	"strings"
)

// Codeowners is a parsed CODEOWNERS file. Matching follows the documented
// GitHub rules (gitignore-like, the last matching line wins); it is checked
// against the table in codeowners_test.go, not against the forge itself.
type Codeowners struct {
	rules []ownerRule
	// Skipped lists the 1-based lines that were ignored because GitHub does not
	// support their syntax (negation with "!", character classes with "[").
	Skipped []int
}

type ownerRule struct {
	pattern string
	owners  []string
	re      *regexp.Regexp
}

// ParseCodeowners reads a CODEOWNERS file. Blank lines and comments are skipped;
// a line with a pattern and no owners leaves the path unowned. A line that uses
// syntax CODEOWNERS does not support is skipped and listed in Skipped, as the
// forge ignores it.
func ParseCodeowners(data []byte) *Codeowners {
	co := &Codeowners{}
	for i, raw := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		fields := splitOwnerLine(raw)
		if len(fields) == 0 {
			continue
		}
		pattern := fields[0]
		if strings.HasPrefix(pattern, "!") || strings.ContainsAny(pattern, "[]") {
			co.Skipped = append(co.Skipped, i+1)
			continue
		}
		re, err := compileOwnerPattern(pattern)
		if err != nil {
			co.Skipped = append(co.Skipped, i+1)
			continue
		}
		co.rules = append(co.rules, ownerRule{pattern: pattern, owners: fields[1:], re: re})
	}
	return co
}

// splitOwnerLine splits a CODEOWNERS line into its pattern and owners: fields
// are separated by whitespace, "\ " keeps a space inside the pattern, and an
// unescaped "#" starts a comment. "\#" at the start of the pattern is a literal "#".
func splitOwnerLine(line string) []string {
	var fields []string
	var cur strings.Builder
	have := false
	flush := func() {
		if have {
			fields = append(fields, cur.String())
			cur.Reset()
			have = false
		}
	}
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case c == '\\' && i+1 < len(line) && (line[i+1] == ' ' || line[i+1] == '#'):
			cur.WriteByte(line[i+1])
			have = true
			i++
		case c == '#':
			flush()
			return fields
		case c == ' ' || c == '\t':
			flush()
		default:
			cur.WriteByte(c)
			have = true
		}
	}
	flush()
	return fields
}

// compileOwnerPattern turns a CODEOWNERS pattern into an anchored regexp over a
// slash-separated repository path. A pattern without a slash (besides a trailing
// one) matches at any depth; a leading slash anchors it to the root. A pattern
// whose last segment names a directory (a trailing slash, "/**", or no wildcard)
// also owns everything below; "docs/*" owns the files directly in docs only.
func compileOwnerPattern(p string) (*regexp.Regexp, error) {
	dirOnly := strings.HasSuffix(p, "/")
	p = strings.TrimSuffix(p, "/")
	anchored := strings.HasPrefix(p, "/") || strings.Contains(p, "/")
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return nil, errEmptyPattern
	}
	var b strings.Builder
	b.WriteString("^")
	if !anchored {
		b.WriteString("(?:.*/)?")
	}
	segs := strings.Split(p, "/")
	last := segs[len(segs)-1]
	for i, seg := range segs {
		switch {
		case seg == "**" && i == len(segs)-1:
			b.WriteString(".+")
			continue
		case seg == "**":
			b.WriteString("(?:.*/)?")
			continue
		}
		b.WriteString(globSegment(seg))
		if i < len(segs)-1 {
			b.WriteString("/")
		}
	}
	if last != "**" && (dirOnly || !strings.ContainsAny(last, "*?")) {
		b.WriteString("(?:/.*)?")
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}

var errEmptyPattern = &patternError{"empty pattern"}

type patternError struct{ msg string }

func (e *patternError) Error() string { return e.msg }

// globSegment converts the wildcards of one path segment: "*" is any run of
// characters but a slash, "?" one such character.
func globSegment(seg string) string {
	var b strings.Builder
	for _, r := range seg {
		switch r {
		case '*':
			b.WriteString("[^/]*")
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	return b.String()
}

// OwnersOf returns the owners of path (slash-separated, relative to the
// repository root) and whether any line matched. The last matching line wins;
// a matching line without owners returns none, with true.
func (c *Codeowners) OwnersOf(path string) (owners []string, matched bool) {
	if c == nil {
		return nil, false
	}
	path = strings.Trim(strings.ReplaceAll(path, "\\", "/"), "/")
	for i := len(c.rules) - 1; i >= 0; i-- {
		if c.rules[i].re.MatchString(path) {
			return append([]string(nil), c.rules[i].owners...), true
		}
	}
	return nil, false
}
