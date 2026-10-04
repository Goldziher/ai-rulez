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
}

// Combined is the JSON document `validate --strict --format json` prints.
type Combined struct {
	Roots    []string  `json:"roots"`
	Findings []Finding `json:"findings"`
	Summary  Summary   `json:"summary"`
}

// Combine merges per-root reports into one document.
func Combine(reports []*Report) Combined {
	c := Combined{Roots: []string{}, Findings: []Finding{}, Summary: Summary{ByCode: map[string]int{}}}
	for _, r := range reports {
		c.Roots = append(c.Roots, r.Root)
		c.Findings = append(c.Findings, r.Findings...)
	}
	for _, f := range c.Findings {
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
	for _, f := range c.Findings {
		fmt.Fprintf(&sb, "%s:%d: %s %s %s: %s\n", f.File, f.Line, f.Severity, f.Code, f.Name, f.Message)
	}
	if c.Summary.Total == 0 {
		fmt.Fprintf(&sb, "strict validation: no findings in %d root(s)\n", len(c.Roots))
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
	_, err := io.WriteString(w, sb.String())
	return err //nolint:wrapcheck // writer error
}
