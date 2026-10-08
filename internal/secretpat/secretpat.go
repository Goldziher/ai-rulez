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
	{"AWS access key id", regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`)},
	{"GitHub token", regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{36,}\b`)},
	{"npm access token", regexp.MustCompile(`\bnpm_[A-Za-z0-9]{36}\b`)},
	{"GitHub fine-grained token", regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{22,}\b`)},
	{"Slack token", regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9-]{10,}\b`)},
	{"Google API key", regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}\b`)},
	{"Stripe live key", regexp.MustCompile(`\b[sr]k_live_[0-9A-Za-z]{24,}\b`)},
	{"Anthropic API key", regexp.MustCompile(`\bsk-ant-[A-Za-z0-9_-]{20,}\b`)},
	{"OpenAI API key", regexp.MustCompile(`\bsk-(?:proj-)?[A-Za-z0-9_-]{40,}\b`)},
	{"private key block", regexp.MustCompile(`-{5}BEGIN (?:[A-Z]+ )*PRIVATE KEY(?: BLOCK)?-{5}`)},
	{"JSON web token", regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`)},
}

// GenericCredential matches `password = "..."` style assignments; the value (group 1) must mix
// letters and digits (HasLetterAndDigit) so placeholders such as "your-key-here" pass.
var GenericCredential = regexp.MustCompile(`(?i)\b(?:api[_-]?key|secret|token|passwd|password|client[_-]?secret)\b["']?\s*[:=]\s*["']([A-Za-z0-9/+_.=-]{20,})["']`)

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
		s = p.Re.ReplaceAllString(s, mask)
	}
	return GenericCredential.ReplaceAllStringFunc(s, func(m string) string {
		if sub := GenericCredential.FindStringSubmatch(m); len(sub) > 1 && HasLetterAndDigit(sub[1]) {
			return strings.Replace(m, sub[1], mask, 1)
		}
		return m
	})
}
