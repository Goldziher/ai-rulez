package lint

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Summary aggregates findings across roots.
type Summary struct {
	Total    int            `json:"total"`
	Errors   int            `json:"errors"`
	Warnings int            `json:"warnings"`
	Infos    int            `json:"infos"`
	ByCode   map[string]int `json:"by_code"`
	// Accepted counts findings the baseline accepted; they are not in the
	// other figures.
	Accepted int `json:"accepted,omitempty"`
}

// BaselineSummary aggregates the baselines applied to the roots.
type BaselineSummary struct {
	Paths    []string        `json:"paths"`
	Accepted int             `json:"accepted"`
	Stale    []BaselineEntry `json:"stale,omitempty"`
	Expired  []BaselineEntry `json:"expired,omitempty"`
}

// Combined is the JSON document `validate --strict --format json` prints.
type Combined struct {
	Roots    []string  `json:"roots"`
	Findings []Finding `json:"findings"`
	Summary  Summary   `json:"summary"`
	// Baseline is set when a baseline was applied.
	Baseline *BaselineSummary `json:"baseline,omitempty"`
	// Budgets lists rules over their [lint.budget].
	Budgets []BudgetExcess `json:"budgets_exceeded,omitempty"`
	// Risk is the advisory risk score (never affects the exit code).
	Risk *CombinedRisk `json:"risk,omitempty"`
	// ChangedOnly is set when the report was narrowed to changed files.
	ChangedOnly *ChangedScope `json:"changed_only,omitempty"`
}

// Combine merges per-root reports into one document.
func Combine(reports []*Report) Combined {
	c := Combined{Roots: []string{}, Findings: []Finding{}, Summary: Summary{ByCode: map[string]int{}}}
	for _, r := range reports {
		c.Roots = append(c.Roots, r.Root)
		c.Findings = append(c.Findings, r.Findings...)
		if r.Scope != nil {
			if c.ChangedOnly == nil {
				c.ChangedOnly = &ChangedScope{Since: r.Scope.Since}
			}
			c.ChangedOnly.Changed += r.Scope.Changed
			c.ChangedOnly.Dependents += r.Scope.Dependents
			c.ChangedOnly.Dropped += r.Scope.Dropped
		}
		if r.Baseline != nil {
			if c.Baseline == nil {
				c.Baseline = &BaselineSummary{Paths: []string{}}
			}
			c.Baseline.Paths = append(c.Baseline.Paths, r.Baseline.Path)
			c.Baseline.Accepted += r.Baseline.Accepted
			c.Baseline.Stale = append(c.Baseline.Stale, r.Baseline.Stale...)
			c.Baseline.Expired = append(c.Baseline.Expired, r.Baseline.Expired...)
		}
	}
	c.Risk = combineRisk(reports)
	for i := range c.Findings {
		f := &c.Findings[i]
		if f.IsAccepted() {
			c.Summary.Accepted++
			continue
		}
		c.Summary.Total++
		c.Summary.ByCode[f.Code]++
		switch f.Severity {
		case SeverityError:
			c.Summary.Errors++
		case SeverityWarning:
			c.Summary.Warnings++
		case SeverityInfo:
			c.Summary.Infos++
		case SeverityOff:
		}
	}
	return c
}

// WriteJSON prints the combined document.
func WriteJSON(w io.Writer, c Combined) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(c) //nolint:wrapcheck // writer error
}

// WriteText prints one line per finding (file:line: severity code name: message)
// followed by a per-code tally.
func WriteText(w io.Writer, c Combined) error {
	var sb strings.Builder
	for i := range c.Findings {
		f := &c.Findings[i]
		if f.IsAccepted() {
			continue
		}
		fmt.Fprintf(&sb, "%s:%d: %s %s %s: %s\n", f.File, f.Line, f.Severity, f.Code, f.Name, f.Message)
	}
	if c.Summary.Total == 0 {
		fmt.Fprintf(&sb, "strict validation: no findings in %d root(s)\n", len(c.Roots))
		writeBaselineText(&sb, c)
		_, err := io.WriteString(w, sb.String())
		return err //nolint:wrapcheck // writer error
	}
	codes := make([]string, 0, len(c.Summary.ByCode))
	for code := range c.Summary.ByCode {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	fmt.Fprintf(&sb, "\nstrict validation: %d error(s), %d warning(s), %d info across %d root(s)\n", c.Summary.Errors, c.Summary.Warnings, c.Summary.Infos, len(c.Roots))
	for _, code := range codes {
		rule, _ := lookupRule(code) //nolint:errcheck // codes come from findings
		fmt.Fprintf(&sb, "  %s %-28s %d\n", code, rule.Name, c.Summary.ByCode[code])
	}
	writeRiskText(&sb, c)
	writeBaselineText(&sb, c)
	_, err := io.WriteString(w, sb.String())
	return err //nolint:wrapcheck // writer error
}

// writeBaselineText adds the baseline and budget lines of the text report.
func writeBaselineText(sb *strings.Builder, c Combined) {
	if s := c.ChangedOnly; s != nil {
		fmt.Fprintf(sb, "changed-only since %s: %d changed file(s), %d file(s) referring to them; %d finding(s) in other files not shown\n", s.Since, s.Changed, s.Dependents, s.Dropped)
	}
	for _, e := range c.Budgets {
		fmt.Fprintf(sb, "budget: %s has %d finding(s), over its budget of %d\n", e.Code, e.Count, e.Max)
	}
	if c.Baseline == nil {
		return
	}
	fmt.Fprintf(sb, "baseline: %d finding(s) accepted", c.Baseline.Accepted)
	if n := len(c.Baseline.Stale); n > 0 {
		fmt.Fprintf(sb, ", %d stale entr%s (fixed or removed; run validate --update-baseline to prune)", n, plural(n, "y", "ies"))
	}
	if n := len(c.Baseline.Expired); n > 0 {
		fmt.Fprintf(sb, ", %d expired entr%s (those findings count again)", n, plural(n, "y", "ies"))
	}
	sb.WriteString("\n")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
