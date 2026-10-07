package review

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
	"unicode"

	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/sarifout"
)

// ReportSchema identifies the JSON report format (schema/review-report.schema.json).
const ReportSchema = "review-report/1"

// Output formats.
const (
	FormatText  = "text"
	FormatJSON  = "json"
	FormatSARIF = "sarif"
)

// Formats lists the accepted --format values.
func Formats() []string { return []string{FormatText, FormatJSON, FormatSARIF} }

// RubricInfo is the rubric as printed in every report: its identity, the
// published weights and the formula.
type RubricInfo struct {
	ID         string          `json:"id"`
	Version    int             `json:"version"`
	Ref        string          `json:"ref"`
	Digest     string          `json:"digest"`
	Formula    string          `json:"formula"`
	AppliesTo  []string        `json:"applies_to"`
	Dimensions []RubricDimInfo `json:"dimensions"`
}

// RubricDimInfo is one published dimension.
type RubricDimInfo struct {
	ID       string   `json:"id"`
	Code     string   `json:"code,omitempty"`
	Group    string   `json:"group"`
	Weight   float64  `json:"weight"`
	Severity string   `json:"severity"`
	Twins    []string `json:"twins,omitempty"`
}

// RunModel names the model of a judged run: what was asked for and what answered.
type RunModel struct {
	Requested string   `json:"requested,omitempty"`
	Resolved  []string `json:"resolved,omitempty"`
}

// RunInfo records what the run did: phase 0 is the offline score, 1 adds the judge.
//
//nolint:tagliatelle // report keys are snake_case by project convention
type RunInfo struct {
	Phase   int     `json:"phase"`
	Offline bool    `json:"offline"`
	Calls   int     `json:"calls"`
	Cached  int     `json:"cached,omitempty"`
	Tokens  int     `json:"tokens"`
	CostUSD float64 `json:"cost_usd"`
	// CapUSD and CapCalls are the spend ceilings the run was held to (0 = unlimited).
	CapUSD         *float64  `json:"cap_usd,omitempty"`
	CapCalls       *int      `json:"cap_calls,omitempty"`
	Incomplete     bool      `json:"incomplete,omitempty"`
	StoppedBecause string    `json:"stopped_because,omitempty"`
	Unjudged       []string  `json:"unjudged,omitempty"`
	Model          *RunModel `json:"model,omitempty"`
	Votes          int       `json:"votes,omitempty"`
	Content        string    `json:"content,omitempty"`
	// HallucinatedEvidence counts quotes the judge cited that are not in the item.
	HallucinatedEvidence int `json:"hallucinated_evidence,omitempty"`
}

// Summary counts the items by status.
type Summary struct {
	Items     int  `json:"items"`
	Scored    int  `json:"scored"`
	Withheld  int  `json:"withheld"`
	Excluded  int  `json:"excluded"`
	Skipped   int  `json:"skipped"`
	MeanScore *int `json:"mean_score"`
	// Judged counts the items a judge answered for; MeanSemanticScore is their mean semantic score.
	Judged            int  `json:"judged,omitempty"`
	MeanSemanticScore *int `json:"mean_semantic_score,omitempty"`
	// Baselined counts findings the baseline already knows.
	Baselined int `json:"baselined,omitempty"`
}

// EgressInfo summarizes what a judged run sent: sizes, never content.
type EgressInfo struct {
	Items       int      `json:"items"`
	Bytes       int      `json:"bytes"`
	ContentMode string   `json:"content_mode"`
	Host        string   `json:"host,omitempty"`
	Withheld    []string `json:"withheld,omitempty"`
	Redacted    []string `json:"redacted,omitempty"`
}

// BaselineInfo says which baseline hid findings.
type BaselineInfo struct {
	File      string `json:"file,omitempty"`
	Baselined int    `json:"baselined"`
	New       int    `json:"new"`
}

// Report is the machine-readable result of `ai-rulez review`.
type Report struct {
	Schema      string           `json:"schema"`
	Rubric      RubricInfo       `json:"rubric"`
	Run         RunInfo          `json:"run"`
	Summary     Summary          `json:"summary"`
	Items       []ItemResult     `json:"items"`
	Findings    []Finding        `json:"findings"`
	Egress      *EgressInfo      `json:"egress,omitempty"`
	Calibration *CalStatus       `json:"calibration,omitempty"`
	Gate        *GateResult      `json:"gate,omitempty"`
	Models      *ModelComparison `json:"models,omitempty"`
	Baseline    *BaselineInfo    `json:"baseline,omitempty"`
	Estimate    *Estimate        `json:"estimate,omitempty"`
}

// NewReport assembles the report. est may be nil.
func NewReport(rb *Rubric, res *Results, est *Estimate) *Report {
	info := RubricInfo{ID: rb.ID, Version: rb.Version, Ref: rb.Ref, Digest: rb.Digest, Formula: Formula, AppliesTo: rb.AppliesTo}
	for _, d := range rb.Dimensions {
		info.Dimensions = append(info.Dimensions, RubricDimInfo{ID: d.ID, Code: d.Code, Group: d.Group, Weight: d.Weight, Severity: d.Severity, Twins: d.Twins})
	}
	rep := &Report{Schema: ReportSchema, Rubric: info, Run: RunInfo{Phase: 0, Offline: true}, Items: res.Items, Estimate: est}
	if rep.Items == nil {
		rep.Items = []ItemResult{}
	}
	rep.Findings = res.Findings(rb)
	if rep.Findings == nil {
		rep.Findings = []Finding{}
	}
	sum, n, semSum, semN := 0, 0, 0, 0
	for _, it := range res.Items {
		rep.Summary.Items++
		switch it.Status {
		case StatusScored:
			rep.Summary.Scored++
			if it.Score != nil {
				sum += *it.Score
				n++
			}
			if it.Semantic != nil && it.Semantic.Score != nil {
				rep.Summary.Judged++
				semSum += *it.Semantic.Score
				semN++
			}
		case StatusWithheld:
			rep.Summary.Withheld++
		case StatusExcluded:
			rep.Summary.Excluded++
		default:
			rep.Summary.Skipped++
		}
	}
	if n > 0 {
		m := int(math.Round(float64(sum) / float64(n)))
		rep.Summary.MeanScore = &m
	}
	if semN > 0 {
		m := int(math.Round(float64(semSum) / float64(semN)))
		rep.Summary.MeanSemanticScore = &m
	}
	return rep
}

// SemanticReport is what a judged run adds to the report.
type SemanticReport struct {
	Outcome *SemanticOutcome
	// Model is the model asked for; Votes the vote cap; Content the content mode.
	Model   string
	Votes   int
	Content string
	// CapUSD and CapCalls are the ceilings the run was held to.
	CapUSD   float64
	CapCalls int
	// Host is where the calls went.
	Host        string
	Calibration *CalStatus
	Gate        *GateResult
	Models      *ModelComparison
	// Estimate is the plan the run was checked against (the egress sizes come from it).
	Estimate *Estimate
}

// WithSemantic records a judged run in the report.
func (r *Report) WithSemantic(sr SemanticReport) {
	o := sr.Outcome
	cap, calls := sr.CapUSD, sr.CapCalls
	r.Run = RunInfo{
		Phase: 1, Calls: o.Usage.Calls, Cached: o.Usage.Cached, Tokens: o.Usage.Tokens, CostUSD: roundUSD(o.Usage.CostUSD),
		CapUSD: &cap, CapCalls: &calls, Incomplete: o.Incomplete, StoppedBecause: o.StoppedBecause, Unjudged: o.Unjudged,
		Model: &RunModel{Requested: sr.Model, Resolved: o.Usage.ResolvedModels()}, Votes: sr.Votes, Content: sr.Content,
		HallucinatedEvidence: o.Usage.Hallucinated,
	}
	r.Calibration, r.Gate, r.Models = sr.Calibration, sr.Gate, sr.Models
	if e := sr.Estimate; e != nil {
		r.Egress = &EgressInfo{Items: e.Totals.Items, Bytes: e.Totals.Bytes, ContentMode: sr.Content, Host: sr.Host, Withheld: e.Withheld}
		for _, it := range r.Items {
			if it.Redacted {
				r.Egress.Redacted = append(r.Egress.Redacted, it.ID)
			}
		}
	}
}

// WriteJSON prints the report as indented JSON.
func (r *Report) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}

// WriteText prints the report for a terminal.
func (r *Report) WriteText(w io.Writer) error {
	var sb strings.Builder
	if r.Run.Offline {
		fmt.Fprintf(&sb, "rubric %s@%d (%s, %s)  offline: lint evidence only, no model call\n", r.Rubric.ID, r.Rubric.Version, r.Rubric.Ref, shortDigest(r.Rubric.Digest))
	} else {
		model := ""
		if r.Run.Model != nil {
			model = r.Run.Model.Requested
			if len(r.Run.Model.Resolved) > 0 {
				model += " (resolved " + strings.Join(r.Run.Model.Resolved, ", ") + ")"
			}
		}
		fmt.Fprintf(&sb, "rubric %s@%d (%s, %s)  model %s  votes<=%d  content %s  advisory\n", r.Rubric.ID, r.Rubric.Version, r.Rubric.Ref, shortDigest(r.Rubric.Digest), model, r.Run.Votes, r.Run.Content)
	}
	hidden := map[string]bool{}
	for _, f := range r.Findings {
		if f.Baselined {
			hidden[f.Fingerprint] = true
		}
	}
	for _, it := range r.Items {
		if it.Status != StatusScored {
			fmt.Fprintf(&sb, "%-28s %-8s %s: %s\n", it.ID, it.Kind, it.Status, it.Reason)
			continue
		}
		score := "n/a"
		if it.Score != nil {
			score = fmt.Sprintf("%d/100", *it.Score)
		}
		fmt.Fprintf(&sb, "%-28s %-8s score %s", it.ID, it.Kind, score)
		if it.Semantic != nil && it.Semantic.Score != nil {
			fmt.Fprintf(&sb, "  semantic %d/100", *it.Semantic.Score)
		}
		if it.Redacted {
			sb.WriteString("  (redacted before sending)")
		}
		sb.WriteString("\n")
		for _, d := range it.Dimensions {
			if d.Status != DimScored || d.Verdict == VerdictPass {
				continue
			}
			fmt.Fprintf(&sb, "  %-6s %-20s %-4s %s\n", d.Code, d.ID, d.Verdict, evidenceSummary(d.Evidence))
		}
		if it.Semantic != nil {
			writeSemanticText(&sb, it, hidden)
		}
	}
	s := r.Summary
	fmt.Fprintf(&sb, "%d items: %d scored, %d withheld, %d excluded, %d skipped", s.Items, s.Scored, s.Withheld, s.Excluded, s.Skipped)
	if s.MeanScore != nil {
		fmt.Fprintf(&sb, "; mean score %d", *s.MeanScore)
	}
	if s.MeanSemanticScore != nil {
		fmt.Fprintf(&sb, "; mean semantic score %d over %d judged", *s.MeanSemanticScore, s.Judged)
	}
	if s.Baselined > 0 {
		fmt.Fprintf(&sb, "; %d finding(s) in the baseline hidden", s.Baselined)
	}
	sb.WriteString("\n")
	if !r.Run.Offline {
		writeRunText(&sb, r)
	}
	if r.Estimate != nil {
		writeEstimateText(&sb, r.Estimate)
	}
	_, err := io.WriteString(w, sb.String())
	if err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}

func writeSemanticText(sb *strings.Builder, it ItemResult, hidden map[string]bool) {
	for _, d := range it.Semantic.Dimensions {
		switch d.Status {
		case SemJudged, SemUnstable:
		case SemError:
			fmt.Fprintf(sb, "  %-6s %-20s error %s\n", d.Code, d.ID, termText(d.Note))
			continue
		default:
			continue
		}
		if d.Verdict == VerdictPass {
			continue
		}
		quote := ""
		if len(d.Evidence) > 0 {
			quote = fmt.Sprintf("%q ", d.Evidence[0].Quote)
		}
		tag := fmt.Sprintf("agree %d/%d", agreeCount(d), len(d.Votes))
		if d.Status == SemUnstable {
			tag = "unstable " + tag
		}
		fp := fingerprint(d.Code, it.ID, d.ID, normSpace(firstQuote(d)))
		if hidden[fp] {
			continue
		}
		fmt.Fprintf(sb, "  %-6s %-20s %-4s %s- %s  [judge, %s]\n", d.Code, d.ID, d.Verdict, quote, termText(d.Rationale), tag)
	}
	if it.Semantic.Truncated {
		sb.WriteString("  AR9G0  truncated: head and tail of the body were sent; verdicts are reported at info\n")
	}
}

// termText makes model- or provider-written text safe for one line of a terminal: line breaks
// and tabs become spaces, other control characters (escape sequences, bells, carriage returns)
// and invisible format characters (bidirectional overrides, zero-width) are dropped.
func termText(s string) string {
	return strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			return ' '
		case unicode.IsControl(r) || unicode.Is(unicode.Cf, r):
			return -1
		}
		return r
	}, s)), " ")
}

func firstQuote(d SemDim) string {
	if len(d.Evidence) > 0 {
		return d.Evidence[0].Quote
	}
	return ""
}

func agreeCount(d SemDim) int {
	n := 0
	for _, v := range d.Votes {
		if v == d.Verdict {
			n++
		}
	}
	return n
}

func writeRunText(sb *strings.Builder, r *Report) {
	ru := r.Run
	capText := "unlimited"
	if ru.CapUSD != nil && *ru.CapUSD > 0 {
		capText = fmt.Sprintf("$%g", *ru.CapUSD)
	}
	fmt.Fprintf(sb, "%d items judged, %d answers cached; calls %d, tokens %d, cost $%.4f (cap %s)\n", r.Summary.Judged, ru.Cached, ru.Calls, ru.Tokens, ru.CostUSD, capText)
	if ru.HallucinatedEvidence > 0 {
		fmt.Fprintf(sb, "%d quote(s) the judge cited were not in the item and were dropped\n", ru.HallucinatedEvidence)
	}
	if ru.Incomplete {
		fmt.Fprintf(sb, "INCOMPLETE: %s", termText(ru.StoppedBecause))
		if len(ru.Unjudged) > 0 {
			fmt.Fprintf(sb, "; not judged: %s", strings.Join(ru.Unjudged, ", "))
		}
		sb.WriteString("\n")
	}
	if c := r.Calibration; c != nil {
		fmt.Fprintf(sb, "calibration: %s", c.State)
		if c.Date != "" {
			fmt.Fprintf(sb, " (%s, %d days old, model %s)", c.Date, c.AgeDays, c.Model)
		}
		for _, why := range c.Reasons {
			fmt.Fprintf(sb, "\n  %s", termText(why))
		}
		sb.WriteString("\n")
	}
	if g := r.Gate; g != nil {
		switch {
		case g.Refused != "":
			fmt.Fprintf(sb, "gate: refused: %s\n", termText(g.Refused))
		case g.Passed:
			fmt.Fprintf(sb, "gate: passed (level %s)\n", g.Level)
		default:
			fmt.Fprintf(sb, "gate: FAILED (level %s)\n", g.Level)
			for _, f := range g.Failures {
				fmt.Fprintf(sb, "  %s %s %s (agreement %.0f%%)\n", f.Code, f.Item, f.Dimension, f.Agreement*100)
			}
		}
	}
	if m := r.Models; m != nil {
		WriteModelsText(sb, m)
	}
}

// WriteModelsText prints how the compared models agree.
func WriteModelsText(sb *strings.Builder, m *ModelComparison) {
	fmt.Fprintf(sb, "models compared: %s\n", strings.Join(m.Models, ", "))
	for _, p := range m.Pairwise {
		fmt.Fprintf(sb, "  %s vs %s  %-20s agreement %.2f  kappa %.2f  (n=%d)\n", p.A, p.B, p.Dimension, p.Agreement, p.Kappa, p.N)
	}
	for _, d := range m.Disagreements {
		var parts []string
		for _, name := range m.Models {
			if v, ok := d.Verdicts[name]; ok {
				parts = append(parts, name+"="+v)
			}
		}
		fmt.Fprintf(sb, "  review first: %s %s: %s\n", d.Item, d.Dimension, strings.Join(parts, " "))
	}
}

func shortDigest(d string) string {
	if len(d) > len("sha256:")+8 {
		return d[:len("sha256:")+8]
	}
	return d
}

func writeEstimateText(sb *strings.Builder, e *Estimate) {
	sb.WriteString("\negress manifest (estimate; nothing is sent by --estimate)\n")
	model := e.Model
	if model == "" {
		model = "(no model configured)"
	}
	fmt.Fprintf(sb, "  model %s  host %s  network allowed: %t  content: %s\n", model, e.Host, e.NetworkAllowed, e.ContentMode)
	if len(e.IgnoredLLMKeys) > 0 {
		fmt.Fprintf(sb, "  ignored repository [llm] keys (user scope only): %s\n", strings.Join(e.IgnoredLLMKeys, ", "))
	}
	for _, it := range e.Items {
		fmt.Fprintf(sb, "  %-28s %d call(s) %6d bytes  sha256 %s", it.ID, len(it.Calls), it.Bytes, shortHex(it.SHA256))
		if it.Truncated {
			sb.WriteString("  truncated")
		}
		sb.WriteString("\n")
	}
	if len(e.Withheld) > 0 {
		fmt.Fprintf(sb, "  withheld (never sent): %s\n", strings.Join(e.Withheld, ", "))
	}
	t := e.Totals
	fmt.Fprintf(sb, "  %d items, %d-%d calls, ~%d-%d input tokens, up to %d output tokens\n", t.Items, t.CallsMin, t.CallsMax, t.InputTokensMin, t.InputTokensMax, t.OutputTokensMax)
	if e.CostKnown {
		fmt.Fprintf(sb, "  cost $%.4f to $%.4f (cap %s, %s)\n", *e.CostMinUSD, *e.CostMaxUSD, e.Cap.costText(), e.Cap.callsText())
	} else {
		fmt.Fprintf(sb, "  cost unknown (cap %s, %s)\n", e.Cap.costText(), e.Cap.callsText())
	}
	for _, r := range e.Refused {
		fmt.Fprintf(sb, "  refused: %s\n", r)
	}
	for _, p := range e.Prompts {
		fmt.Fprintf(sb, "\n--- %s %s ---\n[system]\n%s\n[user]\n%s", p.Item, p.Group, p.System, p.User)
	}
}

func (c Cap) costText() string {
	if c.MaxCostUSD == 0 {
		return "unlimited"
	}
	return fmt.Sprintf("$%g", c.MaxCostUSD)
}

func (c Cap) callsText() string {
	if c.MaxCalls == 0 {
		return "unlimited calls"
	}
	return fmt.Sprintf("%d calls", c.MaxCalls)
}

func shortHex(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}

// SARIF 2.1.0 output. Rule ids are the AR9G dimension codes; results are
// advisory (origin "lint-twin": derived from deterministic lint evidence).

type sarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool       sarifTool      `json:"tool"`
	Results    []sarifResult  `json:"results"`
	Properties map[string]any `json:"properties,omitempty"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	Version        string      `json:"version,omitempty"`
	InformationURI string      `json:"informationUri"`
	Rules          []sarifRule `json:"rules"`
}

type sarifText struct {
	Text string `json:"text"`
}

type sarifRule struct {
	ID                   string         `json:"id"`
	Name                 string         `json:"name"`
	ShortDescription     sarifText      `json:"shortDescription"`
	DefaultConfiguration map[string]any `json:"defaultConfiguration"`
}

type sarifResult struct {
	RuleID              string            `json:"ruleId"`
	RuleIndex           int               `json:"ruleIndex"`
	Level               string            `json:"level"`
	Message             sarifText         `json:"message"`
	Locations           []sarifLocation   `json:"locations"`
	PartialFingerprints map[string]string `json:"partialFingerprints"`
	Properties          map[string]any    `json:"properties"`
}

type sarifLocation struct {
	PhysicalLocation struct {
		ArtifactLocation struct {
			URI       string `json:"uri"`
			URIBaseID string `json:"uriBaseId,omitempty"`
		} `json:"artifactLocation"`
		Region struct {
			StartLine int `json:"startLine"`
		} `json:"region"`
	} `json:"physicalLocation"`
}

// WriteSARIF prints the findings as SARIF 2.1.0. version is the ai-rulez release.
func WriteSARIF(w io.Writer, rb *Rubric, res *Results, version string) error {
	var findings []Finding
	for _, f := range res.Findings(rb) {
		if !f.Baselined {
			findings = append(findings, f)
		}
	}
	phase, offline := 0, true
	for i := range res.Items {
		if res.Items[i].Semantic != nil {
			phase, offline = 1, false
		}
	}
	index := map[string]int{}
	rules := []sarifRule{}
	for _, f := range findings {
		if _, ok := index[f.Code]; ok {
			continue
		}
		index[f.Code] = len(rules)
		name := f.Name
		if info, ok := ruleName(f.Code); ok {
			name = info
		}
		rules = append(rules, sarifRule{
			ID: f.Code, Name: name, ShortDescription: sarifText{Text: name},
			DefaultConfiguration: map[string]any{"level": sarifout.Level(f.Severity)},
		})
	}
	results := make([]sarifResult, 0, len(findings))
	for _, f := range findings {
		var loc sarifLocation
		loc.PhysicalLocation.ArtifactLocation.URI, loc.PhysicalLocation.ArtifactLocation.URIBaseID = sarifout.ArtifactURI(f.Path)
		loc.PhysicalLocation.Region.StartLine = max(f.Line, 1)
		results = append(results, sarifResult{
			RuleID: f.Code, RuleIndex: index[f.Code], Level: sarifout.Level(f.Severity),
			Message: sarifText{Text: f.Message}, Locations: []sarifLocation{loc},
			PartialFingerprints: map[string]string{"aiRulezReviewFingerprint/v1": f.Fingerprint},
			Properties:          resultProperties(f, rb),
		})
	}
	log := sarifLog{Schema: sarifout.Schema, Version: sarifout.Version, Runs: []sarifRun{{
		Tool:       sarifTool{Driver: sarifDriver{Name: "ai-rulez", Version: version, InformationURI: sarifout.InformURI, Rules: rules}},
		Results:    results,
		Properties: map[string]any{"rubricDigest": rb.Digest, "phase": phase, "offline": offline},
	}}}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(log); err != nil {
		return fmt.Errorf("write sarif: %w", err)
	}
	return nil
}

// resultProperties are the SARIF properties of a finding: always advisory, with the origin
// (lint-twin or llm-judge) and, for a judged finding, the vote agreement.
func resultProperties(f Finding, rb *Rubric) map[string]any {
	origin := f.Origin
	if origin == "" {
		origin = OriginLintTwin
	}
	props := map[string]any{"advisory": true, "origin": origin, "item": f.ItemID, "rubric": rb.ID + "@" + fmt.Sprint(rb.Version)}
	if f.Origin == OriginLLMJudge {
		props["agreement"] = f.Agreement
		props["status"] = f.Status
	}
	return props
}

func ruleName(code string) (string, bool) {
	for _, r := range lint.Rules() {
		if r.Code == code {
			return r.Name, true
		}
	}
	return "", false
}
