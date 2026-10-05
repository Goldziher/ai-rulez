package doctor

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/samber/oops"
)

// jsonReport is the stable machine-readable shape.
type jsonReport struct {
	Root     string         `json:"root,omitempty"`
	Summary  map[string]int `json:"summary"`
	Findings []Finding      `json:"findings"`
}

// WriteJSON writes the report as indented JSON.
func WriteJSON(w io.Writer, r *Report) error {
	c := r.Counts()
	out := jsonReport{
		Root: r.Root,
		Summary: map[string]int{
			string(SeverityError):   c[SeverityError],
			string(SeverityWarning): c[SeverityWarning],
			string(SeverityInfo):    c[SeverityInfo],
		},
		Findings: r.Findings,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return oops.Wrapf(err, "write doctor report")
	}
	return nil
}

// WriteText writes the report as a table followed by a summary line.
func WriteText(w io.Writer, r *Report) error {
	if len(r.Findings) > 0 {
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		if _, err := fmt.Fprintln(tw, "SEVERITY\tCHECK\tMESSAGE"); err != nil {
			return wrap(err)
		}
		for _, f := range r.Findings {
			msg := f.Message
			if f.Path != "" {
				msg = f.Path + ": " + msg
			}
			if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\n", f.Severity, f.Check, msg); err != nil {
				return wrap(err)
			}
			if f.Hint != "" {
				if _, err := fmt.Fprintf(tw, "\t\t  fix: %s\n", f.Hint); err != nil {
					return wrap(err)
				}
			}
		}
		if err := tw.Flush(); err != nil {
			return oops.Wrapf(err, "write doctor report")
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return wrap(err)
		}
	}
	c := r.Counts()
	if len(r.Findings) == 0 {
		_, err := fmt.Fprintln(w, "No problems found.")
		return wrap(err)
	}
	_, err := fmt.Fprintf(w, "%s, %s, %d info\n", plural(c[SeverityError], "error"), plural(c[SeverityWarning], "warning"), c[SeverityInfo])
	return wrap(err)
}

func wrap(err error) error {
	if err != nil {
		return oops.Wrapf(err, "write doctor report")
	}
	return nil
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}
