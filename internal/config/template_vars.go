package config

import (
	"regexp"
	"strings"
)

// templateVarRe matches a {{name}} placeholder in author-supplied generated text.
var templateVarRe = regexp.MustCompile(`\{\{\s*([^{}]*?)\s*\}\}`)

// unknownTemplateVars lists the placeholders of text that are not among allowed,
// in order of appearance and without repeats, so a typo is reported instead of
// being written into a generated file.
func unknownTemplateVars(text string, allowed ...string) []string {
	var unknown []string
	seen := map[string]bool{}
	for _, m := range templateVarRe.FindAllStringSubmatch(text, -1) {
		name := m[1]
		if seen[name] {
			continue
		}
		seen[name] = true
		known := false
		for _, a := range allowed {
			known = known || a == name
		}
		if !known {
			unknown = append(unknown, name)
		}
	}
	return unknown
}

// expandTemplateVars replaces each {{name}} placeholder of text with vars[name].
func expandTemplateVars(text string, vars map[string]string) string {
	return templateVarRe.ReplaceAllStringFunc(text, func(match string) string {
		name := strings.TrimSpace(strings.Trim(match, "{}"))
		if v, ok := vars[name]; ok {
			return v
		}
		return match
	})
}
