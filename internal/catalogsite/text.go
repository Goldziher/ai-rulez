package catalogsite

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Display limits: a long name or body is cut on the page; the full text stays
// in catalog.json (up to the builder's own limits).
const (
	maxDisplayBytes = 4096
	truncatedMark   = " [truncated]"
)

// hiddenRune reports whether r is invisible or reorders text: format controls
// (bidi overrides, zero-width characters, BOM), line and paragraph separators
// and control characters other than tab and newline.
func hiddenRune(r rune) bool {
	if r == '\t' || r == '\n' {
		return false
	}
	return unicode.In(r, unicode.Cf, unicode.Cc, unicode.Zl, unicode.Zp, unicode.Co)
}

// hasHidden reports whether s contains a hidden character or invalid UTF-8.
func hasHidden(s string) bool {
	if !utf8.ValidString(s) {
		return true
	}
	return strings.IndexFunc(s, hiddenRune) >= 0
}

// display makes source text safe to show: invalid UTF-8 and hidden characters
// become U+FFFD (so a reader sees them), and over-long text is cut. HTML
// escaping is not done here; html/template does that where the text lands.
func display(s string) string {
	truncated := false
	if len(s) > maxDisplayBytes {
		cut := maxDisplayBytes
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
		s, truncated = s[:cut], true
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if hiddenRune(r) {
			r = utf8.RuneError
		}
		b.WriteRune(r)
	}
	if truncated {
		b.WriteString(truncatedMark)
	}
	return b.String()
}
