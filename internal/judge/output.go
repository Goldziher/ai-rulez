package judge

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/sarifout"
)

// Sanitized returns a copy of the report whose strings carry no control character or secret-looking
// text: a file name, a provider message or a model's wording can hold a terminal escape.
func (r *Report) Sanitized() *Report {
	c := *r
	c.Run.Models = map[string]string{}
	for k, v := range r.Run.Models {
		c.Run.Models[k] = SafeText(v)
	}
	c.Run.EgressHosts = safeList(r.Run.EgressHosts)
	c.Run.Notices = safeList(r.Run.Notices)
	c.Run.FailOn = safeList(r.Run.FailOn)
	c.Units = make([]UnitReport, len(r.Units))
	for i := range r.Units {
		u := r.Units[i]
		u.Path, u.Reason = SafeText(u.Path), SafeText(u.Reason)
		u.Gaps, u.Notes = safeList(u.Gaps), safeList(u.Notes)
		u.Files = safeList(u.Files)
		u.Findings = sanitizeFindings(u.Findings)
		u.Suppressed = sanitizeFindings(u.Suppressed)
		c.Units[i] = u
	}
	return &c
}

func sanitizeFindings(in []*Finding) []*Finding {
	if in == nil {
		return nil
	}
	out := make([]*Finding, len(in))
	for i, f := range in {
		out[i] = sanitizeFinding(f)
	}
	return out
}

func sanitizeFinding(f *Finding) *Finding {
	cp := *f
	cp.Location.Path = SafeText(cp.Location.Path)
	cp.Title, cp.Quote = SafeText(cp.Title), SafeText(cp.Quote)
	if cp.SuggestedFix != nil {
		fix := SafeText(*cp.SuggestedFix)
		cp.SuggestedFix = &fix
	}
	return &cp
}

// fprintf writes to a report destination; a failed write surfaces when the caller flushes or closes it.
func fprintf(w io.Writer, format string, args ...any) {
	if _, err := fmt.Fprintf(w, format, args...); err != nil {
		return
	}
}

func safeList(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = SafeText(s)
	}
	return out
}

// WriteText prints the report for people: one block per unit that has anything to say, then the
// totals. A passing unit is one line.
func (r *Report) WriteText(w io.Writer) {
	r = r.Sanitized()
	if r.Mode == ModeEstimate {
		r.writeEstimateText(w)
		return
	}
	for i := range r.Units {
		writeUnitText(w, &r.Units[i])
	}
	s := r.Summary
	p := r.Run.Provenance
	cov := r.Run.Coverage
	fprintf(w, "\njudged %d units (%d complete, %d partial, %d withheld, %d skipped): %s\n", cov.UnitsTotal, cov.UnitsFullyReviewed, cov.UnitsPartial, cov.UnitsWithheld, cov.UnitsSkipped, strings.ToUpper(s.Decision))
	fprintf(w, "findings: %s; %d calls (%d cached), %d tokens, ~$%.4f\n", countsLine(s), p.Calls, p.CacheHits, p.InputTokens+p.OutputTokens, p.CostUSD)
	g := s.Grounding
	if g.Candidates > 0 {
		fprintf(w, "grounding: %d candidates, %d dropped as ungrounded (%d repaired), %d dropped by the adjudicator\n", g.Candidates, g.DroppedUngrounded, g.Repaired, g.DroppedByAdjud)
	}
	for _, n := range r.Run.Notices {
		fprintf(w, "note: %s\n", n)
	}
}

func countsLine(s Summary) string {
	var parts []string
	for _, sev := range []Severity{SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow, SeverityInfo} {
		if n := s.BySeverity[string(sev)]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, sev))
		}
	}
	if len(parts) == 0 {
		return string(SeverityNone)
	}
	return strings.Join(parts, ", ")
}

func scoreText(p *int) string {
	if p == nil {
		return "-"
	}
	return itoa(*p)
}

func writeUnitText(w io.Writer, u *UnitReport) {
	label := strings.ToUpper(strings.ReplaceAll(u.Status, "_", " "))
	switch u.Status {
	case StatusWithheld, StatusSkipped, StatusError:
		fprintf(w, "%-8s %s (%s)  %s\n", label, u.Path, u.Kind, u.Reason)
		return
	}
	fprintf(w, "%-8s %s (%s)  security %s  governance %s  quality %s\n", label, u.Path, u.Kind, scoreText(u.Scores.Security), scoreText(u.Scores.Governance), scoreText(u.Scores.Quality))
	for _, f := range u.Findings {
		fprintf(w, "  %-8s %s %s  %s:%d  %s\n", f.Severity, f.Code, f.Confidence, f.Location.Path, f.Location.StartLine, f.Title)
		fprintf(w, "           > %s\n", f.Quote)
		if f.Why != "" {
			fprintf(w, "           why: %s\n", f.Why)
		}
		if f.Convention != nil {
			fprintf(w, "           convention: %s L%d: %s\n", f.Convention.ID, f.Convention.Line, f.Convention.Quote)
		}
		if f.SuggestedFix != nil && *f.SuggestedFix != "" {
			fprintf(w, "           fix: %s\n", *f.SuggestedFix)
		}
	}
	for _, c := range u.Criteria {
		if c.Result != CriterionPass {
			fprintf(w, "  criterion %s: %s %s\n", c.RubricID, c.Result, c.Note)
		}
	}
	for _, g := range u.Gaps {
		fprintf(w, "  coverage gap: %s\n", g)
	}
	for _, n := range u.Notes {
		fprintf(w, "  note: %s\n", n)
	}
	for i := range u.Dropped {
		d := &u.Dropped[i]
		fprintf(w, "  dropped (%s) %s %s %s:%d  %s %s\n", d.Stage, d.Code, d.ID, d.Location.Path, d.Location.StartLine, d.ReasonCode, strings.Join(d.Problems, "; "))
	}
}

func (r *Report) writeEstimateText(w io.Writer) {
	fprintf(w, "judge estimate (nothing was sent)\n")
	fprintf(w, "categories: %s\n", joinCats(r.Run.Categories))
	for _, role := range []string{"security", "governance", "quality", roleAdjudicator} {
		m := r.Run.Models[role]
		if m == "" {
			m = "(none: pass --model or set AI_RULEZ_LLM_MODEL)"
		}
		fprintf(w, "model:      %-11s %s\n", role, m)
	}
	host := strings.Join(r.Run.EgressHosts, ", ")
	if host == "" {
		host = "(the provider's default endpoint)"
	}
	fprintf(w, "egress:     %s\n", host)
	for _, s := range r.Run.LoadedSkills {
		fprintf(w, "rubric:     %s (%s, %s)\n", s.Name, s.Tier, strings.Join(s.Categories, ","))
	}
	for i := range r.Units {
		u := &r.Units[i]
		switch u.Status {
		case StatusWouldSend:
			att := ""
			if u.Attached != nil {
				att = fmt.Sprintf("  +%d rules, %d instruction files", len(u.Attached.Rules), len(u.Attached.InstructionFiles))
			}
			fprintf(w, "  send  %s (%s, %d files)%s\n", u.Path, u.Kind, len(u.Files), att)
		default:
			fprintf(w, "  skip  %s (%s)  %s\n", u.Path, u.Kind, u.Reason)
		}
	}
	e := r.Run.Estimate
	cost := "unknown (no price for a model; pass --price-input and --price-output)"
	if e.CostKnown {
		cost = fmt.Sprintf("<= $%.4f (expected ~$%.4f)", e.CostUSD, e.ExpectedCostUSD)
	}
	calls := itoa(e.Calls)
	if e.CallsFit < e.Calls {
		calls += " planned, " + itoa(e.CallsFit) + " fit under the caps"
	}
	fprintf(w, "units to send: %d, calls: %s, prompt tokens: ~%d, max completion tokens: %d\nworst-case cost: %s\n", e.Units, calls, e.PromptTokens, e.MaxCompletionTokens, cost)
	for _, n := range r.Run.Notices {
		fprintf(w, "note: %s\n", n)
	}
}

func joinCats(cs []Category) string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = string(c)
	}
	return strings.Join(out, ", ")
}

// SARIF 2.1.0 ---------------------------------------------------------------------------------

type sarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool        sarifTool         `json:"tool"`
	Invocations []sarifInvocation `json:"invocations"`
	Results     []sarifResult     `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	InformationURI string      `json:"informationUri"`
	Rules          []sarifRule `json:"rules"`
}

type sarifRule struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	ShortDescription sarifText      `json:"shortDescription"`
	Properties       map[string]any `json:"properties,omitempty"`
}

type sarifText struct {
	Text string `json:"text"`
}

type sarifInvocation struct {
	ExecutionSuccessful        bool                `json:"executionSuccessful"`
	ToolExecutionNotifications []sarifNotification `json:"toolExecutionNotifications,omitempty"`
}

type sarifNotification struct {
	Level   string    `json:"level"`
	Message sarifText `json:"message"`
}

type sarifResult struct {
	RuleID              string             `json:"ruleId"`
	Level               string             `json:"level"`
	Message             sarifText          `json:"message"`
	Locations           []sarifLocation    `json:"locations"`
	RelatedLocations    []sarifLocation    `json:"relatedLocations,omitempty"`
	CodeFlows           []sarifCodeFlow    `json:"codeFlows,omitempty"`
	PartialFingerprints map[string]string  `json:"partialFingerprints"`
	Suppressions        []sarifSuppression `json:"suppressions,omitempty"`
	Properties          map[string]any     `json:"properties"`
}

type sarifSuppression struct {
	Kind          string `json:"kind"`
	Justification string `json:"justification"`
}

type sarifLocation struct {
	ID               int            `json:"id,omitempty"`
	PhysicalLocation sarifPhysical  `json:"physicalLocation"`
	Message          *sarifText     `json:"message,omitempty"`
	Properties       map[string]any `json:"properties,omitempty"`
}

type sarifPhysical struct {
	ArtifactLocation sarifArtifact `json:"artifactLocation"`
	Region           *sarifRegion  `json:"region,omitempty"`
}

type sarifArtifact struct {
	URI       string `json:"uri"`
	URIBaseID string `json:"uriBaseId,omitempty"`
}

type sarifRegion struct {
	StartLine int        `json:"startLine"`
	EndLine   int        `json:"endLine,omitempty"`
	Snippet   *sarifText `json:"snippet,omitempty"`
}

type sarifCodeFlow struct {
	ThreadFlows []sarifThreadFlow `json:"threadFlows"`
}

type sarifThreadFlow struct {
	Locations []sarifThreadFlowLocation `json:"locations"`
}

type sarifThreadFlowLocation struct {
	Location sarifLocation `json:"location"`
}

func sarifLevel(s Severity) string {
	switch s {
	case SeverityCritical, SeverityHigh:
		return sarifout.LevelError
	case SeverityMedium:
		return sarifout.LevelWarn
	}
	return sarifout.LevelNote
}

func securitySeverity(s Severity) string {
	switch s {
	case SeverityCritical:
		return "9.5"
	case SeverityHigh:
		return "8.0"
	case SeverityMedium:
		return "5.5"
	case SeverityLow:
		return "3.0"
	}
	return "1.0"
}

func sarifLoc(l Location, quote string, id int) sarifLocation {
	uri, base := sarifout.ArtifactURI(l.Path)
	reg := &sarifRegion{StartLine: max(l.StartLine, 1), EndLine: max(l.EndLine, 1)}
	if quote != "" {
		reg.Snippet = &sarifText{Text: quote}
	}
	return sarifLocation{ID: id, PhysicalLocation: sarifPhysical{ArtifactLocation: sarifArtifact{URI: uri, URIBaseID: base}, Region: reg}}
}

// WriteSARIF writes the report as SARIF 2.1.0: one result per finding, coverage gaps as tool
// execution notifications, and executionSuccessful false when the run was incomplete.
func (r *Report) WriteSARIF(w io.Writer) error {
	r = r.Sanitized()
	run := sarifRun{Tool: sarifTool{Driver: sarifDriver{Name: "ai-rulez judge", InformationURI: sarifout.InformURI}}}
	ruleSeen := map[string]bool{}
	var results []sarifResult
	inv := sarifInvocation{ExecutionSuccessful: r.Summary.Decision != DecisionIncomplete}
	for _, n := range r.Run.Notices {
		inv.ToolExecutionNotifications = append(inv.ToolExecutionNotifications, sarifNotification{Level: sarifout.LevelNote, Message: sarifText{Text: n}})
	}
	for ui := range r.Units {
		u := &r.Units[ui]
		for _, f := range u.Findings {
			if !ruleSeen[f.Code] {
				ruleSeen[f.Code] = true
				info, _ := LookupCode(f.Code)
				run.Tool.Driver.Rules = append(run.Tool.Driver.Rules, sarifRule{
					ID: "ai-rulez/" + f.Code, Name: info.Name, ShortDescription: sarifText{Text: info.Summary},
					Properties: map[string]any{keyCategory: string(info.Category)},
				})
			}
			results = append(results, sarifFinding(f))
		}
		for _, g := range u.Gaps {
			inv.ToolExecutionNotifications = append(inv.ToolExecutionNotifications, sarifNotification{Level: sarifout.LevelWarn, Message: sarifText{Text: u.Path + ": " + g}})
		}
		if u.Status == StatusWithheld {
			inv.ToolExecutionNotifications = append(inv.ToolExecutionNotifications, sarifNotification{Level: sarifout.LevelError, Message: sarifText{Text: u.Path + ": " + u.Reason}})
		}
	}
	sort.SliceStable(run.Tool.Driver.Rules, func(i, j int) bool { return run.Tool.Driver.Rules[i].ID < run.Tool.Driver.Rules[j].ID })
	if run.Tool.Driver.Rules == nil {
		run.Tool.Driver.Rules = []sarifRule{}
	}
	if results == nil {
		results = []sarifResult{}
	}
	run.Results = results
	run.Invocations = []sarifInvocation{inv}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(sarifLog{Schema: sarifout.Schema, Version: sarifout.Version, Runs: []sarifRun{run}}) //nolint:wrapcheck // writer error
}

func sarifFinding(f *Finding) sarifResult {
	res := sarifResult{
		RuleID:    "ai-rulez/" + f.Code,
		Level:     sarifLevel(f.Severity),
		Message:   sarifText{Text: f.Title + ": " + firstNonEmptyStr(f.Why, f.Quote)},
		Locations: []sarifLocation{sarifLoc(f.Location, f.Quote, 0)},
		PartialFingerprints: map[string]string{
			"aiRulezJudge/v1": f.Fingerprint,
		},
		Properties: map[string]any{
			"security-severity": securitySeverity(f.Severity), keyCategory: string(f.Category), "severity": string(f.Severity),
			"confidence": string(f.Confidence), "verdict": string(f.Verdict), "origin": f.Origin, "id": f.ID,
		},
	}
	if len(f.Chain) > 0 {
		tf := sarifThreadFlow{}
		for i, s := range f.Chain {
			loc := sarifLoc(s.Location, s.Quote, i)
			loc.Message = &sarifText{Text: s.Role}
			tf.Locations = append(tf.Locations, sarifThreadFlowLocation{Location: loc})
		}
		res.CodeFlows = []sarifCodeFlow{{ThreadFlows: []sarifThreadFlow{tf}}}
	}
	if f.Convention != nil {
		res.RelatedLocations = append(res.RelatedLocations, sarifLocation{
			ID: 1,
			PhysicalLocation: sarifPhysical{
				ArtifactLocation: sarifArtifact{URI: strings.TrimPrefix(strings.TrimPrefix(f.Convention.ID, "rule:"), "claude-md:"), URIBaseID: sarifout.SrcRoot},
				Region:           &sarifRegion{StartLine: max(f.Convention.Line, 1), Snippet: &sarifText{Text: f.Convention.Quote}},
			},
			Message: &sarifText{Text: "convention"},
		})
	}
	// A SARIF fix needs the replacement text; the suggestion is prose, so it travels in the message
	// and properties instead of a fix that a tool could apply as a deletion.
	if f.SuggestedFix != nil && *f.SuggestedFix != "" {
		res.Message.Text += "\nSuggested fix: " + *f.SuggestedFix
		res.Properties["suggestedFix"] = *f.SuggestedFix
	}
	return res
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
