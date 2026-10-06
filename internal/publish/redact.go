package publish

import "regexp"

// redactions are the credential shapes a delegated CLI could echo into its
// output: forge and npm tokens, bearer headers, and npmrc auth lines. A match
// is replaced before the output is shown or put in an error. This is a safety
// net for tool output, not a substitute for never passing secrets as arguments.
var redactions = []*regexp.Regexp{
	regexp.MustCompile(`\b(?:gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|npm_[A-Za-z0-9]{20,})\b`),
	regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]{8,}`),
	regexp.MustCompile(`(?i)(_authToken|_auth|_password)\s*=\s*\S+`),
	regexp.MustCompile(`(?i)\b(token|password|secret|authorization)\b(\s*[:=]\s*)[^\s,;"']{8,}`),
}

// Redact hides credential-shaped text in s.
func Redact(s string) string {
	for i, re := range redactions {
		switch i {
		case 2:
			s = re.ReplaceAllString(s, "${1}=[redacted]")
		case 3:
			s = re.ReplaceAllString(s, "${1}${2}[redacted]")
		default:
			s = re.ReplaceAllString(s, "[redacted]")
		}
	}
	return s
}
