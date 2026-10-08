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
}

// MayMatch reports whether s holds a stem of p, so Re could match it.
func (p Pattern) MayMatch(s string) bool {
	if p.Stems == nil {
		return true
	}
	for _, st := range p.Stems {
		if strings.Contains(s, st) {
			return true
		}
	}
	return false
}

// Builtin is the credential shapes the security scan recognizes.
var Builtin = []Pattern{
	{"AWS access key id", regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`), []string{"AKIA", "ASIA"}},
	{"GitHub token", regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{36,}\b`), []string{"ghp_", "gho_", "ghu_", "ghs_", "ghr_"}},
	{"GitHub fine-grained token", regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{22,}\b`), []string{"github_pat_"}},
	{"Slack token", regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9-]{10,}\b`), []string{"xox"}},
	{"Google API key", regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}\b`), []string{"AIza"}},
	{"Stripe live key", regexp.MustCompile(`\b[sr]k_live_[0-9A-Za-z]{24,}\b`), []string{"k_live_"}},
	{"Anthropic API key", regexp.MustCompile(`\bsk-ant-[A-Za-z0-9_-]{20,}\b`), []string{"sk-ant-"}},
	{"OpenAI API key", regexp.MustCompile(`\bsk-(?:proj-)?[A-Za-z0-9_-]{40,}\b`), []string{"sk-"}},
	{"private key block", regexp.MustCompile(`-{5}BEGIN (?:[A-Z]+ )*PRIVATE KEY(?: BLOCK)?-{5}`), []string{"-----BEGIN "}},
	{"JSON web token", regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`), []string{"eyJ"}},
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

// Redact replaces every Builtin match, and the value of every GenericCredential assignment,
// with mask.
func Redact(s, mask string) string {
	for _, p := range Builtin {
		if p.MayMatch(s) {
			s = p.Re.ReplaceAllString(s, mask)
		}
	}
	if !MayHaveGenericCredential(s) {
		return s
	}
	return GenericCredential.ReplaceAllStringFunc(s, func(m string) string {
		if sub := GenericCredential.FindStringSubmatch(m); len(sub) > 1 && HasLetterAndDigit(sub[1]) {
			return strings.Replace(m, sub[1], mask, 1)
		}
		return m
	})
}
