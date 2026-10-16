package judge

import (
	"encoding/json"
	"strconv"
	"strings"
	"unicode"

	"github.com/Goldziher/ai-rulez/v5/internal/llm"
)

// minSecretValueLen is the shortest configured secret value that is screened for: a shorter one
// would match ordinary words and mask half of every file.
const minSecretValueLen = 8

const redactMask = "[REDACTED]"

// maskSecretValues replaces every occurrence of a configured secret value (the API key itself, not
// its name) in s with a mask. The value is never reported.
func maskSecretValues(s string, secrets []string) string {
	for _, v := range secrets {
		if len(v) >= minSecretValueLen {
			s = strings.ReplaceAll(s, v, redactMask)
		}
	}
	return s
}

// SafePath is a path made fit to print and send: secret values and secret-looking text are masked
// and control characters become spaces.
func SafePath(p string, secrets []string) string {
	return cleanText(maskSecretValues(p, secrets))
}

// SafeText makes text fit to print: secret-looking text is masked and control characters (terminal
// escapes, line breaks) become spaces.
func SafeText(s string) string { return cleanText(s) }

func cleanText(s string) string {
	s = llm.RedactSecrets(s)
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || !unicode.IsPrint(r) && r != ' ' {
			return ' '
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

// defuseMarkers defuses fence-looking text in a file: the markers use three angle brackets, so a
// space goes into every run of '<' or '>' before it reaches three, whatever its length, and the
// file never contains a run of three.
func defuseMarkers(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	var prev rune
	run := 0
	for _, r := range s {
		if r == '<' || r == '>' {
			if r == prev {
				run++
			} else {
				run = 1
			}
			if run == 3 {
				b.WriteByte(' ')
				run = 1
			}
		} else {
			run = 0
		}
		prev = r
		b.WriteRune(r)
	}
	return b.String()
}

func itoa(n int) string { return strconv.Itoa(n) }

// NormalizeSpace collapses every run of whitespace to one space and trims the ends. It is the only
// normalization the quote verifier applies.
func NormalizeSpace(s string) string { return strings.Join(strings.Fields(s), " ") }

// jsonText encodes a plain value (maps and structs of strings and numbers). Such a value always
// encodes; the fallback only keeps a caller from handling an error that cannot happen.
func jsonText(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("null")
	}
	return b
}
