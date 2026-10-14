package judge

import "strings"

// minSecretValueLen is the shortest configured secret value that is screened for:
// a shorter one would match ordinary words and mask half of every file.
const minSecretValueLen = 8

// maskSecretValues replaces every occurrence of a configured secret value (the
// API key itself, not its name) in s with a mask. The value is never reported.
func maskSecretValues(s string, secrets []string) string {
	for _, v := range secrets {
		if len(v) >= minSecretValueLen {
			s = strings.ReplaceAll(s, v, redactMask)
		}
	}
	return s
}

const redactMask = "[REDACTED]"

// SafePath is a path made fit to print and send: secret values and secret-looking
// text are masked and control characters become spaces.
func SafePath(path string, secrets []string) string {
	return cleanText(maskSecretValues(path, secrets))
}

// SafeText makes text fit to print: secret-looking text is masked and control
// characters (terminal escapes, line breaks) become spaces.
func SafeText(s string) string { return cleanText(s) }
