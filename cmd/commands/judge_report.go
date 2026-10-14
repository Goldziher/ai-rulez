package commands

import (
	"fmt"
	"io"

	"github.com/Goldziher/ai-rulez/v5/internal/jsondoc"
	"github.com/Goldziher/ai-rulez/v5/internal/judge"
)

// The statuses of a file in a judge report.
const (
	judgeStatusJudged    = "judged"
	judgeStatusSkipped   = "skipped"
	judgeStatusError     = "error"
	judgeStatusWouldSend = "would_send"
)

// judgeFileReport is one file of the report. It never carries file text or the prompt.
//
//nolint:tagliatelle // output keys are snake_case by project convention
type judgeFileReport struct {
	Path      string   `json:"path"`
	Status    string   `json:"status"`
	Score     *float64 `json:"score,omitempty"`
	Passed    *bool    `json:"passed,omitempty"`
	Rationale string   `json:"rationale,omitempty"`
	Reason    string   `json:"reason,omitempty"`
	Bytes     int      `json:"bytes"`
	Truncated bool     `json:"truncated,omitempty"`
	Redacted  bool     `json:"redacted,omitempty"`
	// skipKind is the judge.Skip* reason of a skipped file; it is not printed.
	skipKind string
}

// judgeEstimate is the worst case of sending the files that can be sent.
//
//nolint:tagliatelle // output keys are snake_case by project convention
type judgeEstimate struct {
	Files               int     `json:"files"`
	PromptTokens        int     `json:"prompt_tokens"`
	MaxCompletionTokens int     `json:"max_completion_tokens"`
	CostUSD             float64 `json:"cost_usd"`
	CostKnown           bool    `json:"cost_known"`
}

// judgeSummary counts the outcomes and what the run spent.
//
//nolint:tagliatelle // output keys are snake_case by project convention
type judgeSummary struct {
	Judged  int     `json:"judged"`
	Passed  int     `json:"passed"`
	Failed  int     `json:"failed"`
	Skipped int     `json:"skipped"`
	Errors  int     `json:"errors"`
	Calls   int     `json:"calls"`
	Tokens  int     `json:"tokens"`
	CostUSD float64 `json:"cost_usd"`
}

// judgeReport is the document `judge --format json` prints (schema/judge-report.schema.json).
//
//nolint:tagliatelle // output keys are snake_case by project convention
type judgeReport struct {
	// Mode is "judge" for a run that called the model and "estimate" for one that sent nothing.
	Mode       string            `json:"mode"`
	Rubric     string            `json:"rubric"`
	Model      string            `json:"model"`
	EgressHost string            `json:"egress_host"`
	MinScore   float64           `json:"min_score"`
	Files      []judgeFileReport `json:"files"`
	Estimate   judgeEstimate     `json:"estimate"`
	Summary    judgeSummary      `json:"summary"`
}

const (
	judgeModeJudge    = "judge"
	judgeModeEstimate = "estimate"
)

// skippedForSecrets counts the files not sent because they held secret-looking text.
func (r *judgeReport) skippedForSecrets() int {
	n := 0
	for _, f := range r.Files {
		if f.Status == judgeStatusSkipped && f.skipKind == judge.SkipSecrets {
			n++
		}
	}
	return n
}

// tally fills the summary counts from the files.
func (r *judgeReport) tally() {
	s := &r.Summary
	for _, f := range r.Files {
		switch f.Status {
		case judgeStatusJudged:
			s.Judged++
			if f.Passed != nil && *f.Passed {
				s.Passed++
			} else {
				s.Failed++
			}
		case judgeStatusSkipped:
			s.Skipped++
		case judgeStatusError:
			s.Errors++
		}
	}
}

func (r *judgeReport) write(out io.Writer, asJSON bool) error {
	if asJSON {
		return jsondoc.Write(out, r)
	}
	if r.Mode == judgeModeEstimate {
		r.writeEstimateText(out)
		return nil
	}
	r.writeJudgeText(out)
	return nil
}

func (r *judgeReport) writeJudgeText(out io.Writer) {
	for _, f := range r.Files {
		switch f.Status {
		case judgeStatusJudged:
			label := "PASS"
			if f.Passed == nil || !*f.Passed {
				label = "FAIL"
			}
			note := ""
			if f.Truncated {
				note = " (truncated)"
			}
			fprintf(out, "%s  %.2f  %s%s  %s\n", label, *f.Score, f.Path, note, f.Rationale)
		case judgeStatusSkipped:
			fprintf(out, "SKIP        %s  %s\n", f.Path, f.Reason)
		default:
			fprintf(out, "ERROR       %s  %s\n", f.Path, f.Reason)
		}
	}
	s := r.Summary
	fprintf(out, "judged %d: %d passed, %d below %.2f, %d skipped, %d errors; %d calls, %d tokens, ~$%.4f\n",
		s.Judged, s.Passed, s.Failed, r.MinScore, s.Skipped, s.Errors, s.Calls, s.Tokens, s.CostUSD)
}

func (r *judgeReport) writeEstimateText(out io.Writer) {
	host := r.EgressHost
	if host == "" {
		host = "(the provider's default endpoint)"
	}
	model := r.Model
	if model == "" {
		model = "(none: pass --model or set AI_RULEZ_LLM_MODEL)"
	}
	fprintf(out, "judge estimate (nothing was sent)\nrubric: %s\nmodel:  %s\negress: %s\n", r.Rubric, model, host)
	for _, f := range r.Files {
		if f.Status == judgeStatusSkipped {
			fprintf(out, "  skip  %s  %s\n", f.Path, f.Reason)
			continue
		}
		note := ""
		if f.Truncated {
			note = " (head and tail only)"
		}
		fprintf(out, "  send  %s  %d bytes%s\n", f.Path, f.Bytes, note)
	}
	e := r.Estimate
	cost := "unknown (no price for this model; set AI_RULEZ_LLM_MODEL to a priced model)"
	if e.CostKnown {
		cost = fmt.Sprintf("<= $%.6f", e.CostUSD)
	}
	fprintf(out, "files to send: %d, prompt tokens: ~%d, max completion tokens: %d\nworst-case cost: %s\n",
		e.Files, e.PromptTokens, e.MaxCompletionTokens, cost)
	if r.Summary.Skipped > 0 {
		fprintf(out, "skipped: %d\n", r.Summary.Skipped)
	}
}
