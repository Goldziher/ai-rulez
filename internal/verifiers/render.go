package verifiers

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/samber/oops"
)

type jsonReport struct {
	Root    string         `json:"root,omitempty"`
	Summary map[string]int `json:"summary"`
	Results []Result       `json:"results"`
}

func summary(r *Report) map[string]int {
	c := r.Counts()
	return map[string]int{
		string(StatusPass):  c[StatusPass],
		string(StatusFail):  c[StatusFail],
		string(StatusError): c[StatusError],
	}
}

// WriteJSON writes the report as indented JSON.
func WriteJSON(w io.Writer, r *Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(jsonReport{Root: r.Root, Summary: summary(r), Results: r.Results}); err != nil {
		return oops.Wrapf(err, "write verifiers report")
	}
	return nil
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
	c := r.Counts()
	_, err := fmt.Fprintf(w, "\n%d passed, %d failed, %d could not run\n", c[StatusPass], c[StatusFail], c[StatusError])
	return wrapWrite(err)
}

func wrapWrite(err error) error {
	if err != nil {
		return oops.Wrapf(err, "write verifiers report")
	}
	return nil
}
