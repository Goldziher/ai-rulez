package evals

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/Goldziher/ai-rulez/v5/internal/jsondoc"
)

// Output formats of `eval run`.
const (
	FormatText     = "text"
	FormatJSON     = "json"
	FormatMarkdown = "markdown"
	FormatJUnit    = "junit"
)

// Write renders a run report in a format.
func (r *RunReport) Write(w io.Writer, format string) error {
	switch format {
	case FormatText:
		return r.writeText(w)
	case FormatJSON:
		return jsondoc.Write(w, r)
	case FormatMarkdown:
		return r.writeMarkdown(w)
	case FormatJUnit:
		return r.writeJUnit(w)
	}
	return fmt.Errorf("unknown format %q (use text, json, markdown or junit)", format)
}

// Extension is the file extension of a format.
func Extension(format string) string {
	switch format {
	case FormatText:
		return "txt"
	case FormatMarkdown:
		return "md"
	case FormatJUnit:
		return "xml"
	}
	return "json"
}

// writeText renders the run as plain text: a header, one aligned line per
// skill and the problems under it.
func (r *RunReport) writeText(w io.Writer) error {
	var b strings.Builder
	fmt.Fprintf(&b, "Skill eval results: runner %#q, harness %#q, model %#q", r.Runner, r.Harness, r.Model)
	if r.Date != "" {
		fmt.Fprintf(&b, ", date %s", r.Date)
	}
	if r.Ablation {
		b.WriteString(", ablation on")
	}
	b.WriteString("\n")
	if r.DryRun {
		e := r.Estimate
		fmt.Fprintf(&b, "Dry run: %d agent runs, tokens in %s, out %s, estimated cost $%.2f (range $%.2f to $%.2f; a rough estimate).\n",
			e.AgentRuns, tokenRange(e.InputTokensLow, e.InputTokens, e.InputTokensHigh), tokenRange(e.OutputTokensLow, e.OutputTokens, e.OutputTokensHigh),
			e.CostUSD, e.CostLowUSD, e.CostHighUSD)
		if r.PricedAs != "" {
			fmt.Fprintf(&b, "No model was set, so the estimate is priced as %s; pass --model (or --price-in and --price-out) for the model you will run.\n", r.PricedAs)
		}
		if !r.PriceKnown {
			b.WriteString("The model has no built-in price: the estimate uses the sonnet tier; pass --price-in and --price-out for a real figure.\n")
		}
	}
	b.WriteString("\n")
	tw := tabwriter.NewWriter(&b, 0, 4, 2, ' ', 0)
	row := func(format string, args ...any) {
		_, _ = fmt.Fprintf(tw, format, args...) //nolint:errcheck // the tabwriter writes to a strings.Builder, which cannot fail
	}
	row("SKILL\tSTATUS\tCASES\tPASS\tPRECISION\tRECALL\tABLATION\tTOKENS\tCOST\n")
	for i := range r.Skills {
		s := &r.Skills[i]
		if s.Status == RunNoCases || s.Status == RunNotChanged {
			continue
		}
		if s.Score == nil {
			cost := "-"
			if s.Estimate != nil {
				cost = fmt.Sprintf("~$%.2f", s.Estimate.CostUSD)
			}
			row("%s\t%s\t%d\t-\t-\t-\t-\t-\t%s\n", s.ID, s.Status, s.CaseCount, cost)
			continue
		}
		sc := s.Score
		row("%s\t%s\t%d\t%s\t%s\t%s\t%s\t%d\t$%.2f\n", s.ID, statusLabel(s), sc.Scored,
			Percent(&sc.PassRate), Percent(sc.TriggerPrecision), Percent(sc.TriggerRecall), deltaText(sc.AblationDelta), sc.SkillTokens, sc.CostUSD)
	}
	if err := tw.Flush(); err != nil {
		return err //nolint:wrapcheck // a strings.Builder does not fail
	}
	for i := range r.Skills {
		if lines := problemLines(&r.Skills[i]); len(lines) > 0 {
			fmt.Fprintf(&b, "\n%s\n%s\n", r.Skills[i].ID, strings.Join(lines, "\n"))
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func (r *RunReport) writeMarkdown(w io.Writer) error {
	var b strings.Builder
	b.WriteString("# Skill eval results\n\n")
	fmt.Fprintf(&b, "Runner %#q, harness %#q, model %#q", r.Runner, r.Harness, r.Model)
	if r.Date != "" {
		fmt.Fprintf(&b, ", date %s", r.Date)
	}
	if r.Ablation {
		b.WriteString(", ablation on")
	}
	b.WriteString("\n\n")
	if r.DryRun {
		e := r.Estimate
		fmt.Fprintf(&b, "Dry run: %d agent runs, tokens in %s, out %s, estimated cost **$%.2f** (range $%.2f to $%.2f; a rough estimate).\n\n",
			e.AgentRuns, tokenRange(e.InputTokensLow, e.InputTokens, e.InputTokensHigh), tokenRange(e.OutputTokensLow, e.OutputTokens, e.OutputTokensHigh),
			e.CostUSD, e.CostLowUSD, e.CostHighUSD)
		if r.PricedAs != "" {
			fmt.Fprintf(&b, "No model was set, so the estimate is priced as %s. Pass `--model` (or `--price-in` and `--price-out`) for the model you will run.\n\n", r.PricedAs)
		}
		if !r.PriceKnown {
			b.WriteString("The model has no built-in price: the estimate uses the sonnet tier. Pass `--price-in` and `--price-out` for a real figure.\n\n")
		}
	}
	b.WriteString("| Skill | Status | Cases | Pass rate | Trigger precision | Trigger recall | Ablation delta | Skill tokens | Cost |\n")
	b.WriteString("| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for i := range r.Skills {
		if row := markdownRow(&r.Skills[i]); row != "" {
			b.WriteString(row)
		}
	}
	for i := range r.Skills {
		if lines := problemLines(&r.Skills[i]); len(lines) > 0 {
			fmt.Fprintf(&b, "\n### %s\n\n%s\n", r.Skills[i].ID, strings.Join(lines, "\n"))
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// tokenRange renders low, expected and high as "low / expected / high".
func tokenRange(low, expected, high int) string {
	return fmt.Sprintf("%d / %d / %d", low, expected, high)
}

func markdownRow(s *SkillRun) string {
	if s.Status == RunNoCases || s.Status == RunNotChanged {
		return ""
	}
	if s.Score == nil {
		cost := "-"
		if s.Estimate != nil {
			cost = fmt.Sprintf("~$%.2f", s.Estimate.CostUSD)
		}
		return fmt.Sprintf("| %s | %s | %d | - | - | - | - | - | %s |\n", s.ID, s.Status, s.CaseCount, cost)
	}
	sc := s.Score
	return fmt.Sprintf("| %s | %s | %d | %s | %s | %s | %s | %d | $%.2f |\n", s.ID, statusLabel(s), sc.Scored,
		Percent(&sc.PassRate), Percent(sc.TriggerPrecision), Percent(sc.TriggerRecall), deltaText(sc.AblationDelta), sc.SkillTokens, sc.CostUSD)
}

// problemLines lists what went wrong in a skill: invalid files, run errors and
// failing cases.
func problemLines(s *SkillRun) []string {
	var lines []string
	for _, p := range s.Problems {
		lines = append(lines, "- "+p.String())
	}
	if s.Error != "" {
		lines = append(lines, "- "+s.Error)
	}
	for _, w := range s.Warnings {
		lines = append(lines, "- warning: "+w)
	}
	for i := range s.Cases {
		c := &s.Cases[i]
		if c.Status != StatusFailed && c.Status != StatusError {
			continue
		}
		detail := c.Reason
		if len(c.Failures) > 0 {
			detail = strings.Join(c.Failures, "; ")
		}
		lines = append(lines, fmt.Sprintf("- %#q: %s", c.Case, detail))
	}
	return lines
}

func statusLabel(s *SkillRun) string {
	if s.Status == RunRan || s.Status == RunCached {
		if s.Passing {
			return s.Status + ", passing"
		}
		return s.Status + ", failing"
	}
	return s.Status
}

func deltaText(v *float64) string {
	if v == nil {
		return textNA
	}
	return fmt.Sprintf("%+.0f pts", *v*100)
}

// JUnit XML. Times are omitted (always 0) so the file is byte-stable.
type junitSuites struct {
	XMLName  xml.Name     `xml:"testsuites"`
	Name     string       `xml:"name,attr"`
	Tests    int          `xml:"tests,attr"`
	Failures int          `xml:"failures,attr"`
	Errors   int          `xml:"errors,attr"`
	Skipped  int          `xml:"skipped,attr"`
	Suites   []junitSuite `xml:"testsuite"`
}

type junitSuite struct {
	Name       string          `xml:"name,attr"`
	Tests      int             `xml:"tests,attr"`
	Failures   int             `xml:"failures,attr"`
	Errors     int             `xml:"errors,attr"`
	Skipped    int             `xml:"skipped,attr"`
	Properties []junitProperty `xml:"properties>property,omitempty"`
	Cases      []junitCase     `xml:"testcase"`
}

type junitProperty struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

type junitCase struct {
	Name      string        `xml:"name,attr"`
	Classname string        `xml:"classname,attr"`
	Failure   *junitProblem `xml:"failure,omitempty"`
	Error     *junitProblem `xml:"error,omitempty"`
	Skipped   *junitSkipped `xml:"skipped,omitempty"`
}

type junitProblem struct {
	Message string `xml:"message,attr"`
	Text    string `xml:",chardata"`
}

type junitSkipped struct {
	Message string `xml:"message,attr,omitempty"`
}

func (r *RunReport) writeJUnit(w io.Writer) error {
	doc := junitSuites{Name: "ai-rulez eval"}
	for i := range r.Skills {
		s := &r.Skills[i]
		if s.Status == RunNoCases || s.Status == RunNotChanged {
			continue
		}
		suite := junitSuiteFor(s)
		doc.Tests += suite.Tests
		doc.Failures += suite.Failures
		doc.Errors += suite.Errors
		doc.Skipped += suite.Skipped
		doc.Suites = append(doc.Suites, suite)
	}
	out, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("encode junit: %w", err)
	}
	_, err = io.WriteString(w, xml.Header+string(out)+"\n")
	return err
}

const junitRunCase = "run"

// junitSuiteFor renders one skill as a test suite.
func junitSuiteFor(s *SkillRun) junitSuite {
	suite := junitSuite{Name: s.ID}
	class := "skill." + s.ID
	addProp := func(name, value string) {
		suite.Properties = append(suite.Properties, junitProperty{Name: name, Value: value})
	}
	addProp("status", s.Status)
	if s.Score != nil {
		addProp("pass_rate", fmt.Sprintf("%.4f", s.Score.PassRate))
		addProp("trigger_precision", optText(s.Score.TriggerPrecision))
		addProp("trigger_recall", optText(s.Score.TriggerRecall))
		addProp("ablation_delta", optText(s.Score.AblationDelta))
		addProp("skill_tokens", fmt.Sprint(s.Score.SkillTokens))
		addProp("cost_usd", fmt.Sprintf("%.4f", s.Score.CostUSD))
	}
	switch s.Status {
	case RunInvalid:
		for _, p := range s.Problems {
			suite.Cases = append(suite.Cases, junitCase{Name: "case-file " + p.File, Classname: class, Error: &junitProblem{Message: p.Message, Text: p.String()}})
		}
	case RunError, RunOverBudget:
		suite.Cases = append(suite.Cases, junitCase{Name: junitRunCase, Classname: class, Error: &junitProblem{Message: s.Error, Text: s.Error}})
	case RunDryRun:
		suite.Cases = append(suite.Cases, junitCase{Name: junitRunCase, Classname: class, Skipped: &junitSkipped{Message: "dry run"}})
	case RunCached:
		if s.Score != nil && !s.Passing {
			suite.Cases = append(suite.Cases, junitCase{Name: "pass rate", Classname: class, Failure: &junitProblem{
				Message: fmt.Sprintf("cached pass rate %s is below the threshold", Percent(&s.Score.PassRate))}})
		} else {
			suite.Cases = append(suite.Cases, junitCase{Name: junitRunCase, Classname: class, Skipped: &junitSkipped{Message: "cached result, skill and cases unchanged"}})
		}
	}
	for i := range s.Cases {
		suite.Cases = append(suite.Cases, junitCaseFor(&s.Cases[i], class))
	}
	for i := range suite.Cases {
		c := &suite.Cases[i]
		suite.Tests++
		switch {
		case c.Failure != nil:
			suite.Failures++
		case c.Error != nil:
			suite.Errors++
		case c.Skipped != nil:
			suite.Skipped++
		}
	}
	return suite
}

func junitCaseFor(c *CaseScore, class string) junitCase {
	tc := junitCase{Name: c.Case, Classname: class}
	detail := strings.Join(c.Failures, "; ")
	switch c.Status {
	case StatusFailed:
		tc.Failure = &junitProblem{Message: detail, Text: detail}
	case StatusError:
		tc.Error = &junitProblem{Message: c.Reason, Text: c.Reason}
	case StatusSkipped:
		tc.Skipped = &junitSkipped{Message: c.Reason}
	}
	return tc
}

func optText(v *float64) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%.4f", *v)
}
