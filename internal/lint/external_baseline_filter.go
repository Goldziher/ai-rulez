package lint

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// baselineMessageLimit caps the message stored on a scanner baseline entry. The
// message only describes the finding to a reviewer (the fingerprint is what
// matches), so the first line is enough and a 1,000-entry baseline stays small.
const baselineMessageLimit = 160

// BaselineFilter selects which scanner findings --write-baseline records.
type BaselineFilter struct {
	// MinSeverity is the lowest severity recorded; empty records every severity.
	MinSeverity Severity
	// Rules are globs (`*` matches any run of characters) over the scanner rule
	// id, `scanner:rule` and the AR code; empty records every rule.
	Rules []string
}

// ParseBaselineFilter validates the --min-severity and --rule values.
func ParseBaselineFilter(minSeverity string, rules []string) (BaselineFilter, error) {
	var f BaselineFilter
	if strings.TrimSpace(minSeverity) != "" {
		sev, ok := ParseSeverity(minSeverity)
		if !ok || sev == SeverityOff {
			return f, fmt.Errorf("--min-severity %q: expecting info, warning or error", minSeverity)
		}
		f.MinSeverity = sev
	}
	for _, r := range rules {
		r = strings.TrimSpace(r)
		if r == "" {
			return f, fmt.Errorf("--rule needs a rule id or glob")
		}
		f.Rules = append(f.Rules, r)
	}
	return f, nil
}

// active reports whether the filter narrows anything.
func (f BaselineFilter) active() bool { return f.MinSeverity != "" || len(f.Rules) > 0 }

// matches reports whether sf is recorded under the filter.
func (f BaselineFilter) matches(sf *scannerFinding) bool {
	if f.MinSeverity != "" && !sf.Severity.AtLeast(f.MinSeverity) {
		return false
	}
	if len(f.Rules) == 0 {
		return true
	}
	return anyGlob(f.Rules, sf.rule, sf.scanner+":"+sf.rule, sf.Code)
}

// anyGlob reports whether any pattern matches any candidate.
func anyGlob(patterns []string, candidates ...string) bool {
	for _, p := range patterns {
		re := globRegexp(p)
		for _, c := range candidates {
			if c != "" && re.MatchString(c) {
				return true
			}
		}
	}
	return false
}

func globRegexp(glob string) *regexp.Regexp {
	parts := strings.Split(glob, "*")
	for i := range parts {
		parts[i] = regexp.QuoteMeta(parts[i])
	}
	return regexp.MustCompile("(?i)^" + strings.Join(parts, ".*") + "$")
}

// ParseReasonFor splits repeated RULE=text values into a map.
func ParseReasonFor(values []string) (map[string]string, error) {
	out := map[string]string{}
	for _, v := range values {
		rule, text, ok := strings.Cut(v, "=")
		rule, text = strings.TrimSpace(rule), strings.TrimSpace(text)
		if !ok || rule == "" || text == "" {
			return nil, fmt.Errorf("--reason-for %q: expecting RULE=text", v)
		}
		out[rule] = text
	}
	return out, nil
}

// reasonFor is the reason of a new entry: the --reason-for text of the first
// rule (exact id, `scanner:rule`, AR code or glob; the longest pattern wins)
// that matches, else the --reason template. {rule}, {severity} and {scanner} are
// expanded.
func (o ScannerOptions) reasonFor(sf *scannerFinding) string {
	best, bestLen := o.Reason, -1
	for pattern, text := range o.ReasonFor {
		if len(pattern) > bestLen && anyGlob([]string{pattern}, sf.rule, sf.scanner+":"+sf.rule, sf.Code) {
			best, bestLen = text, len(pattern)
		}
	}
	return strings.NewReplacer("{rule}", sf.rule, "{severity}", string(sf.Severity), "{scanner}", sf.scanner).Replace(best)
}

// shortBaselineMessage keeps the first line of a message, cut to the limit.
func shortBaselineMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		msg = strings.TrimSpace(msg[:i])
	}
	if utf8.RuneCountInString(msg) <= baselineMessageLimit {
		return msg
	}
	runes := []rune(msg)
	return strings.TrimSpace(string(runes[:baselineMessageLimit-1])) + "…"
}

// split divides the findings (index-aligned with all) into the ones the filter
// records and the fingerprints of the ones it leaves out.
func (f BaselineFilter) split(all []scannerFinding, findings []Finding) (recorded []Finding, skipped map[string]bool) {
	if !f.active() {
		return findings, nil
	}
	recorded = make([]Finding, 0, len(findings))
	skipped = map[string]bool{}
	for i := range all {
		if f.matches(&all[i]) {
			recorded = append(recorded, findings[i])
		} else {
			skipped[all[i].Fingerprint()] = true
		}
	}
	return recorded, skipped
}

// describeEntries names the scanner and rule of every entry and, for the ones
// that are new (not in old), shortens the message and sets the reason.
func (o ScannerOptions) describeEntries(next *Baseline, old map[string]BaselineEntry, scanners map[string]scannerFinding) {
	for i := range next.Entries {
		e := &next.Entries[i]
		sf, ok := scanners[e.Fingerprint]
		if !ok {
			continue
		}
		e.Scanner, e.Rule = sf.scanner, sf.rule
		if _, existing := old[e.Fingerprint]; !existing {
			e.Message = shortBaselineMessage(e.Message)
			e.Reason = strings.TrimSpace(o.reasonFor(&sf))
		}
	}
}
