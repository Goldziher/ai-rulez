package tagresolve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/semver"
)

// OutdatedSchemaVersion versions the JSON of `lock --outdated --format json`.
const OutdatedSchemaVersion = 1

// Statuses of a source in the outdated report.
const (
	StatusUpToDate    = "up-to-date"
	StatusUpdatable   = "updatable"
	StatusNotLocked   = "not-locked"
	StatusTagMoved    = "tag-moved"
	StatusTagMissing  = "tag-missing"
	StatusUnsatisfied = "unsatisfiable"
	StatusInvalid     = "invalid"
	StatusLowerOnly   = "downgrade-only"
	// StatusLockedNonVersion marks a source pinned at a tag that is not a version
	// (pinned by an arbitrary ref before a constraint was added); moving it needs consent.
	StatusLockedNonVersion = "locked-non-version"
)

// TagRef is a tag and the commit it resolves to.
type TagRef struct {
	Tag    string `json:"tag"`
	Commit string `json:"commit,omitempty"`
}

// Row is one source of the outdated report.
type Row struct {
	Kind       string  `json:"kind"`
	Name       string  `json:"name"`
	Source     string  `json:"source"`
	Constraint string  `json:"constraint"`
	Locked     *TagRef `json:"locked,omitempty"`
	// Allowed is the newest tag the constraint allows.
	Allowed *TagRef `json:"allowed,omitempty"`
	// Latest is the newest version tag whatever the constraint says.
	Latest *TagRef `json:"latest,omitempty"`
	Status string  `json:"status"`
	// Held lists the newer tags min_release_age holds back (AR733).
	Held []Held `json:"held_back,omitempty"`
	// Code is the rule code of an error or warning status (AR730, AR731, AR732, AR735).
	Code string `json:"code,omitempty"`
	// Severity is set for an AR734 finding: "error", "warning" or "info".
	Severity string `json:"severity,omitempty"`
	Note     string `json:"note,omitempty"`
	// Downgrade is true when the newest allowed tag is below the pinned one.
	Downgrade bool `json:"downgrade,omitempty"`
	// MajorAvailable is true when Latest is a newer major version than Allowed.
	MajorAvailable bool `json:"major_available,omitempty"`
}

// Updatable reports whether the constraint allows a newer tag than the pinned one.
func (r Row) Updatable() bool { return r.Status == StatusUpdatable }

// Report is the document of `lock --outdated --format json`.
type Report struct {
	SchemaVersion int     `json:"schema_version"`
	Sources       []Row   `json:"sources"`
	Summary       Summary `json:"summary"`
}

// Summary counts the report.
type Summary struct {
	Total          int `json:"total"`
	Updatable      int `json:"updatable"`
	MajorAvailable int `json:"major_available"`
	HeldBack       int `json:"held_back"`
	TagMoved       int `json:"tag_moved"`
	Errors         int `json:"errors"`
}

// Evaluate compares one source's lock entry (nil when unlocked) with the remote's tags.
func Evaluate(kind, name string, w lockfile.Want, entry *lockfile.Entry, tags []RawTag) Row {
	return EvaluateGated(context.Background(), kind, name, w, entry, tags, nil)
}

// EvaluateGated is Evaluate with a minimum release age: the allowed tag is the
// newest one that passes the gate (the pinned tag always does), and the tags the
// gate held back are listed.
func EvaluateGated(ctx context.Context, kind, name string, w lockfile.Want, entry *lockfile.Entry, tags []RawTag, gate *AgeGate) Row {
	row := Row{Kind: kind, Name: name, Source: w.Source, Constraint: w.Constraint}
	if entry != nil && entry.Tag != "" {
		row.Locked = &TagRef{Tag: entry.Tag, Commit: entry.Commit}
	}
	spec := Spec{Constraint: w.Constraint, TagPrefix: w.TagPrefix, IncludePrerelease: w.IncludePrerelease}
	if entry != nil {
		spec.Pinned = entry.Tag
	}
	sel, err := SelectGated(ctx, tags, spec, gate)
	if err != nil {
		row.Status, row.Code, row.Note = StatusUnsatisfied, CodeUnsatisfiable, err.Error()
		var coded *Error
		if errors.As(err, &coded) {
			row.Code, row.Note = coded.Code, coded.Msg
			if coded.Code == CodeConstraintBad {
				row.Status = StatusInvalid
			}
		}
		row.Note = oneLine(row.Note)
		return row
	}
	row.Allowed = &TagRef{Tag: sel.Chosen.Tag.Name, Commit: sel.Chosen.Tag.Commit}
	row.Latest = &TagRef{Tag: sel.Latest.Tag.Name, Commit: sel.Latest.Tag.Commit}
	row.Held = sel.Held
	row.MajorAvailable = sel.Latest.Version.Major > sel.Chosen.Version.Major
	if len(sel.Notes) > 0 {
		row.Note = strings.Join(sel.Notes, "; ")
	}
	if row.Locked == nil {
		row.Status = StatusNotLocked
		return row
	}
	locked, parsed := semver.ParseTag(entry.Tag, w.TagPrefix)
	row.Downgrade = parsed && sel.Chosen.Version.Compare(locked) < 0
	switch status, now := Check(tags, entry.Tag, entry.Commit); status {
	case StatusMoved:
		row.Status, row.Code = StatusTagMoved, CodeTagMoved
		row.Note = fmt.Sprintf("%s now points to %s, pinned at %s", entry.Tag, short(now.Commit), short(entry.Commit))
		return row
	case StatusMissing:
		row.Status, row.Code = StatusTagMissing, CodeLockedTagMissed
		row.Note = fmt.Sprintf("%s no longer exists on the remote", entry.Tag)
		if row.Downgrade {
			row.Note += fmt.Sprintf("; the newest allowed tag %s is below it, update needs --allow-downgrade", sel.Chosen.Tag.Name)
		}
		return row
	case StatusOK:
	}
	switch {
	case !parsed:
		row.Status = StatusLockedNonVersion
		row.Note = fmt.Sprintf("the pinned tag %s is not a version; moving to %s needs --allow-downgrade", entry.Tag, sel.Chosen.Tag.Name)
	case sel.Chosen.Version.Compare(locked) > 0:
		row.Status = StatusUpdatable
	case sel.Chosen.Version.Compare(locked) < 0:
		row.Status = StatusLowerOnly
		row.Note = fmt.Sprintf("the newest allowed tag %s is below the pinned %s; update needs --allow-downgrade", sel.Chosen.Tag.Name, entry.Tag)
	default:
		row.Status = StatusUpToDate
	}
	return row
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// NewReport sorts the rows (by kind, then name) and counts them.
func NewReport(rows []Row) *Report {
	rep := &Report{SchemaVersion: OutdatedSchemaVersion, Sources: append([]Row{}, rows...)}
	sort.SliceStable(rep.Sources, func(i, j int) bool {
		a, b := rep.Sources[i], rep.Sources[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.Name < b.Name
	})
	rep.Summary.Total = len(rows)
	for _, r := range rows {
		switch r.Status {
		case StatusUpdatable:
			rep.Summary.Updatable++
		case StatusTagMoved:
			rep.Summary.TagMoved++
			rep.Summary.Errors++
		case StatusTagMissing:
			// A vanished pin is a supply-chain signal (the history may have been rewritten): fail the check.
			rep.Summary.Errors++
		case StatusUnsatisfied, StatusInvalid:
			rep.Summary.Errors++
		}
		if r.MajorAvailable {
			rep.Summary.MajorAvailable++
		}
		if len(r.Held) > 0 {
			rep.Summary.HeldBack++
		}
	}
	return rep
}

// MarkOutdated reports every updatable source as an AR734 (source-outdated)
// finding of the given severity ("error", "warning" or "info"; anything else,
// including "off", changes nothing). An error finding makes the report fail.
func (r *Report) MarkOutdated(severity string) {
	switch severity {
	case "error", "warning", "info":
	default:
		return
	}
	for i := range r.Sources {
		row := &r.Sources[i]
		if row.Status != StatusUpdatable || row.Code != "" {
			continue
		}
		row.Code, row.Severity = CodeOutdated, severity
		if severity == "error" {
			r.Summary.Errors++
		}
	}
}

// Failing reports whether the report holds an error finding: a moved or missing
// pinned tag, or an unresolvable constraint.
func (r *Report) Failing() bool { return r.Summary.Errors > 0 }

// WriteJSON writes the report as indented JSON.
func (r *Report) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// WriteText writes the report as a table and a summary line.
func (r *Report) WriteText(w io.Writer) error {
	if len(r.Sources) == 0 {
		_, err := fmt.Fprintln(w, "no source uses a version constraint")
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "SOURCE\tKIND\tCONSTRAINT\tLOCKED\tALLOWED\tLATEST\tNOTE")
	for _, row := range r.Sources {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", row.Name, row.Kind, row.Constraint,
			tagName(row.Locked), tagName(row.Allowed), tagName(row.Latest), rowNote(row))
	}
	if err := tw.Flush(); err != nil {
		return err //nolint:wrapcheck // writer error
	}
	s := r.Summary
	held := ""
	if s.HeldBack > 0 {
		held = fmt.Sprintf(", %d held back", s.HeldBack)
	}
	_, err := fmt.Fprintf(w, "%d source(s): %d updatable%s, %d with a newer major, %d moved tag(s), %d error(s)\n",
		s.Total, s.Updatable, held, s.MajorAvailable, s.TagMoved, s.Errors)
	return err //nolint:wrapcheck // writer error
}

func tagName(t *TagRef) string {
	if t == nil {
		return "-"
	}
	return t.Tag
}

func rowNote(r Row) string {
	var parts []string
	switch r.Status {
	case StatusUpToDate:
		parts = append(parts, "up to date within the constraint")
	case StatusUpdatable:
		parts = append(parts, "update available")
	case StatusNotLocked:
		parts = append(parts, "not locked; run `ai-rulez lock`")
	}
	if r.Code != "" {
		parts = append(parts, r.Code)
	}
	if r.Note != "" {
		parts = append(parts, r.Note)
	}
	for _, h := range r.Held {
		parts = append(parts, h.String())
	}
	if r.MajorAvailable {
		parts = append(parts, "newer major available")
	}
	return strings.Join(parts, "; ")
}
