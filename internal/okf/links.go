package okf

import (
	"net/url"
	"strings"
)

// SplitDest splits the destination of a markdown link into its decoded path and
// the raw `#fragment` / `?query` suffix. internal is false for external URLs,
// mailto: links and pure anchors, which are never rewritten.
func SplitDest(dest string) (p, suffix string, internal bool) {
	dest = strings.TrimSpace(dest)
	if dest == "" || strings.HasPrefix(dest, "#") || strings.Contains(dest, "://") || strings.HasPrefix(dest, "mailto:") {
		return "", "", false
	}
	p = dest
	if i := strings.IndexAny(dest, "#?"); i >= 0 {
		p, suffix = dest[:i], dest[i:]
	}
	if un, err := url.PathUnescape(p); err == nil {
		p = un
	}
	return p, suffix, p != ""
}

// JoinDest is the inverse of SplitDest: it escapes only what a link destination
// needs and appends the suffix.
func JoinDest(p, suffix string) string {
	return (&url.URL{Path: p}).EscapedPath() + suffix
}

// ResolveLink maps a link destination found in dir (a bundle-relative or
// source-relative directory, "" for the root) to a cleaned path. ok is false for
// external URLs and anchors; inside is false when the target escapes the root.
func ResolveLink(dir, dest string) (p string, ok, inside bool) {
	return resolve(dir, dest)
}

// RewriteLinks calls fn for the destination of every inline markdown link and
// image of body, outside fenced code blocks and inline code spans, and splices
// the returned destination in when fn reports true. Everything else is kept byte
// for byte, so a body without rewritten links comes back unchanged. fn also gets
// the 1-based line of the link within body.
func RewriteLinks(body string, fn func(dest string, line int) (string, bool)) string {
	var out strings.Builder
	inFence := false
	for i, line := range strings.SplitAfter(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			out.WriteString(line)
			continue
		}
		if inFence {
			out.WriteString(line)
			continue
		}
		out.WriteString(rewriteLine(line, i+1, fn))
	}
	return out.String()
}

func rewriteLine(line string, lineNo int, fn func(string, int) (string, bool)) string {
	if !strings.Contains(line, "](") {
		return line
	}
	spans := codeSpanRe.FindAllStringIndex(line, -1)
	var out strings.Builder
	last := 0
	for _, m := range linkRe.FindAllStringSubmatchIndex(line, -1) {
		start, end := m[2], m[3]
		if inSpan(spans, start, end) {
			continue
		}
		repl, ok := fn(line[start:end], lineNo)
		if !ok || repl == line[start:end] {
			continue
		}
		out.WriteString(line[last:start])
		out.WriteString(repl)
		last = end
	}
	out.WriteString(line[last:])
	return out.String()
}

func inSpan(spans [][]int, start, end int) bool {
	for _, s := range spans {
		if start < s[1] && end > s[0] {
			return true
		}
	}
	return false
}
