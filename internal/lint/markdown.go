package lint

import (
	"regexp"
	"strconv"
	"strings"
)

// doc is one source file split into lines, with the frontmatter located.
type doc struct {
	lines []string
	// bodyStart is the 0-based index of the first line after the frontmatter.
	bodyStart int
}

func parseDoc(raw string) doc {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	d := doc{lines: strings.Split(raw, "\n")}
	if len(d.lines) > 0 && strings.TrimSpace(d.lines[0]) == "---" {
		for i := 1; i < len(d.lines); i++ {
			if strings.TrimSpace(d.lines[i]) == "---" {
				d.bodyStart = i + 1
				break
			}
		}
	}
	return d
}

// lineOf returns the 1-based line in the frontmatter that contains needle, or
// fallback when it is not found there.
func (d doc) lineOf(needle string, fallback int) int {
	for i := 1; i < d.bodyStart && i < len(d.lines); i++ {
		if strings.Contains(d.lines[i], needle) {
			return i + 1
		}
	}
	return fallback
}

// bodyLine is one prose line: outside the frontmatter and outside fenced code.
type bodyLine struct {
	No int // 1-based
	// Text is the line as written; Plain is Text with inline code spans blanked
	// out (same length) so link and name patterns do not fire inside code.
	Text, Plain string
}

var (
	fenceRe    = regexp.MustCompile("^\\s*(```|~~~)")
	codeSpanRe = regexp.MustCompile("`[^`\n]*`")
	headingRe  = regexp.MustCompile(`^\s{0,3}(#{1,6})\s+(.*?)\s*#*\s*$`)
)

func (d doc) body() []bodyLine {
	var out []bodyLine
	fenced := false
	for i := d.bodyStart; i < len(d.lines); i++ {
		line := d.lines[i]
		if fenceRe.MatchString(line) {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		plain := codeSpanRe.ReplaceAllStringFunc(line, func(s string) string { return strings.Repeat(" ", len(s)) })
		out = append(out, bodyLine{No: i + 1, Text: line, Plain: plain})
	}
	return out
}

// headingSlugs returns the GitHub-style anchors of every heading in raw.
func headingSlugs(raw string) map[string]struct{} {
	slugs := map[string]struct{}{}
	seen := map[string]int{}
	d := parseDoc(raw)
	for _, l := range d.body() {
		m := headingRe.FindStringSubmatch(l.Text)
		if m == nil {
			continue
		}
		slug := slugify(m[2])
		if n := seen[slug]; n > 0 {
			slugs[slug+"-"+strconv.Itoa(n)] = struct{}{}
		} else {
			slugs[slug] = struct{}{}
		}
		seen[slug]++
	}
	return slugs
}

var slugDropRe = regexp.MustCompile(`[^\p{L}\p{N}\s_-]`)

func slugify(heading string) string {
	s := strings.ToLower(strings.TrimSpace(heading))
	s = slugDropRe.ReplaceAllString(s, "")
	return strings.ReplaceAll(s, " ", "-")
}

var (
	inlineLinkRe = regexp.MustCompile(`!?\[[^\]\n]*\]\(\s*(<[^>\n]+>|[^)\s]+)(?:\s+"[^"\n]*")?\s*\)`)
	refLinkRe    = regexp.MustCompile(`^\s{0,3}\[[^\]\n]+\]:\s*(\S+)`)
	backtickRe   = regexp.MustCompile("`([^`\n]+)`")
	schemeRe     = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)
	lineSuffixRe = regexp.MustCompile(`:\d+(?:[-:]\d+)*$`)
	placeholder  = regexp.MustCompile(`[<>{}$*?\[\]|\\\s"'=;&()]|\.{3}|\bXXX\b|\bYOUR_`)
)

// linkTargets lists the link destinations on a prose line.
func linkTargets(plain string) []string {
	var out []string
	for _, m := range inlineLinkRe.FindAllStringSubmatch(plain, -1) {
		out = append(out, strings.Trim(m[1], "<>"))
	}
	if m := refLinkRe.FindStringSubmatch(plain); m != nil {
		out = append(out, strings.Trim(m[1], "<>"))
	}
	return out
}

// ignoreDirective extracts the codes from an `ai-rulez-lint-ignore` comment.
// An empty, non-nil result means "ignore everything on this line".
func ignoreDirective(line string) ([]string, bool) {
	_, after, ok := strings.Cut(line, "ai-rulez-lint-ignore")
	if !ok {
		return nil, false
	}
	after = strings.TrimLeft(after, ": ")
	if i := strings.Index(after, "-->"); i >= 0 {
		after = after[:i]
	}
	codes := append([]string{}, strings.FieldsFunc(after, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' })...)
	return codes, true
}
