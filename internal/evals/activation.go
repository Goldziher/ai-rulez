package evals

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
)

// Eval run modes.
const (
	// ModeCases is the default: full eval cases through a runner.
	ModeCases = "cases"
	// ModeActivation measures only whether the right skill is chosen for a prompt.
	ModeActivation = "activation"
)

// Activation surfaces.
const (
	// SurfaceRetrieval ranks prompts with the offline find_skill ranker; no model, no cost.
	SurfaceRetrieval = "retrieval"
	// SurfaceNative would ask a harness's model to choose; a runner must declare
	// CapabilityActivation for it, and none does yet.
	SurfaceNative = "native"
)

// Activation scopes: which skills compete with the one under test.
const (
	// ScopeDomain is the skill's domain plus the root skills (the default).
	ScopeDomain = "domain"
	// ScopeAll is every skill in the project.
	ScopeAll = "all"
)

// CapabilityActivation is what a runner declares to be sent activation requests.
const CapabilityActivation = "activation"

// ActivationSchemaVersion versions the JSON document of an activation run.
const ActivationSchemaVersion = 1

// Activation thresholds. A positive prompt passes when its activation rate is at
// least activationPositiveMin, a negative one when it is at most
// activationNegativeMax. The retrieval surface is deterministic (one run, a rate of
// 0 or 1), so they only start to matter with repeated native runs.
const (
	activationPositiveMin = 0.8
	activationNegativeMax = 0.2
	// recallAtLow and recallAtHigh are the two cut-offs of the rank metrics.
	recallAtLow  = 1
	recallAtHigh = 3
	// wilsonZ is the normal quantile of a 95% interval.
	wilsonZ = 1.96
	// activationNone is the "winner" of a prompt on which no skill ranked at all.
	activationNone = "none"
)

// Prompt statuses of an activation run.
const (
	PromptPassed     = "passed"
	PromptFailed     = "failed"
	PromptBorderline = "borderline"
)

// activationNote states what the retrieval surface does and does not measure.
const activationNote = "retrieval measures the local find_skill ranker (BM25F over name, triggers, keywords and description), " +
	"not which skill a model would choose; a skill fires when it ranks first"

// CapabilityReporter is implemented by runners that declare what they support
// beyond full case runs.
type CapabilityReporter interface {
	Capabilities() []string
}

// RequireCapability refuses a runner that does not declare capability, so a mode
// is never silently run as something else: an old runner that ignored an
// activation request would otherwise run full cases and report their results as
// activation figures.
func RequireCapability(r Runner, capability string) error {
	if r == nil {
		return fmt.Errorf("no runner configured for %s mode", capability)
	}
	if reporter, ok := r.(CapabilityReporter); ok {
		for _, c := range reporter.Capabilities() {
			if c == capability {
				return nil
			}
		}
	}
	return fmt.Errorf("runner %q does not support %s mode: it does not declare the %q capability", r.Name(), capability, capability)
}

// Interval is a 95% Wilson score interval.
type Interval struct {
	Low  float64 `json:"low"`
	High float64 `json:"high"`
}

// Rate is a proportion with its Wilson interval.
type Rate struct {
	Value    float64  `json:"value"`
	N        int      `json:"n"`
	Interval Interval `json:"interval"`
}

// Wilson returns the 95% Wilson score interval of k successes in n trials. It
// behaves at the edges (k = 0 or k = n) where the normal approximation does not,
// which matters because small n is the norm. n < 1 has no interval.
func Wilson(k, n int) Interval {
	if n < 1 {
		return Interval{}
	}
	p := float64(k) / float64(n)
	nf := float64(n)
	z2 := wilsonZ * wilsonZ
	denom := 1 + z2/nf
	centre := (p + z2/(2*nf)) / denom
	margin := wilsonZ * math.Sqrt(p*(1-p)/nf+z2/(4*nf*nf)) / denom
	return Interval{Low: round(math.Max(0, centre-margin)), High: round(math.Min(1, centre+margin))}
}

// newRate builds a Rate of k in n; nil when n is zero.
func newRate(k, n int) *Rate {
	if n < 1 {
		return nil
	}
	return &Rate{Value: round(float64(k) / float64(n)), N: n, Interval: Wilson(k, n)}
}

// ActivationPrompt is the outcome of one prompt.
type ActivationPrompt struct {
	Case string `json:"case"`
	// Expect is whether the skill should fire for the prompt.
	Expect   bool `json:"expect"`
	NearMiss bool `json:"near_miss,omitempty"`
	// Rate is the fraction of runs in which the skill fired; Runs is how many ran.
	Rate float64 `json:"rate"`
	Runs int     `json:"runs"`
	// Rank is the 1-based rank of the skill among the competing skills; null when
	// it did not rank at all.
	Rank   *int   `json:"rank"`
	Winner string `json:"winner"`
	// TopScore is the score of the winner.
	TopScore float64 `json:"top_score"`
	Status   string  `json:"status"`
}

// StolenBy counts the positive prompts of a skill that a sibling won.
type StolenBy struct {
	Skill   string  `json:"skill"`
	Prompts int     `json:"prompts"`
	Share   float64 `json:"share"`
}

// ActivationSkill is the activation result of one skill.
type ActivationSkill struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	// Competing lists the skills ranked against this one, itself included.
	Competing []string `json:"competing,omitempty"`
	// Digest is the skill's digest and SetDigest the digest of the competing set:
	// a sibling's changed description changes the competition, so it changes this.
	Digest    string `json:"digest,omitempty"`
	SetDigest string `json:"set_digest,omitempty"`
	Passing   bool   `json:"passing"`
	// Precision is TP/(TP+FP) over the skill's prompts, Recall TP/positives and
	// FalseActivation FP/negatives; each with its Wilson interval, null when the
	// denominator is zero.
	Precision       *Rate `json:"precision"`
	Recall          *Rate `json:"recall"`
	FalseActivation *Rate `json:"false_activation"`
	// RecallAt1, RecallAt3 and MRR are rank metrics over the positive prompts.
	RecallAt1 *float64 `json:"recall_at_1"`
	RecallAt3 *float64 `json:"recall_at_3"`
	MRR       *float64 `json:"mrr"`
	// Ignored counts cases whose fixtures, assertions or rubric were not used: only
	// the prompt and expect_trigger take part in an activation run.
	Ignored  int                `json:"ignored,omitempty"`
	Prompts  []ActivationPrompt `json:"prompts,omitempty"`
	StolenBy []StolenBy         `json:"stolen_by,omitempty"`
	Warnings []string           `json:"warnings,omitempty"`
	Problems []Problem          `json:"problems,omitempty"`
	Error    string             `json:"error,omitempty"`
}

// ActivationReport is the JSON document of `eval run --mode activation`.
type ActivationReport struct {
	SchemaVersion int    `json:"schema_version"`
	Mode          string `json:"mode"`
	Surface       string `json:"surface"`
	Scope         string `json:"scope"`
	Note          string `json:"note"`
	Date          string `json:"date,omitempty"`
	// Cost is always zero for the retrieval surface: no model is called.
	Cost   ActivationCost    `json:"cost"`
	Skills []ActivationSkill `json:"skills"`
	// Confusion[expected][won] counts the positive prompts of skill expected that
	// skill won (or "none" when nothing ranked), over every skill run.
	Confusion map[string]map[string]int `json:"confusion"`
	Failed    bool                      `json:"failed"`
}

// ActivationCost is the estimated and actual cost of an activation run.
type ActivationCost struct {
	EstimateUSD float64 `json:"estimate_usd"`
	ActualUSD   float64 `json:"actual_usd"`
}

// ActivationRecord is the summary kept in the results store (rates and ids only,
// never prompts or outputs).
type ActivationRecord struct {
	Surface   string `json:"surface"`
	Scope     string `json:"scope"`
	Digest    string `json:"digest"`
	SetDigest string `json:"set_digest"`
	Date      string `json:"date,omitempty"`
	Positives int    `json:"positives"`
	Negatives int    `json:"negatives"`
	// Recall and Precision are null when nothing was measured (no positive, or no
	// prompt on which the skill fired).
	Recall    *float64   `json:"recall"`
	Precision *float64   `json:"precision"`
	StolenBy  []StolenBy `json:"stolen_by,omitempty"`
}

// Record is the summary of a measured skill for the results store.
func (s *ActivationSkill) Record(surface, scope, date string) *ActivationRecord {
	rec := &ActivationRecord{Surface: surface, Scope: scope, Digest: s.Digest, SetDigest: s.SetDigest, Date: date, StolenBy: s.StolenBy}
	for i := range s.Prompts {
		if s.Prompts[i].Expect {
			rec.Positives++
		} else {
			rec.Negatives++
		}
	}
	if s.Recall != nil {
		rec.Recall = &s.Recall.Value
	}
	if s.Precision != nil {
		rec.Precision = &s.Precision.Value
	}
	return rec
}

// setDigest hashes the competing skills' digests, in id order.
func setDigest(ids []string, digests map[string]string) string {
	sorted := append([]string(nil), ids...)
	sort.Strings(sorted)
	h := sha256.New()
	for _, id := range sorted {
		fmt.Fprintf(h, "%s\x00%s\n", id, digests[id])
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// Write renders the report as JSON or Markdown.
func (r *ActivationReport) Write(w io.Writer, format string) error {
	switch format {
	case FormatJSON:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(r)
	case FormatMarkdown:
		return r.writeMarkdown(w)
	}
	return fmt.Errorf("activation results are written as json or markdown, not %q", format)
}

func (r *ActivationReport) writeMarkdown(w io.Writer) error {
	var b strings.Builder
	b.WriteString("# Skill activation results\n\n")
	fmt.Fprintf(&b, "Surface %#q, scope %#q", r.Surface, r.Scope)
	if r.Date != "" {
		fmt.Fprintf(&b, ", date %s", r.Date)
	}
	b.WriteString(", cost $0.00.\n\n" + r.Note + ".\n\n")
	b.WriteString("| Skill | Status | Prompts | Recall | Precision | False activation | recall@1 | recall@3 | MRR |\n")
	b.WriteString("| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for i := range r.Skills {
		s := &r.Skills[i]
		if s.Status == RunNoCases || s.Status == RunNotChanged {
			continue
		}
		fmt.Fprintf(&b, "| %s | %s | %d | %s | %s | %s | %s | %s | %s |\n", s.ID, activationStatus(s), len(s.Prompts),
			rateText(s.Recall), rateText(s.Precision), rateText(s.FalseActivation),
			Percent(s.RecallAt1), Percent(s.RecallAt3), mrrText(s.MRR))
	}
	for i := range r.Skills {
		if lines := activationDetail(&r.Skills[i]); len(lines) > 0 {
			fmt.Fprintf(&b, "\n### %s\n\n%s\n", r.Skills[i].ID, strings.Join(lines, "\n"))
		}
	}
	if matrix := confusionLines(r.Confusion); len(matrix) > 0 {
		b.WriteString("\n### Confusion (positive prompts: expected skill, then the skill that ranked first)\n\n")
		b.WriteString(strings.Join(matrix, "\n") + "\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func activationStatus(s *ActivationSkill) string {
	if s.Status == RunRan {
		if s.Passing {
			return "ran, passing"
		}
		return "ran, failing"
	}
	return s.Status
}

func rateText(r *Rate) string {
	if r == nil {
		return "n/a"
	}
	return fmt.Sprintf("%.0f%% (%.0f-%.0f%%)", r.Value*100, r.Interval.Low*100, r.Interval.High*100)
}

func mrrText(v *float64) string {
	if v == nil {
		return "n/a"
	}
	return fmt.Sprintf("%.2f", *v)
}

func activationDetail(s *ActivationSkill) []string {
	var lines []string
	for _, p := range s.Problems {
		lines = append(lines, "- "+p.String())
	}
	if s.Error != "" {
		lines = append(lines, "- "+s.Error)
	}
	for _, w := range s.Warnings {
		lines = append(lines, "- warning: "+w)
	}
	for _, st := range s.StolenBy {
		lines = append(lines, fmt.Sprintf("- %#q ranked first on %d of the positive prompts (%.0f%%)", st.Skill, st.Prompts, st.Share*100))
	}
	for i := range s.Prompts {
		if p := &s.Prompts[i]; p.Status != PromptPassed {
			lines = append(lines, fmt.Sprintf("- %#q: expected %s, %s ranked first (rank of this skill: %s)", p.Case, firesWord(p.Expect), winnerText(p.Winner), rankText(p.Rank)))
		}
	}
	return lines
}

func firesWord(expect bool) string {
	if expect {
		return "to fire"
	}
	return "not to fire"
}

func winnerText(w string) string {
	if w == activationNone {
		return "no skill"
	}
	return fmt.Sprintf("%#q", w)
}

func rankText(rank *int) string {
	if rank == nil {
		return "unranked"
	}
	return fmt.Sprint(*rank)
}

func confusionLines(m map[string]map[string]int) []string {
	expected := make([]string, 0, len(m))
	for k := range m {
		expected = append(expected, k)
	}
	sort.Strings(expected)
	var lines []string
	for _, e := range expected {
		won := make([]string, 0, len(m[e]))
		for k := range m[e] {
			won = append(won, k)
		}
		sort.Strings(won)
		parts := make([]string, 0, len(won))
		for _, k := range won {
			parts = append(parts, fmt.Sprintf("%s %d", k, m[e][k]))
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", e, strings.Join(parts, ", ")))
	}
	return lines
}
