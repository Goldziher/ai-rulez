package lint

import (
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/secretpat"
)

// ScanText applies the security rules (AR0xx) with default settings to one text
// that is not part of a loaded project, for example a file staged by `convert`.
// Inline ai-rulez-lint-ignore comments in the text are not honored: the text
// is not trusted to silence its own findings.
func ScanText(file, text string, opts ...Option) []Finding {
	r := &runner{cfg: &config.Config{}, docs: map[string]doc{file: {}}}
	for _, opt := range opts {
		opt(r)
	}
	r.resolveSettings()
	r.securityScan(file, text)
	return securityOnly(r.findings)
}

// DetectSecret reports whether s contains a credential the security scan
// recognizes (cloud keys, tokens, private keys, JWTs and key=value
// assignments with a long mixed value) and returns the pattern name.
func DetectSecret(s string) (string, bool) {
	for _, p := range builtinSecrets {
		if p.mayMatch(s) && p.re.MatchString(s) {
			return p.name, true
		}
	}
	if !secretpat.MayHaveGenericCredential(s) {
		return "", false
	}
	for _, m := range genericCredential.FindAllStringSubmatch(s, -1) {
		if hasLetterAndDigit(m[1]) {
			return "hard-coded credential", true
		}
	}
	return "", false
}
