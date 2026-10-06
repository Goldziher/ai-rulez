package improve

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/evals"
	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
)

// epsilon absorbs float error when comparing rates.
const epsilon = 1e-9

// CaseOutcome is the majority outcome of one case over the runs of an arm.
type CaseOutcome struct {
	Case      string `json:"case"`
	Pass      bool   `json:"pass"`
	Triggered bool   `json:"triggered"`
	// Unstable marks a case whose pass/fail was not unanimous across runs.
	Unstable bool `json:"unstable,omitempty"`
	Expect   bool `json:"expect_trigger"`
	NearMiss bool `json:"near_miss,omitempty"`
}

// Metrics summarises an arm over the cases both arms scored.
type Metrics struct {
	Scored                 int      `json:"scored"`
	Passed                 int      `json:"passed"`
	PassRate               float64  `json:"pass_rate"`
	TriggerPrecision       *float64 `json:"trigger_precision"`
	TriggerRecall          *float64 `json:"trigger_recall"`
	NearMissFalsePositives int      `json:"near_miss_false_positives"`
}

// Measurement is one evaluated arm: per-case majority outcomes plus the spend.
type Measurement struct {
	Outcomes []CaseOutcome `json:"outcomes"`
	// Skipped are cases the runner could not run in any run.
	Skipped []string `json:"skipped,omitempty"`
	CostUSD float64  `json:"cost_usd"`
	// NoCost is set when the runner reported no cost in a budgeted run and the
	// whole remaining budget was charged.
	NoCost   bool     `json:"no_cost_reported,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// Evaluator scores a skill directory against cases through an eval runner,
// repeating the run Runs times and taking the majority per case.
type Evaluator struct {
	Runner  evals.Runner
	Harness string
	Model   string
	Runs    int
	Grade   evals.GradeOptions
	Price   evals.Price
	Counter tokens.Counter
}

// Eval runs cases (already expanded) against the skill in dir. budget is the
// USD left (0: unlimited); a run that reports no cost under a budget is charged
// the whole budget, the rule the eval engine uses.
func (e *Evaluator) Eval(ctx context.Context, id, dir, digest string, cases []evals.Case, budget float64) (*Measurement, error) {
	runs := max(e.Runs, 1)
	type tally struct {
		pass, fail, trig, noTrig, seen int
		expect, nearMiss               bool
	}
	by := map[string]*tally{}
	m := &Measurement{}
	skillTokens := 0
	if e.Counter != nil {
		if data, err := os.ReadFile(filepath.Join(dir, skillFile)); err == nil { //nolint:gosec // the skill copy in the run directory
			skillTokens = e.Counter.Count(string(data))
		}
	}
	left := budget
	for run := 0; run < runs; run++ {
		req := &evals.Request{
			Version: evals.ProtocolVersion, Harness: e.Harness, Model: e.Model, MaxCostUSD: roundUSD(left),
			Skill: evals.SkillRef{ID: id, Dir: dir, Digest: digest}, Cases: cases,
		}
		resp, err := e.Runner.Run(ctx, req)
		if err != nil {
			return m, fmt.Errorf("eval runner: %w", err)
		}
		if err := resp.Validate(req); err != nil {
			return m, fmt.Errorf("eval runner: %w", err)
		}
		score, scores := evals.Score(cases, resp, evals.ScoreOptions{Grade: e.Grade, SkillTokens: skillTokens, Price: e.Price}) //nolint:contextcheck // grading is bounded by GradeOptions
		charged := score.CostUSD
		if budget > 0 && !reportedCost(resp) {
			charged = math.Max(charged, left)
			m.NoCost = true
		}
		m.CostUSD = roundUSD(m.CostUSD + charged)
		if budget > 0 {
			left = math.Max(0, budget-m.CostUSD)
		}
		for i := range scores {
			cs := &scores[i]
			t := by[cs.Case]
			if t == nil {
				t = &tally{expect: cs.ExpectTrigger, nearMiss: cs.NearMiss}
				by[cs.Case] = t
			}
			if cs.Status == evals.StatusSkipped {
				continue
			}
			t.seen++
			if cs.Status == evals.StatusPassed {
				t.pass++
			} else {
				t.fail++
			}
			if cs.Triggered != nil && *cs.Triggered {
				t.trig++
			} else {
				t.noTrig++
			}
		}
		if budget > 0 && left <= 0 && run < runs-1 {
			m.Warnings = append(m.Warnings, "the budget ran out before every run finished; the majority uses the runs that completed")
			break
		}
	}
	ids := make([]string, 0, len(by))
	for id := range by {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, cid := range ids {
		t := by[cid]
		if t.seen == 0 {
			m.Skipped = append(m.Skipped, cid)
			continue
		}
		m.Outcomes = append(m.Outcomes, CaseOutcome{
			Case: cid, Pass: t.pass*2 > t.seen, Triggered: t.trig*2 > t.seen,
			Unstable: t.pass != 0 && t.fail != 0, Expect: t.expect, NearMiss: t.nearMiss,
		})
	}
	return m, nil
}

func reportedCost(resp *evals.Response) bool {
	if resp.CostUSD > 0 {
		return true
	}
	for i := range resp.Results {
		r := &resp.Results[i]
		if r.CostUSD > 0 || r.InputTokens > 0 || r.OutputTokens > 0 {
			return true
		}
	}
	return false
}

func roundUSD(v float64) float64 { return math.Round(v*1e4) / 1e4 }

// MetricsOf summarises outcomes, optionally restricted to the cases in only.
func MetricsOf(outcomes []CaseOutcome, only map[string]bool) Metrics {
	var m Metrics
	var tp, fp, fn int
	for _, o := range outcomes {
		if only != nil && !only[o.Case] {
			continue
		}
		m.Scored++
		if o.Pass {
			m.Passed++
		}
		switch {
		case o.Expect && o.Triggered:
			tp++
		case !o.Expect && o.Triggered:
			fp++
		case o.Expect && !o.Triggered:
			fn++
		}
		if o.NearMiss && o.Triggered {
			m.NearMissFalsePositives++
		}
	}
	if m.Scored > 0 {
		m.PassRate = roundRate(float64(m.Passed) / float64(m.Scored))
	}
	m.TriggerPrecision = ratio(tp, tp+fp)
	m.TriggerRecall = ratio(tp, tp+fn)
	return m
}

func ratio(num, den int) *float64 {
	if den == 0 {
		return nil
	}
	v := roundRate(float64(num) / float64(den))
	return &v
}

func roundRate(v float64) float64 { return math.Round(v*1e4) / 1e4 }

// PairRow is one case of the paired comparison.
type PairRow struct {
	Case string `json:"case"`
	Base bool   `json:"base_pass"`
	Cand bool   `json:"candidate_pass"`
	// Unstable rows are shown but never count as wins or losses.
	Unstable bool `json:"unstable,omitempty"`
}

// Comparison is the paired held-out comparison of a candidate with the baseline.
type Comparison struct {
	Base         Metrics   `json:"baseline"`
	Cand         Metrics   `json:"candidate"`
	Gain         float64   `json:"gain"`
	Wins         []string  `json:"wins"`
	Losses       []string  `json:"losses"`
	Unstable     []string  `json:"unstable,omitempty"`
	Table        []PairRow `json:"table"`
	Underpowered bool      `json:"underpowered"`
	// CI is the bootstrap interval of Gain over the paired cases.
	CI *CI `json:"gain_ci,omitempty"`
	// BaseScored and Skipped are gate inputs, not part of the report: the
	// cases the baseline scored, and those of them the candidate did not.
	BaseScored int      `json:"-"`
	Skipped    []string `json:"-"`
}

// Gate holds the acceptance thresholds.
type Gate struct {
	MinGain        float64
	MaxRegressions int
	// RequireCIAboveZero makes the lower end of the bootstrap interval of the
	// gain a gate condition. Off by default: suites of 5-20 cases almost never
	// clear it, so the interval is reported, not enforced.
	RequireCIAboveZero bool
}

// Verdict is the gate's decision for one round.
type Verdict struct {
	Accept  bool     `json:"accept"`
	Reasons []string `json:"reasons,omitempty"`
}

// underpoweredBelow is the held-out size under which the report says "underpowered".
const underpoweredBelow = 8

// Compare pairs the baseline and candidate on the cases both scored.
func Compare(base, cand []CaseOutcome) Comparison {
	candBy := map[string]CaseOutcome{}
	for _, o := range cand {
		candBy[o.Case] = o
	}
	both := map[string]bool{}
	var b, c []CaseOutcome
	cmp := Comparison{Wins: []string{}, Losses: []string{}, Table: []PairRow{}}
	for _, o := range base {
		cmp.BaseScored++
		co, ok := candBy[o.Case]
		if !ok {
			// A case the candidate could not be scored on is a loss, never a pass by absence.
			cmp.Skipped = append(cmp.Skipped, o.Case)
			cmp.Losses = append(cmp.Losses, o.Case)
			continue
		}
		both[o.Case] = true
		b, c = append(b, o), append(c, co)
		row := PairRow{Case: o.Case, Base: o.Pass, Cand: co.Pass, Unstable: o.Unstable || co.Unstable}
		cmp.Table = append(cmp.Table, row)
		switch {
		case row.Unstable:
			cmp.Unstable = append(cmp.Unstable, o.Case)
		case !o.Pass && co.Pass:
			cmp.Wins = append(cmp.Wins, o.Case)
		case o.Pass && !co.Pass:
			cmp.Losses = append(cmp.Losses, o.Case)
		}
	}
	cmp.Base, cmp.Cand = MetricsOf(b, both), MetricsOf(c, both)
	cmp.Gain = roundRate(cmp.Cand.PassRate - cmp.Base.PassRate)
	cmp.CI = BootstrapGain(cmp.Table, BootstrapResamples)
	cmp.Underpowered = len(both) < underpoweredBelow || (cmp.CI != nil && cmp.CI.IncludesZero())
	return cmp
}

// Decide applies the acceptance gate to a comparison. Lint, security and the
// token cap are the diff policy's business and have already passed.
func (g Gate) Decide(cmp Comparison) Verdict {
	var reasons []string
	switch {
	case len(cmp.Table) == 0:
		reasons = append(reasons, "no evidence: no held-out case was scored by both the baseline and the candidate")
	case len(cmp.Skipped) > 0:
		reasons = append(reasons, fmt.Sprintf("regression: the candidate skipped %d held-out case(s) the baseline scored", len(cmp.Skipped)))
	}
	// --min-gain 0 must not accept a candidate that gained nothing: some stable win is always required.
	if cmp.Gain+epsilon < g.MinGain || cmp.Gain <= epsilon || len(cmp.Wins) == 0 {
		reasons = append(reasons, fmt.Sprintf("below gain: %+.1f points and %d stable win(s), need %+.1f points and at least one win", cmp.Gain*100, len(cmp.Wins), g.MinGain*100))
	}
	if g.RequireCIAboveZero && (cmp.CI == nil || cmp.CI.Low <= epsilon) {
		reasons = append(reasons, "below confidence: the 95% bootstrap interval of the gain includes zero")
	}
	if len(cmp.Losses) > g.MaxRegressions {
		reasons = append(reasons, fmt.Sprintf("regression: %d held-out case(s) flipped pass to fail, allowed %d", len(cmp.Losses), g.MaxRegressions))
	}
	if worse(cmp.Base.TriggerPrecision, cmp.Cand.TriggerPrecision) {
		reasons = append(reasons, "regression: trigger precision dropped")
	}
	if worse(cmp.Base.TriggerRecall, cmp.Cand.TriggerRecall) {
		reasons = append(reasons, "regression: trigger recall dropped")
	}
	if cmp.Cand.NearMissFalsePositives > cmp.Base.NearMissFalsePositives {
		reasons = append(reasons, "regression: more near-miss false positives")
	}
	return Verdict{Accept: len(reasons) == 0, Reasons: reasons}
}

// worse reports a candidate metric below the baseline; a metric that exists in
// the baseline and not in the candidate counts as worse.
func worse(base, cand *float64) bool {
	switch {
	case base == nil:
		return false
	case cand == nil:
		return true
	}
	return *cand+epsilon < *base
}

// Decision is the one-word feedback an optimizer receives about a round; it
// never carries per-case held-out information.
func (v Verdict) Decision() string {
	if v.Accept {
		return "accepted"
	}
	for _, r := range v.Reasons {
		if strings.HasPrefix(r, "regression") {
			return "rejected: regression"
		}
	}
	for _, r := range v.Reasons {
		if strings.HasPrefix(r, "below confidence") {
			return "rejected: below confidence"
		}
	}
	return "rejected: below gain"
}
