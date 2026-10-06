package review

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"

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

// Run records what the run did; phase 0 never calls a model.
type RunInfo struct {
	Phase   int     `json:"phase"`
	Offline bool    `json:"offline"`
	Calls   int     `json:"calls"`
	Tokens  int     `json:"tokens"`
	CostUSD float64 `json:"cost_usd"`
}

// Summary counts the items by status.
type Summary struct {
	Items     int  `json:"items"`
	Scored    int  `json:"scored"`
	Withheld  int  `json:"withheld"`
	Excluded  int  `json:"excluded"`
	Skipped   int  `json:"skipped"`
	MeanScore *int `json:"mean_score"`
}

// Report is the machine-readable result of `ai-rulez review`.
type Report struct {
	Schema   string       `json:"schema"`
	Rubric   RubricInfo   `json:"rubric"`
	Run      RunInfo      `json:"run"`
	Summary  Summary      `json:"summary"`
	Items    []ItemResult `json:"items"`
	Estimate *Estimate    `json:"estimate,omitempty"`
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
	sum, n := 0, 0
	for _, it := range res.Items {
		rep.Summary.Items++
		switch it.Status {
		case StatusScored:
			rep.Summary.Scored++
			if it.Score != nil {
				sum += *it.Score
				n++
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
	return rep
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
	fmt.Fprintf(&sb, "rubric %s@%d (%s, %s)  offline: lint evidence only, no model call\n", r.Rubric.ID, r.Rubric.Version, r.Rubric.Ref, shortDigest(r.Rubric.Digest))
	for _, it := range r.Items {
		if it.Status != StatusScored {
			fmt.Fprintf(&sb, "%-28s %-8s %s: %s\n", it.ID, it.Kind, it.Status, it.Reason)
			continue
		}
		score := "n/a"
		if it.Score != nil {
			score = fmt.Sprintf("%d/100", *it.Score)
		}
		fmt.Fprintf(&sb, "%-28s %-8s score %s\n", it.ID, it.Kind, score)
		for _, d := range it.Dimensions {
			if d.Status != DimScored || d.Verdict == VerdictPass {
				continue
			}
			fmt.Fprintf(&sb, "  %-6s %-20s %-4s %s\n", d.Code, d.ID, d.Verdict, evidenceSummary(d.Evidence))
		}
	}
	s := r.Summary
	fmt.Fprintf(&sb, "%d items: %d scored, %d withheld, %d excluded, %d skipped", s.Items, s.Scored, s.Withheld, s.Excluded, s.Skipped)
	if s.MeanScore != nil {
		fmt.Fprintf(&sb, "; mean score %d", *s.MeanScore)
	}
	sb.WriteString("\n")
	if r.Estimate != nil {
		writeEstimateText(&sb, r.Estimate)
	}
	_, err := io.WriteString(w, sb.String())
	if err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}

func shortDigest(d string) string {
	if len(d) > len("sha256:")+8 {
		return d[:len("sha256:")+8]
	}
	return d
}

func writeEstimateText(sb *strings.Builder, e *Estimate) {
	sb.WriteString("\negress manifest (estimate; nothing is sent in this build)\n")
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
	findings := res.Findings(rb)
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
			Properties:          map[string]any{"advisory": true, "origin": "lint-twin", "item": f.ItemID, "rubric": rb.ID + "@" + fmt.Sprint(rb.Version)},
		})
	}
	log := sarifLog{Schema: sarifout.Schema, Version: sarifout.Version, Runs: []sarifRun{{
		Tool:       sarifTool{Driver: sarifDriver{Name: "ai-rulez", Version: version, InformationURI: sarifout.InformURI, Rules: rules}},
		Results:    results,
		Properties: map[string]any{"rubricDigest": rb.Digest, "phase": 0, "offline": true},
	}}}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(log); err != nil {
		return fmt.Errorf("write sarif: %w", err)
	}
	return nil
}

func ruleName(code string) (string, bool) {
	for _, r := range lint.Rules() {
		if r.Code == code {
			return r.Name, true
		}
	}
	return "", false
}
