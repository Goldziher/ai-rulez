package review

import (
	"math"
	"path"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
)

// Item statuses.
const (
	StatusScored   = "scored"
	StatusWithheld = "withheld"
	StatusExcluded = "excluded"
	StatusSkipped  = "skipped"
)

// Dimension statuses: scored from lint evidence, or left for a judge.
const (
	DimScored    = "scored"
	DimNotScored = "not-scored"
)

// withholdCodes are the lint findings that keep an item from ever leaving the machine.
var withholdCodes = map[string]bool{lint.CodeSecretDetected: true, lint.CodeHiddenCharacters: true}

// Evidence is one lint finding behind a dimension's verdict.
type Evidence struct {
	Code     string `json:"code"`
	Name     string `json:"name,omitempty"`
	Severity string `json:"severity"`
	Line     int    `json:"line,omitempty"`
	Message  string `json:"message"`
}

// DimResult is the offline result of one dimension for one item.
type DimResult struct {
	ID       string     `json:"id"`
	Code     string     `json:"code,omitempty"`
	Weight   float64    `json:"weight"`
	Status   string     `json:"status"`
	Verdict  string     `json:"verdict,omitempty"`
	Evidence []Evidence `json:"evidence,omitempty"`
	// Preempted is true when a twin reported an error: a judge would not be asked.
	Preempted bool `json:"preempted,omitempty"`
	// Note says why a dimension was not scored.
	Note string `json:"note,omitempty"`

	severity string
}

// ItemResult is the review of one item.
type ItemResult struct {
	Item
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
	// Score is the offline score 0-100; nil when the item was not scored.
	Score      *int        `json:"score"`
	Dimensions []DimResult `json:"dimensions,omitempty"`
}

// Input is everything Run needs; the caller supplies the lint findings so the
// package never reads the environment, the clock or the process.
type Input struct {
	Rubric   *Rubric
	Items    []Item
	Selector []string
	Findings []lint.Finding
	Config   *config.ReviewConfig
	// IncludeImports also reviews content from includes, installed skills and builtins.
	IncludeImports bool
	// Content overrides [review] content when set (descriptions or full).
	Content string
}

// ContentMode resolves the effective content mode.
func (in Input) ContentMode() string {
	if in.Content != "" {
		return in.Content
	}
	if in.Config != nil && in.Config.Content != "" {
		return in.Config.Content
	}
	return config.ReviewContentDescriptions
}

// Results is the output of Run: one entry per selected item, sorted by ID.
type Results struct {
	Items []ItemResult
	// siblings are all items that could be named as a sibling, per kind.
	pool []Item
}

// Run scores the selected items against the rubric from lint evidence alone.
func Run(in Input) *Results {
	evidence := groupFindings(in.Findings)
	var exclude []string
	if in.Config != nil {
		exclude = in.Config.Exclude
	}
	res := &Results{}
	status := map[string]ItemResult{}
	for _, it := range in.Items {
		status[it.ID] = judgeItem(in, it, evidenceFor(evidence, it), exclude)
	}
	for _, it := range in.Items {
		if r := status[it.ID]; r.Status == StatusScored {
			res.pool = append(res.pool, it)
		}
	}
	for _, it := range Selected(in.Items, in.Selector) {
		res.Items = append(res.Items, status[it.ID])
	}
	return res
}

// judgeItem decides an item's status and, when it is reviewable, its score.
func judgeItem(in Input, it Item, ev []lint.Finding, exclude []string) ItemResult {
	r := ItemResult{Item: it}
	switch {
	case !in.Rubric.Applies(it.Kind):
		r.Status, r.Reason = StatusSkipped, "rubric "+in.Rubric.ID+" does not apply to "+it.Kind+" items"
		return r
	case !it.Owned && !in.IncludeImports:
		r.Status, r.Reason = StatusSkipped, "imported content (include, installed skill or builtin); use --include-imports"
		return r
	case it.ReadError != "":
		r.Status, r.Reason = StatusSkipped, "unreadable: "+it.ReadError
		return r
	}
	if g, ok := excludedBy(it, exclude); ok {
		r.Status, r.Reason = StatusExcluded, "matches [review] exclude "+g
		return r
	}
	for i := range ev {
		if withholdCodes[ev[i].Code] {
			r.Status = StatusWithheld
			r.Reason = ev[i].Code + " " + ev[i].Name + ": never sent to a judge"
			return r
		}
	}
	r.Status = StatusScored
	r.Dimensions = scoreDimensions(in.Rubric, ev)
	r.Score = scoreOf(r.Dimensions)
	return r
}

func scoreDimensions(rb *Rubric, ev []lint.Finding) []DimResult {
	out := make([]DimResult, 0, len(rb.Dimensions))
	for _, d := range rb.Dimensions {
		dr := DimResult{ID: d.ID, Code: d.Code, Weight: d.Weight, severity: d.Severity}
		if len(d.Twins) == 0 {
			dr.Status, dr.Note = DimNotScored, "no lint twin: needs a judge"
			out = append(out, dr)
			continue
		}
		dr.Status, dr.Verdict = DimScored, VerdictPass
		twins := map[string]bool{}
		for _, t := range d.Twins {
			twins[t] = true
		}
		for i := range ev {
			f := &ev[i]
			if !twins[f.Code] || f.Severity == lint.SeverityOff {
				continue
			}
			dr.Evidence = append(dr.Evidence, Evidence{Code: f.Code, Name: f.Name, Severity: string(f.Severity), Line: f.Line, Message: f.Message})
			switch f.Severity {
			case lint.SeverityError:
				dr.Verdict, dr.Preempted = VerdictFail, true
			case lint.SeverityWarning:
				if dr.Verdict == VerdictPass {
					dr.Verdict = VerdictWarn
				}
			}
		}
		out = append(out, dr)
	}
	return out
}

// scoreOf applies the published formula to the scored dimensions; nil when none was scored.
func scoreOf(dims []DimResult) *int {
	sum, weights := 0.0, 0.0
	for _, d := range dims {
		if d.Status != DimScored {
			continue
		}
		sum += d.Weight * verdictValue[d.Verdict]
		weights += d.Weight
	}
	if weights == 0 {
		return nil
	}
	s := int(math.Round(100 * sum / weights))
	return &s
}

// groupFindings indexes findings by repository path.
func groupFindings(findings []lint.Finding) map[string][]lint.Finding {
	out := map[string][]lint.Finding{}
	for i := range findings {
		p := findings[i].RepoPath()
		out[p] = append(out[p], findings[i])
	}
	return out
}

// evidenceFor returns the findings about an item: its own file and, for a skill
// or command directory, the files below it.
func evidenceFor(byPath map[string][]lint.Finding, it Item) []lint.Finding {
	out := append([]lint.Finding(nil), byPath[it.Path]...)
	if base := strings.ToUpper(path.Base(it.Path)); base == "SKILL.MD" || base == "COMMAND.MD" {
		dir := path.Dir(it.Path) + "/"
		var paths []string
		for p := range byPath {
			if strings.HasPrefix(p, dir) && p != it.Path {
				paths = append(paths, p)
			}
		}
		sort.Strings(paths)
		for _, p := range paths {
			out = append(out, byPath[p]...)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		return out[i].Code < out[j].Code
	})
	return out
}

// Finding is a review finding: a dimension below pass on one item.
type Finding struct {
	Code     string
	Name     string
	Severity string
	ItemID   string
	Path     string
	Line     int
	Message  string
	Evidence []Evidence
	// Fingerprint is stable across line moves: sha256 of code, item id, dimension and twin codes.
	Fingerprint string
}

// Findings lists the dimensions below pass as findings, sorted by item then code.
// A note is reported for every item that was withheld, excluded or skipped, so
// nothing is dropped silently (AR9G0).
func (r *Results) Findings(rb *Rubric) []Finding {
	var out []Finding
	for i := range r.Items {
		it := &r.Items[i]
		if it.Status != StatusScored {
			out = append(out, Finding{
				Code: lint.CodeReviewRunNote, Name: "review-run-note", Severity: string(lint.SeverityInfo),
				ItemID: it.ID, Path: it.Path, Line: 1, Message: it.ID + " " + it.Status + ": " + it.Reason,
				Fingerprint: fingerprint(lint.CodeReviewRunNote, it.ID, it.Status, ""),
			})
			continue
		}
		for _, d := range it.Dimensions {
			if d.Status != DimScored || d.Verdict == VerdictPass || d.Code == "" {
				continue
			}
			var codes []string
			line := 1
			for j, e := range d.Evidence {
				codes = append(codes, e.Code)
				if j == 0 && e.Line > 0 {
					line = e.Line
				}
			}
			sort.Strings(codes)
			name := d.ID
			if dim, ok := lookupDim(rb, d.ID); ok {
				name = dim.ID
			}
			out = append(out, Finding{
				Code: d.Code, Name: name, Severity: d.severity, ItemID: it.ID, Path: it.Path, Line: line,
				Message:     it.ID + " " + d.ID + " " + d.Verdict + ": " + evidenceSummary(d.Evidence),
				Evidence:    d.Evidence,
				Fingerprint: fingerprint(d.Code, it.ID, d.ID, strings.Join(codes, ",")),
			})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ItemID != out[j].ItemID {
			return out[i].ItemID < out[j].ItemID
		}
		return out[i].Code < out[j].Code
	})
	return out
}

func lookupDim(rb *Rubric, id string) (Dimension, bool) {
	if rb == nil {
		return Dimension{}, false
	}
	return rb.Dimension(id)
}

func evidenceSummary(ev []Evidence) string {
	parts := make([]string, 0, len(ev))
	for _, e := range ev {
		parts = append(parts, e.Code+" "+e.Message)
	}
	return strings.Join(parts, "; ")
}
