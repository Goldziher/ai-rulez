package improve

import (
	"fmt"
	"strings"
)

// prBody renders the pull request description of an accepted run. Everything the optimizer or the
// candidate wrote goes into code spans or fenced blocks with control characters removed, so it cannot
// become markup, a link or a mention. The body says plainly that the change is not approved.
func prBody(r *Report, runID string) string {
	var b strings.Builder
	rd := acceptedRound(r)
	fmt.Fprintf(&b, "## Improve %s\n\n", mdCode(r.Skill))
	fmt.Fprintf(&b, "Candidate of `ai-rulez improve run` %s, round %d of at most %d, accepted by the held-out gate. **Not approved**: nothing in ai-rulez marks this change as approved; a human decides.\n\n", mdCode(runID), r.AcceptedRound, r.Gate.MaxRounds)

	writeResult(&b, r, rd)
	writeGuards(&b, rd)
	writeCost(&b, r)
	writeDetails(&b, r, rd)
	return b.String()
}

func writeResult(b *strings.Builder, r *Report, rd *RoundReport) {
	b.WriteString("### Result on held-out cases\n\n")
	if rd != nil && rd.Held != nil {
		h := rd.Held
		b.WriteString("| | Baseline | Candidate |\n|---|---|---|\n")
		fmt.Fprintf(b, "| Pass rate | %.0f%% | %.0f%% |\n", h.Base.PassRate*100, h.Cand.PassRate*100)
		fmt.Fprintf(b, "| Trigger precision | %s | %s |\n", ratioText(h.Base.TriggerPrecision), ratioText(h.Cand.TriggerPrecision))
		fmt.Fprintf(b, "| Trigger recall | %s | %s |\n", ratioText(h.Base.TriggerRecall), ratioText(h.Cand.TriggerRecall))
		fmt.Fprintf(b, "| Near-miss false positives | %d | %d |\n\n", h.Base.NearMissFalsePositives, h.Cand.NearMissFalsePositives)
		fmt.Fprintf(b, "Gain %+.1f points over %d held-out case(s), %d runs each, majority vote.", h.Gain*100, len(h.Table), r.Runs)
		if ci := h.CI; ci != nil {
			fmt.Fprintf(b, " %.0f%% bootstrap interval of the gain: [%+.1f, %+.1f] points.", ci.Confidence*100, ci.Low*100, ci.High*100)
		}
		b.WriteString("\n\n")
		fmt.Fprintf(b, "- Wins: %s\n- Losses: %s\n", idList(h.Wins), idList(h.Losses))
		if len(h.Unstable) > 0 {
			fmt.Fprintf(b, "- Unstable (not counted): %s\n", idList(h.Unstable))
		}
		if h.Underpowered {
			fmt.Fprintf(b, "- **Underpowered** (%s): treat the gain as weak evidence.\n", CodeUnderpowered)
		}
		b.WriteString("\n")
	}

}

func writeGuards(b *strings.Builder, rd *RoundReport) {
	b.WriteString("### Guards that held\n\n")
	b.WriteString("- Diff policy: only the editable files changed; no new tool, script, executable bit or frontmatter privilege, and no new security finding.\n")
	if rd != nil && rd.Siblings != nil {
		if rd.Siblings.Skipped != "" {
			fmt.Fprintf(b, "- Sibling trigger guard: skipped (%s).\n", Sanitize(rd.Siblings.Skipped, 200))
		} else {
			fmt.Fprintf(b, "- Sibling trigger guard (%s ranker): %d sibling skill(s) checked, none lost trigger recall.\n", rd.Siblings.Surface, len(rd.Siblings.Results))
		}
	}
	b.WriteString("\n")

}

func writeCost(b *strings.Builder, r *Report) {
	b.WriteString("### Cost, egress and environment\n\n")
	fmt.Fprintf(b, "- Spent $%.2f of $%.2f (evals $%.2f, optimizer-reported $%.2f).\n", r.Costs.TotalUSD, r.Costs.MaxUSD, r.Costs.EvalUSD, r.Costs.OptimizerUSD)
	if r.Costs.OptimizerReportedNoCost {
		b.WriteString("- The optimizer reported no cost; its own spend is bounded only by its credentials.\n")
	}
	opt := r.Optimizer
	if r.Adapter != "" {
		opt = r.Adapter
	}
	fmt.Fprintf(b, "- Optimizer: %s; evals: %s runner, harness %s", mdCode(opt), mdCode(r.EvalRunner), mdCode(r.Harness))
	if r.Model != "" {
		fmt.Fprintf(b, ", model %s", mdCode(r.Model))
	}
	b.WriteString(".\n")
	if r.Isolation != nil {
		fmt.Fprintf(b, "- Isolation: mode %s, confined: %t.\n", mdCode(r.Isolation.Mode), r.Isolation.Confined)
	}
	fmt.Fprintf(b, "- Declared egress: %s (informational: not enforced unless the environment does).\n", idList(r.Egress))
	fmt.Fprintf(b, "- Environment variable names forwarded: %s.\n\n", idList(r.EnvPass))

}

func writeDetails(b *strings.Builder, r *Report, rd *RoundReport) {
	if rd != nil && rd.Description != nil {
		b.WriteString("### Description change\n\nBefore:\n\n")
		b.WriteString(mdFence(rd.Description.Before))
		b.WriteString("\nAfter:\n\n")
		b.WriteString(mdFence(rd.Description.After))
		b.WriteString("\n")
	}
	if rd != nil && (rd.Summary != "" || rd.Notes != "") {
		b.WriteString("### What the optimizer said (untrusted text)\n\n")
		b.WriteString(mdFence(strings.TrimSpace(rd.Summary + "\n" + rd.Notes)))
		b.WriteString("\n")
	}
	var warnings []string
	warnings = append(warnings, r.Warnings...)
	if rd != nil {
		warnings = append(warnings, rd.Warnings...)
	}
	if len(warnings) > 0 {
		b.WriteString("### Warnings\n\n")
		for _, w := range warnings {
			fmt.Fprintf(b, "- %s\n", mdInline(w))
		}
		b.WriteString("\n")
	}

	b.WriteString("### Reviewer checklist\n\n")
	b.WriteString("- [ ] Read the full diff, not only this summary.\n")
	b.WriteString("- [ ] Read the new description out loud: does it say what the skill does and when it applies?\n")
	b.WriteString("- [ ] `allowed-tools`, `model` and `disable-model-invocation` are unchanged.\n")
	b.WriteString("- [ ] The lock diff pins only this skill (and the eval results, if refreshed).\n")
	b.WriteString("- [ ] The held-out cases are ones you would trust to catch a regression.\n\n")
	b.WriteString("This pull request was made by `ai-rulez improve pr` from a local run. It is **not approved**; approval stays a human step.\n")
}

func ratioText(v *float64) string {
	if v == nil {
		return "n/a"
	}
	return fmt.Sprintf("%.0f%%", *v*100)
}

func idList(ids []string) string {
	if len(ids) == 0 {
		return "none"
	}
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = mdCode(id)
	}
	return strings.Join(parts, ", ")
}

// mdCode renders s as an inline code span whose delimiters cannot be closed from inside.
func mdCode(s string) string {
	s = Sanitize(s, 200)
	fence := strings.Repeat("`", longestRun(s, '`')+1)
	pad := ""
	if strings.HasPrefix(s, "`") || strings.HasSuffix(s, "`") {
		pad = " "
	}
	return fence + pad + s + pad + fence
}

// mdInline renders untrusted one-line text as a code span (a warning may quote a path or a message).
func mdInline(s string) string { return mdCode(s) }

// mdFence renders s as a fenced block longer than any backtick run inside it.
func mdFence(s string) string {
	s = strings.TrimSpace(SanitizeMultiline(s))
	fence := strings.Repeat("`", max(3, longestRun(s, '`')+1))
	return fence + "text\n" + s + "\n" + fence + "\n"
}

func longestRun(s string, c rune) int {
	best, cur := 0, 0
	for _, r := range s {
		if r == c {
			cur++
			best = max(best, cur)
		} else {
			cur = 0
		}
	}
	return best
}
