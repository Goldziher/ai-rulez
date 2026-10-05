package evals

import (
	"fmt"
	"math"
)

// Case statuses.
const (
	StatusPassed  = "passed"
	StatusFailed  = "failed"
	StatusError   = "error"
	StatusSkipped = "skipped"
)

// CaseScore is the scored outcome of one case.
type CaseScore struct {
	Case          string   `json:"case"`
	ExpectTrigger bool     `json:"expect_trigger"`
	NearMiss      bool     `json:"near_miss,omitempty"`
	Status        string   `json:"status"`
	Triggered     *bool    `json:"triggered,omitempty"`
	OutcomeGraded bool     `json:"outcome_graded"`
	OutcomePassed *bool    `json:"outcome_passed,omitempty"`
	WithoutPassed *bool    `json:"without_passed,omitempty"`
	CostUSD       float64  `json:"cost_usd,omitempty"`
	Tokens        int      `json:"tokens,omitempty"`
	Failures      []string `json:"failures,omitempty"`
	Reason        string   `json:"reason,omitempty"`
}

// SkillScore is the per-skill summary.
type SkillScore struct {
	// Cases counts expanded cases; Scored excludes skipped ones.
	Cases   int `json:"cases"`
	Scored  int `json:"scored"`
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Errors  int `json:"errors"`
	Skipped int `json:"skipped"`
	// PassRate is Passed/Scored (errors count as failures); 0 when nothing was scored.
	PassRate float64 `json:"pass_rate"`
	// TriggerPrecision is TP/(TP+FP) and TriggerRecall TP/(TP+FN) over scored "with"
	// runs; null when the denominator is zero.
	TriggerPrecision *float64 `json:"trigger_precision"`
	TriggerRecall    *float64 `json:"trigger_recall"`
	// NearMissFalsePositives counts near-miss cases where the skill fired.
	NearMissCases          int `json:"near_miss_cases"`
	NearMissFalsePositives int `json:"near_miss_false_positives"`
	// OutcomeWith and OutcomeWithout are the outcome pass rates over the positive,
	// outcome-graded cases that ran in both arms; AblationDelta is their difference.
	// All three are null without ablation data.
	OutcomeWith    *float64 `json:"outcome_pass_with"`
	OutcomeWithout *float64 `json:"outcome_pass_without"`
	AblationDelta  *float64 `json:"ablation_delta"`
	// AblationCases is how many cases the three figures above are based on.
	AblationCases int `json:"ablation_cases,omitempty"`
	// SkillTokens is the token count of the skill's SKILL.md (cl100k_base, an
	// approximation). RunTokens and CostUSD are what the runner reported for the
	// "with" and "without" arms together.
	SkillTokens int     `json:"skill_tokens"`
	RunTokens   int     `json:"run_tokens"`
	CostUSD     float64 `json:"cost_usd"`
}

// ScoreOptions configures Score.
type ScoreOptions struct {
	Grade       GradeOptions
	SkillTokens int
	// Price prices the reported tokens when a runner reports usage but no cost.
	Price Price
}

// tally accumulates the counts behind the trigger and ablation figures.
type tally struct {
	tp, fp, fn, both, withPass, withoutPass int
}

// Score grades a runner response against the (expanded) cases and summarizes it.
func Score(cases []Case, resp *Response, opts ScoreOptions) (SkillScore, []CaseScore) {
	byCase := map[string]map[string]*Result{}
	var totalCost float64
	var inTokens, outTokens, totalTokens int
	for i := range resp.Results {
		r := &resp.Results[i]
		if byCase[r.Case] == nil {
			byCase[r.Case] = map[string]*Result{}
		}
		byCase[r.Case][r.Arm] = r
		totalCost += r.CostUSD
		inTokens += r.InputTokens
		outTokens += r.OutputTokens
		totalTokens += r.InputTokens + r.OutputTokens
	}
	// Spend must never be undercounted, or the --max-cost stop can be missed: a
	// missing case or an omitted per-case cost shows up as a lower sum than the
	// runner's own total, and reported tokens price out when no cost is given.
	totalCost = math.Max(totalCost, resp.CostUSD)
	if totalCost == 0 && opts.Price != (Price{}) {
		totalCost = (float64(inTokens)*opts.Price.InPerMTok + float64(outTokens)*opts.Price.OutPerMTok) / 1e6
	}

	score := SkillScore{Cases: len(cases), SkillTokens: opts.SkillTokens, RunTokens: totalTokens, CostUSD: round(totalCost)}
	scores := make([]CaseScore, 0, len(cases))
	var t tally
	for i := range cases {
		c := &cases[i]
		cs := scoreCase(c, byCase[c.ID][ArmWith], byCase[c.ID][ArmWithout], opts, &t)
		if cs.NearMiss {
			score.NearMissCases++
			if cs.Triggered != nil && *cs.Triggered {
				score.NearMissFalsePositives++
			}
		}
		switch cs.Status {
		case StatusPassed:
			score.Passed++
		case StatusFailed:
			score.Failed++
		case StatusError:
			score.Errors++
		case StatusSkipped:
			score.Skipped++
		}
		scores = append(scores, cs)
	}
	score.Scored = score.Cases - score.Skipped
	if score.Scored > 0 {
		score.PassRate = round(float64(score.Passed) / float64(score.Scored))
	}
	score.TriggerPrecision = ratio(t.tp, t.tp+t.fp)
	score.TriggerRecall = ratio(t.tp, t.tp+t.fn)
	if t.both > 0 {
		with, without := round(float64(t.withPass)/float64(t.both)), round(float64(t.withoutPass)/float64(t.both))
		delta := round(with - without)
		score.OutcomeWith, score.OutcomeWithout, score.AblationDelta = &with, &without, &delta
		score.AblationCases = t.both
	}
	return score, scores
}

// scoreCase grades one case from its "with" and "without" arm results.
func scoreCase(c *Case, with, without *Result, opts ScoreOptions, t *tally) CaseScore {
	cs := CaseScore{Case: c.ID, ExpectTrigger: c.Expects(), NearMiss: c.NearMissOf != ""}
	for _, r := range []*Result{with, without} {
		if r != nil {
			cs.CostUSD += r.CostUSD
			cs.Tokens += r.InputTokens + r.OutputTokens
		}
	}
	cs.CostUSD = round(cs.CostUSD)

	if status, reason := unscoredStatus(with); status != "" {
		cs.Status, cs.Reason = status, reason
		return cs
	}

	cs.Triggered = with.Triggered
	grade := GradeOutcome(c, with, opts.Grade)
	cs.OutcomeGraded = grade.Graded
	if grade.Graded {
		passed := grade.Passed
		cs.OutcomePassed = &passed
	}
	triggerOK := *with.Triggered == c.Expects()
	switch {
	case triggerOK:
	case c.Expects():
		cs.Failures = append(cs.Failures, "skill did not trigger")
	default:
		cs.Failures = append(cs.Failures, "skill triggered but should not have")
	}
	cs.Failures = append(cs.Failures, grade.Failures...)
	cs.Status = StatusFailed
	if triggerOK && grade.Passed {
		cs.Status = StatusPassed
	}

	t.countTrigger(c.Expects(), *with.Triggered)
	if c.Expects() && grade.Graded && without != nil && !without.Skipped && without.Error == "" {
		wg := GradeOutcome(c, without, opts.Grade)
		passed := wg.Passed
		cs.WithoutPassed = &passed
		t.countAblation(grade.Passed, wg.Passed)
	}
	return cs
}

// unscoredStatus returns a non-empty status when the "with" result cannot be graded.
func unscoredStatus(with *Result) (status, reason string) {
	switch {
	case with == nil:
		return StatusError, "the runner returned no result for this case"
	case with.Skipped:
		return StatusSkipped, with.Reason
	case with.Error != "":
		return StatusError, with.Error
	case with.Triggered == nil:
		return StatusError, "the runner did not report whether the skill triggered"
	}
	return "", ""
}

func (t *tally) countTrigger(expected, fired bool) {
	switch {
	case expected && fired:
		t.tp++
	case !expected && fired:
		t.fp++
	case expected:
		t.fn++
	}
}

func (t *tally) countAblation(withPassed, withoutPassed bool) {
	t.both++
	if withPassed {
		t.withPass++
	}
	if withoutPassed {
		t.withoutPass++
	}
}

func ratio(num, den int) *float64 {
	if den == 0 {
		return nil
	}
	v := round(float64(num) / float64(den))
	return &v
}

// round keeps four decimals so stored scores are stable across platforms.
func round(v float64) float64 { return math.Round(v*1e4) / 1e4 }

// Percent formats an optional rate for text output.
func Percent(v *float64) string {
	if v == nil {
		return "n/a"
	}
	return fmt.Sprintf("%.0f%%", *v*100)
}
