package preflight

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Goldziher/ai-rulez/v5/internal/includes"
)

const redacted = "<redacted>"

// bidiAndInvisible are format characters that reorder or hide text: bidi
// embeddings, overrides and isolates, marks, zero-width characters and the BOM.
func bidiOrInvisible(r rune) bool {
	switch {
	case r >= 0x202A && r <= 0x202E, r >= 0x2066 && r <= 0x2069:
		return true
	case r == 0x200B || r == 0x200C || r == 0x200D || r == 0x200E || r == 0x200F:
		return true
	}
	return r == 0x061C || r == 0x2060 || r == 0xFEFF
}

// Sanitize makes repository-controlled text safe to print to a terminal: control
// characters (ESC, CR, LF, other C0 and C1 codes, DEL), bidi overrides and
// invisible characters are replaced with a visible escape, so a hook command
// cannot rewrite the line that announces it.
func Sanitize(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && size == 1:
			fmt.Fprintf(&b, `\x%02x`, s[i])
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f):
			fmt.Fprintf(&b, `\x%02x`, r)
		case bidiOrInvisible(r) || !unicode.IsPrint(r) && !unicode.IsSpace(r):
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			b.WriteRune(r)
		}
		i += size
	}
	return b.String()
}

var (
	secretFlagRe = regexp.MustCompile(`(?i)(--?[a-z0-9_-]*(?:token|password|passwd|secret|api-?key|apikey|auth|credential)[a-z0-9_-]*)(=|\s+)(\S+)`)
	schemeRe     = regexp.MustCompile(`(?i)\b(bearer|basic)\s+\S+`)
	authHeaderRe = regexp.MustCompile(`(?i)\bauthorization\s*[:=]\s*(?:(?:bearer|basic|token)\s+)?[^\s'"]+`)
	secretEnvRe  = regexp.MustCompile(`(?i)\b([a-z0-9_]*(?:token|password|passwd|secret|api_?key|auth|credential)[a-z0-9_]*)=(\S+)`)
	prefixedRe   = regexp.MustCompile(`\b(?:sk-|ghp_|gho_|ghs_|ghu_|github_pat_|glpat-|xox[abprs]-|AKIA)[A-Za-z0-9_-]{8,}`)
	longTokenRe  = regexp.MustCompile(`[A-Za-z0-9+/_=-]{32,}`)
	secretNameRe = regexp.MustCompile(`(?i)(token|password|passwd|secret|api_?key|auth|credential|private)`)
)

const maxSlashesInToken = 3

// looksLikeToken reports whether a long run of base64 or hex characters reads as
// a credential rather than a path: it mixes letters and digits and has few slashes.
func looksLikeToken(s string) bool {
	var letter, digit bool
	for _, r := range s {
		letter = letter || unicode.IsLetter(r)
		digit = digit || unicode.IsDigit(r)
	}
	return letter && digit && strings.Count(s, "/") <= maxSlashesInToken
}

// Redact hides the credentials a command line commonly carries: secret-named
// flags and their values, bearer or basic credentials, Authorization headers,
// NAME=value pairs with a secret-looking name, URL user information and query
// values, well-known token prefixes and long base64 or hex runs.
func Redact(s string) string {
	s = includes.RedactURL(s)
	s = authHeaderRe.ReplaceAllString(s, "Authorization: "+redacted)
	s = schemeRe.ReplaceAllString(s, "${1} "+redacted)
	s = secretFlagRe.ReplaceAllString(s, "${1}${2}"+redacted)
	s = secretEnvRe.ReplaceAllString(s, "${1}="+redacted)
	s = prefixedRe.ReplaceAllString(s, redacted)
	return longTokenRe.ReplaceAllStringFunc(s, func(m string) string {
		if looksLikeToken(m) {
			return redacted
		}
		return m
	})
}

// EnvValue renders one environment variable: the value is shown unless the name
// or the value looks like a credential.
func EnvValue(name, value string) string {
	if secretNameRe.MatchString(name) {
		return name + "=" + redacted
	}
	return name + "=" + Redact(value)
}

// Display is the form of repository-controlled text that is safe to print:
// escaped first, so an escape sequence cannot hide an argument from the
// redaction, then redacted, then shortened to limit characters.
func Display(s string, limit int) string {
	s = Redact(Sanitize(s))
	if limit > 0 && utf8.RuneCountInString(s) > limit {
		s = string([]rune(s)[:limit]) + "..."
	}
	return s
}
