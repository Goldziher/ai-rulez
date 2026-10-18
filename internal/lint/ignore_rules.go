package lint

import (
	"fmt"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// ruleIgnore is a compiled [[lint.ignore_rules]] entry: the codes it names and
// the files it covers.
type ruleIgnore struct {
	codes map[string]bool
	paths []globMatcher
}

// ruleCodes resolves one `rules` entry to the codes it names: a code or name
// resolves to one code, a pattern with a wildcard to every code it matches.
func ruleCodes(pattern string) []string {
	pattern = strings.TrimSpace(pattern)
	if !strings.ContainsAny(pattern, "*?[") {
		if rule, ok := lookupRule(pattern); ok {
			return []string{rule.Code}
		}
		return nil
	}
	m, ok := newGlob(strings.ToUpper(pattern))
	if !ok {
		return nil
	}
	var out []string
	for _, rule := range ruleTables().rules {
		if m.match(rule.Code) {
			out = append(out, rule.Code)
		}
	}
	return out
}

func compileRuleIgnores(entries []config.LintIgnoreRule) []ruleIgnore {
	var out []ruleIgnore
	for _, e := range entries {
		ri := ruleIgnore{codes: map[string]bool{}, paths: compileGlobs(e.Paths)}
		for _, pat := range e.Rules {
			for _, code := range ruleCodes(pat) {
				ri.codes[code] = true
			}
		}
		if len(ri.codes) > 0 && len(ri.paths) > 0 {
			out = append(out, ri)
		}
	}
	return out
}

// ruleIgnored reports whether a [[lint.ignore_rules]] entry drops the findings
// of code in the file abs.
func (r *runner) ruleIgnored(code, abs string) bool {
	for i := range r.ignoreRules {
		if r.ignoreRules[i].codes[code] && r.pathMatches(abs, r.ignoreRules[i].paths) {
			return true
		}
	}
	return false
}

// validateIgnoreRules reports entries that would silently match nothing.
func validateIgnoreRules(entries []config.LintIgnoreRule) []string {
	var problems []string
	for i, e := range entries {
		at := fmt.Sprintf("lint.ignore_rules[%d]", i)
		if len(e.Rules) == 0 {
			problems = append(problems, at+": rules is empty; name the codes or names to drop")
		}
		for _, pat := range e.Rules {
			switch {
			case strings.TrimSpace(pat) == "":
				problems = append(problems, at+": rules has a blank entry")
			case len(ruleCodes(pat)) == 0 && strings.ContainsAny(pat, "*?["):
				problems = append(problems, fmt.Sprintf("%s: rules pattern %q matches no rule", at, pat))
			case len(ruleCodes(pat)) == 0:
				problems = append(problems, fmt.Sprintf("%s: unknown rule %q", at, pat))
			}
		}
		if len(e.Paths) == 0 {
			problems = append(problems, at+": paths is empty; name the files the ignore covers")
		}
		for _, p := range e.Paths {
			switch {
			case strings.TrimSpace(p) == "":
				problems = append(problems, at+": paths has a blank entry")
			case malformedGlob(p):
				problems = append(problems, fmt.Sprintf("%s: paths glob %q is malformed", at, p))
			}
		}
	}
	return problems
}
