// Package secretpat holds the credential patterns shared by the security scan (lint AR001) and
// the model layer's refuse-to-send check (llm.RedactSecrets), so a key shape the scan knows is
// never sent to a model unnoticed. It has no dependencies inside ai-rulez, so both may import it.
package secretpat

import (
	"regexp"
	"strings"
)

// Pattern is one named credential shape.
type Pattern struct {
	Name string
	Re   *regexp.Regexp
	// Stems are literals every match of Re contains, compared exactly. MayMatch
	// rejects text without one, which skips the regular expression search on the
	// great majority of text. Nil means no pre-check.
	Stems []string
	// Fold makes the Stems comparison ignore ASCII case, for a pattern compiled with (?i).
	Fold bool
}

// MayMatch reports whether s holds a stem of p, so Re could match it.
func (p Pattern) MayMatch(s string) bool {
	if p.Stems == nil {
		return true
	}
	for _, st := range p.Stems {
		if p.Fold && indexFold(s, st) >= 0 || !p.Fold && strings.Contains(s, st) {
			return true
		}
	}
	return false
}

// FindValue returns the secret in the first match of re in s: the first capture group when the
// pattern has one (a context-anchored pattern captures only the value, not the name before it),
// otherwise the whole match. It is "" when re does not match.
func FindValue(re *regexp.Regexp, s string) string {
	m := re.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	if len(m) > 1 {
		return m[1]
	}
	return m[0]
}

// ReplaceValue replaces the secret of every match of re in s with mask: the first capture group
// when the pattern has one, so the name before the value and the character after it stay, and
// the whole match otherwise.
func ReplaceValue(re *regexp.Regexp, s, mask string) string {
	return ReplaceValueFunc(re, s, func(string) string { return mask })
}

// ReplaceValueFunc is ReplaceValue with the replacement computed from the secret.
func ReplaceValueFunc(re *regexp.Regexp, s string, mask func(secret string) string) string {
	if re.NumSubexp() == 0 {
		return re.ReplaceAllStringFunc(s, mask)
	}
	var b strings.Builder
	last := 0
	for _, loc := range re.FindAllStringSubmatchIndex(s, -1) {
		if loc[2] < 0 {
			continue
		}
		b.WriteString(s[last:loc[2]])
		b.WriteString(mask(s[loc[2]:loc[3]]))
		last = loc[3]
	}
	b.WriteString(s[last:])
	return b.String()
}

// Builtin is the credential shapes the security scan recognizes.
//
// The minimum lengths are deliberately strict: a detector that blocks a commit
// must not flag a placeholder, so a GitHub token needs the 36 characters real
// ones carry. The redaction layers that sit in front of a model, a log or a
// publish step (llm.RedactSecrets, publish.Redact, preflight) are deliberately
// more eager (a GitHub token from 20 characters, an sk- key from 8): hiding a
// false positive costs nothing, leaking a true one does. They therefore also run
// this list, and the two never need the same numbers.
var Builtin = []Pattern{
	{"AWS access key id", regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`), []string{"AKIA", "ASIA"}, false},
	// The AWS secret access key has no prefix of its own, so it is matched only when assigned to a
	// name containing aws_secret_access_key (any case, = or :, value optionally quoted); an
	// arbitrary 40-character string is not a finding. Group 1 is the value.
	{"AWS secret access key", regexp.MustCompile(`(?i)aws_secret_access_key["']?\s*[:=]\s*["']?([A-Za-z0-9/+]{40})(?:[^A-Za-z0-9/+]|$)`), []string{"aws_secret_access_key"}, true},
	{"GitHub token", regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{36,}\b`), []string{"ghp_", "gho_", "ghu_", "ghs_", "ghr_"}, false},
	{"npm access token", regexp.MustCompile(`\bnpm_[A-Za-z0-9]{36}\b`), []string{"npm_"}, false},
	{"GitHub fine-grained token", regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{22,}\b`), []string{"github_pat_"}, false},
	{"Slack token", regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9-]{10,}\b`), []string{"xox"}, false},
	{"Google API key", regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}\b`), []string{"AIza"}, false},
	{"Stripe live key", regexp.MustCompile(`\b[sr]k_live_[0-9A-Za-z]{24,}\b`), []string{"k_live_"}, false},
	{"Anthropic API key", regexp.MustCompile(`\bsk-ant-[A-Za-z0-9_-]{20,}\b`), []string{"sk-ant-"}, false},
	{"OpenAI API key", regexp.MustCompile(`\bsk-(?:proj-)?[A-Za-z0-9_-]{40,}\b`), []string{"sk-"}, false},
	{"private key block", regexp.MustCompile(`-{5}BEGIN (?:[A-Z]+ )*PRIVATE KEY(?: BLOCK)?-{5}`), []string{"-----BEGIN "}, false},
	{"JSON web token", regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`), []string{"eyJ"}, false},
}

// GenericCredential matches `password = "..."` style assignments; the value (group 1) must mix
// letters and digits (HasLetterAndDigit) so placeholders such as "your-key-here" pass.
var GenericCredential = regexp.MustCompile(`(?i)\b(?:api[_-]?key|secret|token|passwd|password|client[_-]?secret)\b["']?\s*[:=]\s*["']([A-Za-z0-9/+_.=-]{20,})["']`)

// MayHaveGenericCredential reports whether s holds a word GenericCredential
// starts from (compared without case, as the pattern is), so the pattern can
// be skipped for text that has none.
func MayHaveGenericCredential(s string) bool {
	for _, stem := range [...]string{"api", "secret", "token", "passwd", "password"} {
		if indexFold(s, stem) >= 0 {
			return true
		}
	}
	// The pattern's case folding also equates U+017F (long s) with s and U+212A
	// (Kelvin sign) with k, which the ASCII comparison above does not see.
	return strings.Contains(s, "\u017f") || strings.Contains(s, "\u212a")
}

// indexFold is the index of the first occurrence of the lower-case ASCII word
// stem in s, ignoring ASCII case; -1 when there is none.
func indexFold(s, stem string) int {
	lower := func(c byte) byte {
		if c >= 'A' && c <= 'Z' {
			return c + 'a' - 'A'
		}
		return c
	}
	for i := 0; i+len(stem) <= len(s); i++ {
		if lower(s[i]) != stem[0] {
			continue
		}
		j := 1
		for j < len(stem) && lower(s[i+j]) == stem[j] {
			j++
		}
		if j == len(stem) {
			return i
		}
	}
	return -1
}

// HasLetterAndDigit reports whether s holds at least one ASCII letter and one digit.
func HasLetterAndDigit(s string) bool {
	letter, digit := false, false
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9':
			digit = true
		case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'):
			letter = true
		}
	}
	return letter && digit
}

// IdentifierLike reports whether s is a snake_case identifier: three or more lower-case
// segments joined by underscores, at least two of them plain words ("saq_tn3270_scan_password").
// A configuration key name carries letters and digits like a password does, but nobody
// types a credential as a run of words.
func IdentifierLike(s string) bool {
	segments := strings.Split(s, "_")
	if len(segments) < 3 {
		return false
	}
	words := 0
	for _, seg := range segments {
		if seg == "" || seg[0] < 'a' || seg[0] > 'z' {
			return false
		}
		allLetters := true
		for i := 0; i < len(seg); i++ {
			c := seg[i]
			isLetter := c >= 'a' && c <= 'z'
			if !isLetter && (c < '0' || c > '9') {
				return false
			}
			allLetters = allLetters && isLetter
		}
		if allLetters && len(seg) >= 3 {
			words++
		}
	}
	return words >= 2
}

// IsCredentialValue reports whether the value of a GenericCredential assignment reads as a
// secret: it mixes letters and digits and is no identifier.
func IsCredentialValue(s string) bool {
	return HasLetterAndDigit(s) && !IdentifierLike(s)
}

// Redact replaces every Builtin match, and the value of every GenericCredential assignment,
// with mask.
func Redact(s, mask string) string {
	for _, p := range Builtin {
		if p.MayMatch(s) {
			s = ReplaceValue(p.Re, s, mask)
		}
	}
	if !MayHaveGenericCredential(s) {
		return s
	}
	return GenericCredential.ReplaceAllStringFunc(s, func(m string) string {
		if sub := GenericCredential.FindStringSubmatch(m); len(sub) > 1 && IsCredentialValue(sub[1]) {
			return strings.Replace(m, sub[1], mask, 1)
		}
		return m
	})
}

// GenericName is the pattern name reported for a GenericCredential hit.
const GenericName = "hard-coded credential"

// Hit is one credential found by Scan. It names the pattern and the line, never the secret.
type Hit struct {
	Name string
	// Line is the 1-based line of the hit.
	Line int
}

// Scan reports every credential the security scan (AR001) recognizes in text, line by line:
// the Builtin shapes and the GenericCredential assignments whose value reads as a secret.
// It is the single detector behind the security scan and the model gate of the judge.
func Scan(text string) []Hit {
	var hits []Hit
	for i, line := range strings.Split(text, "\n") {
		for _, name := range ScanLine(line) {
			hits = append(hits, Hit{Name: name, Line: i + 1})
		}
	}
	return hits
}

// ScanLine returns the pattern name of every credential in one line.
func ScanLine(line string) []string {
	var names []string
	for _, p := range Builtin {
		if p.MayMatch(line) && FindValue(p.Re, line) != "" {
			names = append(names, p.Name)
		}
	}
	if MayHaveGenericCredential(line) {
		for _, m := range GenericCredential.FindAllStringSubmatch(line, -1) {
			if IsCredentialValue(m[1]) {
				names = append(names, GenericName)
			}
		}
	}
	return names
}
