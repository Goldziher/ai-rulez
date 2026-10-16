package judge

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Points is what a finding of a severity takes off a category score, before confidence and decay.
func Points(s Severity) float64 {
	switch s {
	case SeverityCritical:
		return 40
	case SeverityHigh:
		return 20
	case SeverityMedium:
		return 8
	case SeverityLow:
		return 3
	}
	return 0
}

// confidenceWeight scales the points of a finding by how firmly it is established.
func confidenceWeight(c Confidence) float64 {
	switch c {
	case ConfidenceHigh:
		return 1.0
	case ConfidenceMedium:
		return 0.6
	case ConfidenceLow:
		return 0.3
	}
	return 0
}

// decay weights the n-th largest deduction of a category (counting from 0): later findings count
// less, and past the third they add nothing, so one noisy file cannot drive a score below what its
// worst problems justify.
func decay(i int) float64 {
	switch i {
	case 0:
		return 1
	case 1:
		return 0.5
	case 2:
		return 0.25
	}
	return 0
}

// CategoryScore computes 100 - sum(points x confidence weight x decay), over the findings of one
// category sorted by deduction, rounded and clamped to [0, 100]. Info findings take nothing off.
func CategoryScore(findings []*Finding) int {
	deductions := make([]float64, 0, len(findings))
	for _, f := range findings {
		if d := Points(f.Severity) * confidenceWeight(f.Confidence); d > 0 {
			deductions = append(deductions, d)
		}
	}
	sort.Sort(sort.Reverse(sort.Float64Slice(deductions)))
	total := 0.0
	for i, d := range deductions {
		total += d * decay(i)
	}
	return int(math.Max(0, math.Min(100, math.Round(100-total))))
}

// FailRule is one entry of --fail-on.
type FailRule struct {
	// Category is empty for a rule on every category (score:N).
	Category Category
	// Severity is the lowest severity that fails a unit; SeverityNone never fails on a finding.
	Severity Severity
	// MinScore, when positive, fails a unit whose category score is below it.
	MinScore int
}

// FailOn decides which units fail.
type FailOn struct {
	Rules []FailRule
}

// DefaultFailOn is security:high, governance:high, quality:none: a quality finding is advice.
func DefaultFailOn() FailOn {
	return FailOn{Rules: []FailRule{
		{Category: CategorySecurity, Severity: SeverityHigh},
		{Category: CategoryGovernance, Severity: SeverityHigh},
		{Category: CategoryQuality, Severity: SeverityNone},
	}}
}

// ParseFailOn parses "security:high,governance:high,quality:none", "score:70" and
// "quality:score:70". A category the list does not name keeps its default.
func ParseFailOn(entries []string) (FailOn, error) {
	out := DefaultFailOn()
	var extra []FailRule
	for _, raw := range entries {
		for _, e := range strings.Split(raw, ",") {
			e = strings.ToLower(strings.TrimSpace(e))
			if e == "" {
				continue
			}
			parts := strings.Split(e, ":")
			rule, err := parseFailRule(parts)
			if err != nil {
				return FailOn{}, fmt.Errorf("--fail-on %q: %w", e, err)
			}
			if rule.MinScore > 0 {
				extra = append(extra, rule)
				continue
			}
			for i := range out.Rules {
				if out.Rules[i].Category == rule.Category && out.Rules[i].MinScore == 0 {
					out.Rules[i].Severity = rule.Severity
				}
			}
		}
	}
	out.Rules = append(out.Rules, extra...)
	return out, nil
}

func parseFailRule(parts []string) (FailRule, error) {
	switch {
	case len(parts) == 2 && parts[0] == "score":
		n, err := parseScoreThreshold(parts[1])
		return FailRule{MinScore: n, Severity: SeverityNone}, err
	case len(parts) == 3 && parts[1] == "score":
		cat, ok := ParseCategory(parts[0])
		n, err := parseScoreThreshold(parts[2])
		if !ok || err != nil {
			return FailRule{}, fmt.Errorf("use <category>:score:<1-100>")
		}
		return FailRule{Category: cat, MinScore: n, Severity: SeverityNone}, nil
	case len(parts) == 2:
		return parseSeverityRule(parts[0], parts[1])
	}
	return FailRule{}, fmt.Errorf("use <category>:<severity>, <category>:none, score:<n> or <category>:score:<n>")
}

func parseScoreThreshold(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > 100 {
		return 0, fmt.Errorf("score must be a number from 1 to 100")
	}
	return n, nil
}

func parseSeverityRule(category, severity string) (FailRule, error) {
	cat, ok := ParseCategory(category)
	if !ok {
		return FailRule{}, fmt.Errorf("%q is not security, quality or governance", category)
	}
	sev := Severity(severity)
	if sev != SeverityNone && !sev.Valid() {
		return FailRule{}, fmt.Errorf("%q is not critical, high, medium, low, info or none", severity)
	}
	return FailRule{Category: cat, Severity: sev}, nil
}

// Strings renders the rules the way they are written.
func (f FailOn) Strings() []string {
	out := make([]string, 0, len(f.Rules))
	for _, r := range f.Rules {
		switch {
		case r.MinScore > 0 && r.Category == "":
			out = append(out, fmt.Sprintf("score:%d", r.MinScore))
		case r.MinScore > 0:
			out = append(out, fmt.Sprintf("%s:score:%d", r.Category, r.MinScore))
		default:
			out = append(out, fmt.Sprintf("%s:%s", r.Category, r.Severity))
		}
	}
	return out
}

// Fails reports whether a unit with these findings and scores meets a fail-on rule.
func (f FailOn) Fails(findings []*Finding, scores Scores) bool {
	for _, r := range f.Rules {
		if r.MinScore > 0 {
			for cat, s := range map[Category]*int{CategorySecurity: scores.Security, CategoryQuality: scores.Quality, CategoryGovernance: scores.Governance} {
				if s != nil && *s < r.MinScore && (r.Category == "" || r.Category == cat) {
					return true
				}
			}
			continue
		}
		if r.Severity == SeverityNone {
			continue
		}
		for _, fi := range findings {
			if fi.Category == r.Category && fi.Severity.Rank() >= r.Severity.Rank() {
				return true
			}
		}
	}
	return false
}
