package review

import (
	"fmt"
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
	// Redacted is true when the item holds a credential that [review] on_secret = "redact"
	// masks before anything is sent; the masked text is all a judge ever sees.
	Redacted bool `json:"redacted,omitempty"`
	// Score is the offline score 0-100; nil when the item was not scored.
	Score      *int        `json:"score"`
	Dimensions []DimResult `json:"dimensions,omitempty"`
	// Semantic is the judge's result; nil for an offline run or an item that was not judged.
	Semantic *SemanticResult `json:"semantic,omitempty"`
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
	// Only, when not nil, narrows the selection to these item ids (--since). Siblings
	// are still drawn from every scored item.
	Only map[string]bool
}

// OnSecretMode resolves [review] on_secret: withhold (the default) or redact.
func (in Input) OnSecretMode() string {
	if in.Config != nil && in.Config.OnSecret != "" {
		return in.Config.OnSecret
	}
	return config.ReviewOnSecretWithhold
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
	// notes are run-level findings added after the run (AR9G0, AR9G9).
	notes []Finding
	// baselined holds the fingerprints of accepted findings (see SetBaseline).
	baselined map[string]bool
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
			res.pool = append(res.pool, sendView(it, r.Redacted))
		}
	}
	for _, it := range Selected(in.Items, in.Selector) {
		if in.Only != nil && !in.Only[it.ID] {
			continue
		}
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
	secretFinding, elsewhere := false, false
	for i := range ev {
		switch ev[i].Code {
		case lint.CodeHiddenCharacters:
			r.Status = StatusWithheld
			r.Reason = ev[i].Code + " " + ev[i].Name + ": never sent to a judge"
			return r
		case lint.CodeSecretDetected:
			secretFinding = true
			elsewhere = elsewhere || ev[i].RepoPath() != it.Path
			if in.OnSecretMode() != config.ReviewOnSecretRedact || elsewhere {
				r.Status = StatusWithheld
				r.Reason = ev[i].Code + " " + ev[i].Name + ": never sent to a judge"
				return r
			}
		}
	}
	// The lint findings above can be removed by [lint] ignore, severity, ignore_paths or an
	// inline ignore in the item itself; what may leave the machine must not depend on them.
	if reason := directHiddenReason(it); reason != "" {
		r.Status, r.Reason = StatusWithheld, reason
		return r
	}
	if reason := directSecretReason(it); reason != "" {
		if in.OnSecretMode() != config.ReviewOnSecretRedact {
			r.Status, r.Reason = StatusWithheld, reason
			return r
		}
		secretFinding = true
	}
	if secretFinding {
		// on_secret = "redact": mask the credential, and send only if nothing credential-shaped is left.
		if reason := directSecretReason(sendView(it, true)); reason != "" {
			r.Status, r.Reason = StatusWithheld, reason+" (redaction left a credential-shaped value)"
			return r
		}
		r.Redacted = true
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

// Finding is a review finding: a dimension below pass on one item, or a note about the run.
//
//nolint:tagliatelle // report keys are snake_case by project convention
type Finding struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Severity string `json:"severity"`
	ItemID   string `json:"item,omitempty"`
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Message  string `json:"message"`
	// Origin is lint-twin (derived from deterministic lint evidence) or llm-judge.
	Origin string `json:"origin"`
	// Dimension, Verdict, Status and Agreement describe a dimension finding.
	Dimension  string     `json:"dimension,omitempty"`
	Verdict    string     `json:"verdict,omitempty"`
	Status     string     `json:"status,omitempty"`
	Agreement  float64    `json:"agreement,omitempty"`
	Quote      string     `json:"quote,omitempty"`
	Suggestion string     `json:"suggestion,omitempty"`
	Evidence   []Evidence `json:"evidence,omitempty"`
	// Fingerprint is stable across line moves: sha256 of code, item id, dimension and the first
	// normalised quote (judge) or the twin codes (lint).
	Fingerprint string `json:"fingerprint"`
	// Baselined is true when the finding is in the baseline: it is listed but never gates or fails.
	Baselined bool `json:"baselined,omitempty"`
}

// Pool returns the sendable views of every scored item: the items siblings are drawn from.
func (r *Results) Pool() []Item { return r.pool }

// AddNote records a run-level finding (AR9G0, AR9G9) that Findings and the SARIF output carry.
func (r *Results) AddNote(f Finding) { r.notes = append(r.notes, f) }

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
				Origin: OriginLintTwin, Fingerprint: fingerprint(lint.CodeReviewRunNote, it.ID, it.Status, ""),
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
				Message: it.ID + " " + d.ID + " " + d.Verdict + ": " + evidenceSummary(d.Evidence),
				Origin:  OriginLintTwin, Dimension: d.ID, Verdict: d.Verdict, Status: d.Status,
				Evidence:    d.Evidence,
				Fingerprint: fingerprint(d.Code, it.ID, d.ID, strings.Join(codes, ",")),
			})
		}
		out = append(out, semanticFindings(it)...)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ItemID != out[j].ItemID {
			return out[i].ItemID < out[j].ItemID
		}
		return out[i].Code < out[j].Code
	})
	for i := range out {
		out[i].Baselined = r.baselined[out[i].Fingerprint] && !isNote(out[i].Code)
	}
	return append(out, r.notes...)
}

// semanticFindings lists the judge's findings of one item: verdicts below pass, at the
// dimension's severity ceiling, or at info when the votes disagree or the item was truncated.
func semanticFindings(it *ItemResult) []Finding {
	if it.Semantic == nil {
		return nil
	}
	var out []Finding
	for _, d := range it.Semantic.Dimensions {
		if (d.Status != SemJudged && d.Status != SemUnstable) || d.Verdict == VerdictPass || d.Code == "" {
			continue
		}
		sev := d.severity
		if sev == "" {
			sev = string(lint.SeverityWarning)
		}
		if d.Status == SemUnstable || d.Capped {
			sev = string(lint.SeverityInfo)
		}
		quote := ""
		if len(d.Evidence) > 0 {
			quote = d.Evidence[0].Quote
		}
		msg := fmt.Sprintf("%s %s %s: %s", it.ID, d.ID, d.Verdict, d.Rationale)
		if d.Status == SemUnstable {
			msg = fmt.Sprintf("%s %s unstable (%s on %.0f%% of %d votes): %s", it.ID, d.ID, d.Verdict, d.Agreement*100, len(d.Votes), d.Rationale)
		}
		out = append(out, Finding{
			Code: d.Code, Name: d.ID, Severity: sev, ItemID: it.ID, Path: it.Path, Line: 1, Message: strings.TrimSpace(msg),
			Origin: OriginLLMJudge, Dimension: d.ID, Verdict: d.Verdict, Status: d.Status, Agreement: d.Agreement,
			Quote: quote, Suggestion: d.Suggestion,
			Fingerprint: fingerprint(d.Code, it.ID, d.ID, normSpace(quote)),
		})
	}
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

// directSecretReason scans everything an item could send for a credential, with no lint
// setting applied. It returns "" for clean content.
func directSecretReason(it Item) string {
	for _, text := range [...]string{it.Raw, it.Name, it.Description, it.Body, it.Frontmatter} {
		if name, ok := lint.DetectSecret(text); ok {
			return lint.CodeSecretDetected + " secret-detected: " + name + " in the item content: never sent to a judge"
		}
	}
	return ""
}

// directHiddenReason is directSecretReason for hidden characters.
func directHiddenReason(it Item) string {
	for _, text := range [...]string{it.Raw, it.Name, it.Description, it.Body, it.Frontmatter} {
		if name, ok := lint.DetectHidden(text); ok {
			return lint.CodeHiddenCharacters + " hidden-characters: " + name + ": never sent to a judge"
		}
	}
	return ""
}

// sendView is the item as a judge sees it: unchanged, or with every credential masked
// when the item is redacted. The raw file is dropped from a masked view so nothing
// downstream can send it by mistake.
func sendView(it Item, redacted bool) Item {
	if !redacted {
		return it
	}
	it.Raw = ""
	it.Name = lint.RedactSecrets(it.Name)
	it.Description = lint.RedactSecrets(it.Description)
	it.Body = lint.RedactSecrets(it.Body)
	it.Frontmatter = lint.RedactSecrets(it.Frontmatter)
	return it
}
