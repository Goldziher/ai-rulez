package importer

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// ReportSchemaVersion is the schema_version of the JSON report
// (schema/convert-report.schema.json).
const ReportSchemaVersion = 1

// File actions in a report.
const (
	ActionCreate    = "create"
	ActionUnchanged = "unchanged"
	ActionOverwrite = "overwrite"
	ActionConflict  = "conflict"
	ActionMerge     = "merge"
	ActionManual    = "manual"
)

// Counts tallies findings by status.
type Counts struct {
	Mapped       int `json:"mapped"`
	Approximated int `json:"approximated"`
	Dropped      int `json:"dropped"`
	NeedsAction  int `json:"needs_action"`
	Unsupported  int `json:"unsupported"`
}

// FileAction is one file the conversion would write (or skip).
type FileAction struct {
	Path   string `json:"path"`
	Action string `json:"action"`
}

// SecurityResult is the outcome of the pre-write security scan.
type SecurityResult struct {
	Blocked  bool              `json:"blocked"`
	Code     string            `json:"code,omitempty"`
	Findings []SecurityFinding `json:"findings"`
}

// SecurityFinding is one finding of the scan (an AR0xx rule).
type SecurityFinding struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	File     string `json:"file"`
	Line     int    `json:"line,omitempty"`
	Message  string `json:"message"`
	// Planned is the path and line in the planned .ai-rulez tree when File and
	// Line point at the source file the text came from.
	Planned string `json:"planned,omitempty"`
	// Allowed is set when --allow-findings lets the finding through.
	Allowed bool `json:"allowed,omitempty"`
}

// ValidationResult is the outcome of loading the planned tree.
type ValidationResult struct {
	Errors   int      `json:"errors"`
	Warnings int      `json:"warnings"`
	Messages []string `json:"messages,omitempty"`
}

// Report is the lossiness report of one conversion.
type Report struct {
	SchemaVersion int              `json:"schema_version"`
	Importer      string           `json:"importer"`
	Source        string           `json:"source"`
	Into          string           `json:"into"`
	Written       bool             `json:"written"`
	Counts        Counts           `json:"counts"`
	Findings      []Finding        `json:"findings"`
	Files         []FileAction     `json:"files"`
	Security      SecurityResult   `json:"security"`
	Validation    ValidationResult `json:"validation"`

	// Summary lines for the text report; not part of the JSON.
	plan string
	// needsLock is set when the converted config names remote sources that
	// ai-rulez.lock should pin.
	needsLock bool
}

// NeedsLock reports whether the converted config holds remote sources that
// `ai-rulez lock` should pin (convert never writes the lock itself; --lock runs it).
func (r *Report) NeedsLock() bool { return r.needsLock }

func (r *Report) count() {
	r.Counts = Counts{}
	for _, f := range r.Findings {
		switch f.Status {
		case StatusMapped:
			r.Counts.Mapped++
		case StatusApproximated:
			r.Counts.Approximated++
		case StatusDropped:
			r.Counts.Dropped++
		case StatusNeedsAction:
			r.Counts.NeedsAction++
		case StatusUnsupported:
			r.Counts.Unsupported++
		}
	}
}

// Conflicts returns the number of files that exist with different content.
func (r *Report) Conflicts() int {
	n := 0
	for _, f := range r.Files {
		if f.Action == ActionConflict {
			n++
		}
	}
	return n
}

// Matches reports whether any finding has one of the given statuses (the
// names of --fail-on: mapped, approximated, dropped, needs-action, unsupported).
func (r *Report) Matches(statuses []string) bool {
	for _, f := range r.Findings {
		for _, s := range statuses {
			if string(f.Status) == s || strings.ReplaceAll(string(f.Status), "-", "_") == s {
				return true
			}
		}
	}
	return false
}

// WriteJSON prints the report as JSON.
func (r *Report) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// WriteText prints the report for people. Mapped findings are grouped to one
// line per target kind so the lossy ones stand out.
func (r *Report) WriteText(w io.Writer) {
	fmt.Fprintf(w, "source      %s (%s)\n", r.Importer, r.Source)
	fmt.Fprintf(w, "into        %s\n", r.Into)
	if r.plan != "" {
		fmt.Fprintf(w, "plan        %s\n", r.plan)
	}
	if r.Written {
		fmt.Fprintf(w, "written     %d file(s)\n", countActions(r.Files, ActionCreate, ActionOverwrite, ActionMerge))
	} else {
		fmt.Fprintf(w, "written     (nothing written)\n")
	}
	fmt.Fprintln(w)

	files := append([]FileAction(nil), r.Files...)
	sort.SliceStable(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	for _, f := range files {
		fmt.Fprintf(w, "%-10s  %s\n", strings.ToUpper(f.Action), f.Path)
	}
	if len(files) > 0 {
		fmt.Fprintln(w)
	}
	for _, f := range r.Findings {
		if f.Status == StatusMapped {
			continue
		}
		line := fmt.Sprintf("%-13s %s", strings.ToUpper(string(f.Status)), f.Source)
		if f.Field != "" {
			line += "  " + f.Field
		}
		if f.Target != "" {
			line += " -> " + f.Target
		}
		if f.Reason != "" {
			line += "  " + f.Reason
		}
		fmt.Fprintln(w, line)
	}
	c := r.Counts
	fmt.Fprintf(w, "\nSummary: %d mapped, %d approximated, %d dropped, %d needs-action, %d unsupported.\n",
		c.Mapped, c.Approximated, c.Dropped, c.NeedsAction, c.Unsupported)
	if len(r.Security.Findings) == 0 {
		fmt.Fprintln(w, "Security scan: 0 findings.")
	} else {
		fmt.Fprintf(w, "Security scan: %d finding(s)%s.\n", len(r.Security.Findings), blockedSuffix(r.Security.Blocked))
		for _, f := range r.Security.Findings {
			fmt.Fprintf(w, "  %s %s %s:%d %s%s\n", f.Severity, f.Code, f.File, f.Line, f.Message, findingSuffix(f))
		}
	}
	fmt.Fprintf(w, "Validation of the planned tree: %d error(s), %d warning(s).\n", r.Validation.Errors, r.Validation.Warnings)
	for _, m := range r.Validation.Messages {
		fmt.Fprintf(w, "  %s\n", m)
	}
}

func findingSuffix(f SecurityFinding) string {
	var parts []string
	if f.Planned != "" {
		parts = append(parts, "planned "+f.Planned)
	}
	if f.Allowed {
		parts = append(parts, "allowed by --allow-findings")
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, "; ") + ")"
}

func blockedSuffix(b bool) string {
	if b {
		return " (blocked, nothing written)"
	}
	return ""
}

func countActions(files []FileAction, actions ...string) int {
	n := 0
	for _, f := range files {
		for _, a := range actions {
			if f.Action == a {
				n++
			}
		}
	}
	return n
}
