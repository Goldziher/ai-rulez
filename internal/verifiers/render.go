package verifiers

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/jsondoc"
)

type jsonReport struct {
	Root    string         `json:"root,omitempty"`
	Mode    string         `json:"mode,omitempty"`
	Summary map[string]int `json:"summary"`
	Results []Result       `json:"results"`
	LLM     *LLMUsage      `json:"llm,omitempty"`
}

func summary(r *Report) map[string]int {
	c := r.Counts()
	out := map[string]int{
		string(StatusPass):          c[StatusPass],
		string(StatusFail):          c[StatusFail],
		string(StatusError):         c[StatusError],
		string(StatusNotApplicable): c[StatusNotApplicable],
	}
	// Reported only when present, so a report without these stays as it was.
	for _, s := range []Status{StatusSkipped, StatusInactive} {
		if c[s] > 0 {
			out[string(s)] = c[s]
		}
	}
	return out
}

// WriteJSON writes the report as indented JSON.
func WriteJSON(w io.Writer, r *Report) error {
	return jsondoc.Write(w, jsonReport{Root: r.Root, Mode: r.Mode, Summary: summary(r), Results: r.Results, LLM: r.LLM})
}

// WriteText writes the report as a table followed by a summary line.
func WriteText(w io.Writer, r *Report) error {
	if len(r.Results) == 0 {
		_, err := fmt.Fprintln(w, "No verifiers configured.")
		return wrapWrite(err)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "STATUS\tSEVERITY\tNAME\tMESSAGE")
	for _, res := range r.Results {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", res.Status, res.Severity, sanitize(res.Name), sanitize(res.Message))
	}
	if err := tw.Flush(); err != nil {
		return wrapWrite(err)
	}
	writeDetails(w, r)
	c := r.Counts()
	line := fmt.Sprintf("\n%d passed, %d failed, %d could not run", c[StatusPass], c[StatusFail], c[StatusError])
	if n := c[StatusNotApplicable]; n > 0 {
		line += fmt.Sprintf(", %d not applicable", n)
	}
	if n := c[StatusSkipped]; n > 0 {
		line += fmt.Sprintf(", %d skipped", n)
	}
	if n := c[StatusInactive]; n > 0 {
		line += fmt.Sprintf(", %d inactive", n)
	}
	if r.Mode != "" {
		line += " (" + r.Mode + ")"
	}
	if u := r.LLM; u != nil && u.Calls > 0 {
		line += fmt.Sprintf("\nllm: %d call(s), %d from cache, %d prompt + %d completion tokens, about $%.4f", u.Calls, u.Cached, u.PromptTokens, u.CompletionTokens, u.CostUSD)
		if u.MaxCostUSD > 0 {
			line += fmt.Sprintf(" of $%.2f", u.MaxCostUSD)
		}
	}
	_, err := fmt.Fprintln(w, line)
	return wrapWrite(err)
}

// writeDetails prints, for each failed or invalid verifier, the rule it
// enforces, every finding and the fix.
func writeDetails(w io.Writer, r *Report) {
	first := true
	for _, res := range r.Results {
		if res.Status != StatusFail && !(res.Status == StatusError && res.Code != "") {
			continue
		}
		if len(res.Findings) == 0 && res.Fix == "" && res.Target == nil {
			continue
		}
		if first {
			fmt.Fprintln(w)
			first = false
		}
		head := res.Code + " " + sanitize(res.Name)
		if res.Target != nil {
			head += fmt.Sprintf(" (%s %q", res.Target.Kind, sanitize(res.Target.ID))
			if res.Target.Path != "" {
				head += ", " + sanitize(res.Target.Path)
				if res.Target.Line > 0 {
					head += fmt.Sprintf(":%d", res.Target.Line)
				}
			}
			head += ")"
		}
		fmt.Fprintln(w, head)
		for _, f := range res.Findings {
			fmt.Fprintln(w, "  "+findingLine(f))
		}
		if res.Fix != "" {
			fmt.Fprintln(w, "  fix: "+sanitize(res.Fix))
		}
	}
}

func findingLine(f Finding) string {
	loc := ""
	if f.File != "" {
		loc = sanitize(f.File)
		if f.Line > 0 {
			loc += fmt.Sprintf(":%d", f.Line)
		}
		loc += "  "
	}
	line := loc + sanitize(f.Message)
	if f.Match != "" {
		line += "  [" + f.Match + "]"
	}
	return line
}

func wrapWrite(err error) error {
	if err != nil {
		return oops.Wrapf(err, "write verifiers report")
	}
	return nil
}
