package improve

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
)

// ShowSchema names the JSON document `improve show --format json` prints.
const ShowSchema = "improve-show/1"

// ShowResult is a saved run as `improve show` presents it.
type ShowResult struct {
	Schema string  `json:"schema"`
	Report *Report `json:"report"`
	// Signed says whether report.json carries a valid MAC from this machine's user key. An unsigned
	// run is shown, but `improve apply` and `improve pr` refuse it.
	Signed bool `json:"signed"`
	// Diff is diff.patch of an accepted run, control characters removed.
	Diff string `json:"diff,omitempty"`
}

// Show loads a saved run for display. It reads files only and changes nothing.
func Show(configDir, runID string) (*ShowResult, error) {
	report, dir, raw, err := loadReport(configDir, runID)
	if err != nil {
		return nil, err
	}
	res := &ShowResult{Schema: ShowSchema, Report: report, Signed: verifyReport(dir, runID, raw)}
	if report.Accepted() {
		patch, err := safefs.ReadRegular(filepath.Join(dir, "diff.patch"))
		switch {
		case err == nil:
			res.Diff = SanitizeMultiline(string(patch))
		case !os.IsNotExist(err):
			return nil, fmt.Errorf("read diff.patch: %w", err)
		}
	}
	return res, nil
}

// Text renders the run for a terminal.
func (s *ShowResult) Text() string {
	var b strings.Builder
	b.WriteString(FormatReport(s.Report))
	if !s.Signed {
		b.WriteString("warning: report.json is not signed by this machine's user key (copied, edited or made elsewhere): improve apply and improve pr refuse it\n")
	}
	if s.Diff != "" {
		fmt.Fprintf(&b, "\n%s\n", s.Diff)
	}
	return b.String()
}

// SanitizeMultiline makes untrusted text safe to print: every control character except newline and
// tab (terminal escapes included) and every format character (bidi overrides, zero-width) becomes
// U+FFFD. Unlike Sanitize it keeps lines, so a diff stays a diff.
func SanitizeMultiline(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range strings.ReplaceAll(s, "\r\n", "\n") {
		switch {
		case r == '\n' || r == '\t':
			b.WriteRune(r)
		case unicode.IsControl(r) || unicode.Is(unicode.Cf, r):
			b.WriteRune('�')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// FormatReport renders a report as the terminal summary of a run.
func FormatReport(r *Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Run %s for %s: %s\n", r.RunID, r.Skill, r.Status)
	if r.Adapter != "" {
		fmt.Fprintf(&b, "Optimizer: %s\n", Sanitize(r.Adapter, 100))
	}
	if r.Baseline != nil {
		fmt.Fprintf(&b, "Baseline held-out pass rate %.0f%% (%d case(s))\n", r.Baseline.PassRate*100, r.Baseline.Scored)
	}
	for i := range r.Rounds {
		formatRound(&b, &r.Rounds[i])
	}
	if r.Reason != "" {
		fmt.Fprintf(&b, "%s\n", Sanitize(r.Reason, 300))
	}
	fmt.Fprintf(&b, "Spent $%.2f of $%.2f (evals $%.2f, optimizer-reported $%.2f)\n", r.Costs.TotalUSD, r.Costs.MaxUSD, r.Costs.EvalUSD, r.Costs.OptimizerUSD)
	if r.Costs.OptimizerReportedNoCost {
		b.WriteString("warning: the optimizer reported no cost; its own spend is bounded only by its credentials\n")
	}
	if r.Isolation != nil && r.Isolation.Confined {
		fmt.Fprintf(&b, "Isolation: %s (no_network=%t, writes confined=%t)\n", r.Isolation.Backend, r.Isolation.NoNetwork, r.Isolation.NoWrites)
	}
	fmt.Fprintf(&b, "Report: .ai-rulez/local/improve/%s/report.json\n", r.RunID)
	if r.Accepted() {
		fmt.Fprintf(&b, "Diff:   .ai-rulez/local/improve/%s/diff.patch\nReview it, then: ai-rulez improve apply %s   (or: ai-rulez improve pr %s)\n", r.RunID, r.RunID, r.RunID)
	}
	return b.String()
}

func formatRound(b *strings.Builder, rd *RoundReport) {
	fmt.Fprintf(b, "Round %d: %s", rd.Round, rd.Decision)
	if rd.Held != nil {
		fmt.Fprintf(b, " (held-out %.0f%% -> %.0f%%, %+.1f points, %d win(s), %d loss(es)", rd.Held.Base.PassRate*100, rd.Held.Cand.PassRate*100, rd.Held.Gain*100, len(rd.Held.Wins), len(rd.Held.Losses))
		if ci := rd.Held.CI; ci != nil {
			fmt.Fprintf(b, ", %.0f%% CI [%+.1f, %+.1f]", ci.Confidence*100, ci.Low*100, ci.High*100)
		}
		b.WriteString(")")
	}
	b.WriteString("\n")
	for _, v := range rd.Violations {
		fmt.Fprintf(b, "  %s\n", v.String())
	}
	for _, reason := range rd.Reasons {
		fmt.Fprintf(b, "  %s\n", Sanitize(reason, 400))
	}
	for _, warn := range rd.Warnings {
		fmt.Fprintf(b, "  warning: %s\n", Sanitize(warn, 400))
	}
	if s := rd.Siblings; s != nil && len(s.Results) > 0 {
		fmt.Fprintf(b, "  siblings checked (%s): %d, regressed: %d\n", s.Surface, len(s.Results), len(s.Regressions()))
	}
	if rd.Description != nil {
		fmt.Fprintf(b, "  description: %q -> %q\n", rd.Description.Before, rd.Description.After)
	}
}
