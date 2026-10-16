package judgebench

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Report is the part of a judge report (`ai-rulez judge --format json --explain-drops`) the scoring reads.
//
//nolint:tagliatelle // the report keys are snake_case
type Report struct {
	Run struct {
		Provenance struct {
			Calls        int     `json:"calls"`
			CacheHits    int     `json:"cache_hits"`
			InputTokens  int     `json:"input_tokens"`
			OutputTokens int     `json:"output_tokens"`
			CostUSD      float64 `json:"cost_usd"`
		} `json:"provenance"`
		Models   map[string]string `json:"models"`
		Estimate struct {
			Units           int     `json:"units"`
			Calls           int     `json:"calls"`
			CostUSD         float64 `json:"cost_usd"`
			ExpectedCostUSD float64 `json:"expected_cost_usd"`
		} `json:"estimate"`
	} `json:"run"`
	Units   []ReportUnit `json:"units"`
	Summary struct {
		Decision  string `json:"decision"`
		Grounding struct {
			Candidates         int `json:"candidates"`
			QuoteVerified      int `json:"quote_verified"`
			DroppedUngrounded  int `json:"dropped_ungrounded"`
			DroppedByAdjud     int `json:"dropped_by_adjudicator"`
			Repaired           int `json:"repaired"`
			BelowMinConfidence int `json:"below_min_confidence"`
		} `json:"grounding"`
	} `json:"summary"`
}

// ReportUnit is one unit of the report.
//
//nolint:tagliatelle // the report keys are snake_case
type ReportUnit struct {
	Path     string    `json:"path"`
	Status   string    `json:"status"`
	Complete bool      `json:"complete"`
	Findings []Finding `json:"findings"`
	Dropped  []Dropped `json:"dropped"`
	Criteria []struct {
		RubricID string `json:"rubric_id"`
		Result   string `json:"result"`
	} `json:"criteria_results"`
}

// Finding is a reported finding.
type Finding struct {
	ID       string `json:"id"`
	Code     string `json:"code"`
	Category string `json:"category"`
	Severity string `json:"severity"`
	Quote    string `json:"quote"`
	Location struct {
		Path      string `json:"path"`
		StartLine int    `json:"start_line"`
	} `json:"location"`
	Chain []struct {
		Location struct {
			Path string `json:"path"`
		} `json:"location"`
		Quote string `json:"quote"`
	} `json:"chain"`
}

// Dropped is a candidate that did not become a finding.
type Dropped struct {
	Code     string `json:"code"`
	Category string `json:"category"`
	Quote    string `json:"quote"`
	Stage    string `json:"stage"`
	Location struct {
		Path string `json:"path"`
	} `json:"location"`
}

// Counts is a confusion count.
type Counts struct {
	TP, FP, FN int
	// Localized counts true positives whose finding sits in the expected file and quotes the expected text.
	Localized int
}

// Precision is TP/(TP+FP); 1 when nothing was reported.
func (c Counts) Precision() float64 {
	if c.TP+c.FP == 0 {
		return 1
	}
	return float64(c.TP) / float64(c.TP+c.FP)
}

// Recall is TP/(TP+FN); 1 when nothing was expected.
func (c Counts) Recall() float64 {
	if c.TP+c.FN == 0 {
		return 1
	}
	return float64(c.TP) / float64(c.TP+c.FN)
}

// F1 is the harmonic mean of precision and recall.
func (c Counts) F1() float64 {
	p, r := c.Precision(), c.Recall()
	if p+r == 0 {
		return 0
	}
	return 2 * p * r / (p + r)
}

// Metrics are the results of scoring one report.
type Metrics struct {
	Units      int
	ByCode     map[string]*Counts
	ByCategory map[string]*Counts
	// TrapFalsePositiveRate is the share of trap units (benign lookalikes of a known false-positive
	// shape) with at least one security finding.
	TrapUnits, TrapFalsePositives int
	// TwinUnits and TwinsFound count the malicious twins of the traps and how many were found.
	TwinUnits, TwinsFound int
	// CleanUnits are units with no defect in any category; CleanFindings counts the findings on them.
	CleanUnits, CleanFindings int
	// FalsePositivesPer100 is every false positive over all labeled units, per 100 units.
	FalsePositivesPer100 float64
	// SeverityOK counts true positives that met their minimum severity, SeverityChecked all with one.
	SeverityOK, SeverityChecked int
	// PrecisionBeforeAdjudicator and PrecisionAfter measure the adjudicator's lift on security.
	BeforeAdjudicator, AfterAdjudicator Counts
	// Withheld counts the expected secret units that were withheld.
	WithheldExpected, WithheldGot int
	// Missing lists the units of the manifest the report has no unit for.
	Missing []string
	// Partial lists reported units that were not completely reviewed.
	Partial []string
	// Grounding, cost and size of the run.
	UngroundedRate float64
	Candidates     int
	Repaired       int
	CostUSD        float64
	Calls          int
	CacheHits      int
	CostPerUnit    float64
	Decision       string
	Models         map[string]string
}

func rank(sev string) int {
	switch sev {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	}
	return 0
}

func normalize(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }

// The finding categories of the judge report.
const (
	catSecurity   = "security"
	catQuality    = "quality"
	catGovernance = "governance"
)

func category(code string) string {
	switch {
	case strings.HasPrefix(code, "JR-S"):
		return catSecurity
	case strings.HasPrefix(code, "JR-Q"):
		return catQuality
	case strings.HasPrefix(code, "JR-G"):
		return catGovernance
	}
	return ""
}

// unitFor returns the report units that belong to a manifest unit: the unit it names or those below it.
func unitFor(rep *Report, u Unit) []*ReportUnit {
	var out []*ReportUnit
	p := u.Path()
	for i := range rep.Units {
		ru := &rep.Units[i]
		if ru.Path == p || strings.HasPrefix(ru.Path, p+"/") {
			out = append(out, ru)
		}
	}
	return out
}

// Score compares a report with the labels. Units of the manifest the report does not cover are
// listed in Missing and count as misses for their expected findings.
func Score(m Manifest, rep *Report, only map[string]bool) Metrics {
	mt := Metrics{ByCode: map[string]*Counts{}, ByCategory: map[string]*Counts{}, Models: rep.Run.Models, Decision: rep.Summary.Decision}
	count := func(code string) {
		if mt.ByCode[code] == nil {
			mt.ByCode[code] = &Counts{}
		}
		if mt.ByCategory[category(code)] == nil {
			mt.ByCategory[category(code)] = &Counts{}
		}
	}
	bump := func(code string, f func(*Counts)) {
		count(code)
		f(mt.ByCode[code])
		f(mt.ByCategory[category(code)])
	}
	fp := 0
	for i := range m.Units {
		u := &m.Units[i]
		if only != nil && !only[u.ID] {
			continue
		}
		mt.Units++
		rus := unitFor(rep, *u)
		if len(rus) == 0 {
			mt.Missing = append(mt.Missing, u.ID)
		}
		var findings []Finding
		var dropped []Dropped
		for _, ru := range rus {
			findings = append(findings, ru.Findings...)
			dropped = append(dropped, ru.Dropped...)
			if !ru.Complete && ru.Status != "withheld_secret" {
				mt.Partial = append(mt.Partial, ru.Path)
			}
		}
		scoreUnit(&mt, *u, rus, findings, dropped, bump, &fp)
	}
	if mt.Units > 0 {
		mt.FalsePositivesPer100 = float64(fp) * 100 / float64(mt.Units)
	}
	g := rep.Summary.Grounding
	mt.Candidates, mt.Repaired = g.Candidates, g.Repaired
	if g.Candidates > 0 {
		mt.UngroundedRate = float64(g.DroppedUngrounded) / float64(g.Candidates)
	}
	p := rep.Run.Provenance
	mt.CostUSD, mt.Calls, mt.CacheHits = p.CostUSD, p.Calls, p.CacheHits
	if mt.Units > 0 {
		mt.CostPerUnit = p.CostUSD / float64(mt.Units)
	}
	sort.Strings(mt.Partial)
	return mt
}

func scoreUnit(mt *Metrics, u Unit, rus []*ReportUnit, findings []Finding, dropped []Dropped, bump func(string, func(*Counts)), fp *int) {
	matched, hits := scoreExpectations(mt, u, rus, findings, bump)
	scoreUnexpected(mt, u, findings, matched, bump, fp)
	scoreGroup(mt, u, findings, hits)
	scoreAdjudicator(mt, u, findings, dropped)
}

// scoreExpectations matches each expected finding of the unit and returns the findings that satisfied
// one and the number of expectations that were met.
func scoreExpectations(mt *Metrics, u Unit, rus []*ReportUnit, findings []Finding, bump func(string, func(*Counts))) (matched map[int]bool, hits int) {
	matched = map[int]bool{}
	for _, e := range u.Expect {
		if u.ExpectStatus != "" {
			if scoreWithheld(mt, e, u.ExpectStatus, rus, bump) {
				hits++
			}
			continue
		}
		idx := firstUnmatched(findings, e.Code, matched)
		if idx < 0 {
			bump(e.Code, func(c *Counts) { c.FN++ })
			continue
		}
		matched[idx] = true
		hits++
		scoreMatch(mt, &findings[idx], e, bump)
	}
	return matched, hits
}

// scoreWithheld counts an expectation about a unit's status (a withheld secret) and reports whether it was met.
func scoreWithheld(mt *Metrics, e Expect, status string, rus []*ReportUnit, bump func(string, func(*Counts))) bool {
	mt.WithheldExpected++
	got := false
	for _, ru := range rus {
		got = got || ru.Status == status
	}
	if !got {
		bump(e.Code, func(c *Counts) { c.FN++ })
		return false
	}
	mt.WithheldGot++
	bump(e.Code, func(c *Counts) { c.TP++; c.Localized++ })
	return true
}

func firstUnmatched(findings []Finding, code string, matched map[int]bool) int {
	for i := range findings {
		if findings[i].Code == code && !matched[i] {
			return i
		}
	}
	return -1
}

func scoreMatch(mt *Metrics, f *Finding, e Expect, bump func(string, func(*Counts))) {
	local := locates(*f, e)
	bump(e.Code, func(c *Counts) {
		c.TP++
		if local {
			c.Localized++
		}
	})
	if e.MinSeverity != "" {
		mt.SeverityChecked++
		if rank(f.Severity) >= rank(e.MinSeverity) {
			mt.SeverityOK++
		}
	}
}

// scoreUnexpected counts the findings no expectation claimed, in a category the unit is clean for, as false positives.
func scoreUnexpected(mt *Metrics, u Unit, findings []Finding, matched map[int]bool, bump func(string, func(*Counts)), fp *int) {
	clean := map[string]bool{}
	for _, c := range u.CleanCategories {
		clean[c] = true
	}
	for i := range findings {
		f := &findings[i]
		if matched[i] || !clean[f.Category] {
			continue
		}
		bump(f.Code, func(c *Counts) { c.FP++ })
		*fp++
		if u.Group == GroupClean || u.Group == GroupQBenign || u.Group == GroupGBenign {
			mt.CleanFindings++
		}
	}
}

func scoreGroup(mt *Metrics, u Unit, findings []Finding, hits int) {
	switch u.Group {
	case GroupClean:
		mt.CleanUnits++
	case GroupTrap:
		mt.TrapUnits++
		for i := range findings {
			if findings[i].Category == catSecurity {
				mt.TrapFalsePositives++
				break
			}
		}
	case GroupTwin:
		mt.TwinUnits++
		if len(u.Expect) > 0 && hits == len(u.Expect) {
			mt.TwinsFound++
		}
	}
}

// scoreAdjudicator measures the adjudicator's lift on the security category: the candidates it saw, with and without its decisions.
func scoreAdjudicator(mt *Metrics, u Unit, findings []Finding, dropped []Dropped) {
	for i := range findings {
		if f := &findings[i]; f.Category == catSecurity {
			classify(&mt.AfterAdjudicator, u, f.Code, f.Category)
			classify(&mt.BeforeAdjudicator, u, f.Code, f.Category)
		}
	}
	for _, d := range dropped {
		if (d.Stage == "adjudicator" || d.Stage == "merged") && d.Category == catSecurity {
			classify(&mt.BeforeAdjudicator, u, d.Code, d.Category)
		}
	}
}

// classify counts a security candidate as a true positive when the unit expects its code and as a
// false positive when the unit is clean for security.
func classify(c *Counts, u Unit, code, cat string) {
	for _, e := range u.Expect {
		if e.Code == code {
			c.TP++
			return
		}
	}
	for _, cc := range u.CleanCategories {
		if cc == cat {
			c.FP++
			return
		}
	}
}

// locates reports whether a finding sits in the expected file and quotes the expected text (in its quote or in a chain step).
func locates(f Finding, e Expect) bool {
	want := normalize(e.Contains)
	if f.Location.Path == e.Path && strings.Contains(normalize(f.Quote), want) {
		return true
	}
	for _, s := range f.Chain {
		if s.Location.Path == e.Path && strings.Contains(normalize(s.Quote), want) {
			return true
		}
	}
	return false
}

// Decode reads a judge JSON report.
func Decode(r io.Reader) (*Report, error) {
	var rep Report
	if err := json.NewDecoder(r).Decode(&rep); err != nil {
		return nil, fmt.Errorf("decode the judge report: %w", err)
	}
	return &rep, nil
}

// Write prints the metrics as a table and returns the first write error.
func (mt Metrics) Write(w io.Writer) error {
	ew := &errWriter{w: w}
	ew.printf("units scored: %d   decision: %s   models: %v\n", mt.Units, mt.Decision, mt.Models)
	ew.printf("\n%-12s %4s %4s %4s  %5s %5s %5s  %9s\n", "category", "TP", "FP", "FN", "prec", "rec", "F1", "localized")
	for _, cat := range []string{catSecurity, catGovernance, catQuality} {
		if c := mt.ByCategory[cat]; c != nil {
			ew.printf("%-12s %4d %4d %4d  %5.2f %5.2f %5.2f  %9d\n", cat, c.TP, c.FP, c.FN, c.Precision(), c.Recall(), c.F1(), c.Localized)
		}
	}
	ew.printf("\n%-8s %4s %4s %4s  %5s %5s %5s\n", "code", "TP", "FP", "FN", "prec", "rec", "F1")
	codes := make([]string, 0, len(mt.ByCode))
	for c := range mt.ByCode {
		codes = append(codes, c)
	}
	sort.Strings(codes)
	for _, code := range codes {
		c := mt.ByCode[code]
		ew.printf("%-8s %4d %4d %4d  %5.2f %5.2f %5.2f\n", code, c.TP, c.FP, c.FN, c.Precision(), c.Recall(), c.F1())
	}
	ew.printf("\ntrap false-positive rate: %d/%d (%.1f%%)\n", mt.TrapFalsePositives, mt.TrapUnits, pct(mt.TrapFalsePositives, mt.TrapUnits))
	ew.printf("malicious twins found:    %d/%d (%.1f%%)\n", mt.TwinsFound, mt.TwinUnits, pct(mt.TwinsFound, mt.TwinUnits))
	ew.printf("clean units with findings: %d findings on %d clean units\n", mt.CleanFindings, mt.CleanUnits)
	ew.printf("false positives per 100 units: %.1f\n", mt.FalsePositivesPer100)
	ew.printf("severity at or above the label: %d/%d\n", mt.SeverityOK, mt.SeverityChecked)
	ew.printf("secret units withheld: %d/%d\n", mt.WithheldGot, mt.WithheldExpected)
	ew.printf("adjudicator (security) precision: %.2f before -> %.2f after (candidates %d -> %d)\n",
		mt.BeforeAdjudicator.Precision(), mt.AfterAdjudicator.Precision(), mt.BeforeAdjudicator.TP+mt.BeforeAdjudicator.FP, mt.AfterAdjudicator.TP+mt.AfterAdjudicator.FP)
	ew.printf("grounding: %d candidates, %.1f%% ungrounded before the verifier (%d repaired), 100%% grounded after\n", mt.Candidates, 100*mt.UngroundedRate, mt.Repaired)
	ew.printf("cost: $%.4f for %d calls (%d cached), $%.4f per unit\n", mt.CostUSD, mt.Calls, mt.CacheHits, mt.CostPerUnit)
	if len(mt.Missing) > 0 {
		ew.printf("units missing from the report: %s\n", strings.Join(mt.Missing, ", "))
	}
	if len(mt.Partial) > 0 {
		ew.printf("partly reviewed: %s\n", strings.Join(mt.Partial, ", "))
	}
	return ew.err
}

// errWriter keeps the first write error, so a table of lines needs one check.
type errWriter struct {
	w   io.Writer
	err error
}

func (e *errWriter) printf(format string, args ...any) {
	if e.err == nil {
		_, e.err = fmt.Fprintf(e.w, format, args...)
	}
}

func pct(n, d int) float64 {
	if d == 0 {
		return 0
	}
	return 100 * float64(n) / float64(d)
}

// CompareHeader and Row print one line per tool for a side-by-side comparison on the same units.
const CompareHeader = "tool            units  sec-prec  sec-rec  sec-F1  trapFP  twins  FP/100  cost"

// Row renders the security results of a tool as one line of the comparison.
func (mt Metrics) Row(name string) string {
	c := mt.ByCategory[catSecurity]
	if c == nil {
		c = &Counts{}
	}
	return fmt.Sprintf("%-15s %5d  %8.2f  %7.2f  %6.2f  %2d/%-3d  %2d/%-2d  %6.1f  $%.2f",
		name, mt.Units, c.Precision(), c.Recall(), c.F1(), mt.TrapFalsePositives, mt.TrapUnits, mt.TwinsFound, mt.TwinUnits, mt.FalsePositivesPer100, mt.CostUSD)
}
