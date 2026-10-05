package evals

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func bp(b bool) *bool       { return &b }
func fp(f float64) *float64 { return &f }

func mkCase(id string, trigger bool, asserts ...Assertion) Case {
	return Case{ID: id, Prompt: "p " + id, ExpectTrigger: bp(trigger), Assertions: asserts}
}

func TestScore_PrecisionRecallPassRate(t *testing.T) {
	contains := Assertion{Type: AssertContains, Value: "ok"}
	cases := []Case{
		mkCase("tp1", true, contains), // triggered, output ok -> pass
		mkCase("tp2", true, contains), // triggered, output bad -> fail (still TP)
		mkCase("fn", true),            // not triggered -> fail, FN
		mkCase("tn", false),           // not triggered -> pass
		mkCase("fp", false),           // triggered -> fail, FP
		{ID: "nm.near-miss-1", Prompt: "q", ExpectTrigger: bp(false), NearMissOf: "tp1"}, // triggered -> FP + near miss FP
		mkCase("skipme", true),
	}
	resp := &Response{Version: 1, Results: []Result{
		{Case: "tp1", Arm: ArmWith, Triggered: bp(true), Output: "all ok", InputTokens: 100, OutputTokens: 10, CostUSD: 0.01},
		{Case: "tp2", Arm: ArmWith, Triggered: bp(true), Output: "nope", CostUSD: 0.02},
		{Case: "fn", Arm: ArmWith, Triggered: bp(false)},
		{Case: "tn", Arm: ArmWith, Triggered: bp(false)},
		{Case: "fp", Arm: ArmWith, Triggered: bp(true)},
		{Case: "nm.near-miss-1", Arm: ArmWith, Triggered: bp(true)},
		{Case: "skipme", Arm: ArmWith, Skipped: true, Reason: "unsupported"},
	}}
	score, scores := Score(cases, resp, ScoreOptions{SkillTokens: 321})

	assert.Equal(t, 7, score.Cases)
	assert.Equal(t, 6, score.Scored)
	assert.Equal(t, 1, score.Skipped)
	assert.Equal(t, 2, score.Passed) // tp1, tn
	assert.Equal(t, 4, score.Failed)
	assert.InDelta(t, 0.3333, score.PassRate, 1e-9)
	// TP=2 (tp1,tp2) FP=2 (fp, near-miss) FN=1
	assert.InDelta(t, 0.5, *score.TriggerPrecision, 1e-9)
	assert.InDelta(t, 0.6667, *score.TriggerRecall, 1e-9)
	assert.Equal(t, 1, score.NearMissCases)
	assert.Equal(t, 1, score.NearMissFalsePositives)
	assert.Equal(t, 321, score.SkillTokens)
	assert.Equal(t, 110, score.RunTokens)
	assert.InDelta(t, 0.03, score.CostUSD, 1e-9)
	assert.Nil(t, score.AblationDelta)

	byID := map[string]CaseScore{}
	for _, s := range scores {
		byID[s.Case] = s
	}
	assert.Equal(t, StatusPassed, byID["tp1"].Status)
	assert.Equal(t, StatusFailed, byID["tp2"].Status)
	assert.Contains(t, byID["fn"].Failures, "skill did not trigger")
	assert.Contains(t, byID["fp"].Failures, "skill triggered but should not have")
	assert.Equal(t, StatusSkipped, byID["skipme"].Status)
}

func TestScore_NilRatiosWhenNoDenominator(t *testing.T) {
	cases := []Case{mkCase("only-negative", false)}
	resp := &Response{Version: 1, Results: []Result{{Case: "only-negative", Arm: ArmWith, Triggered: bp(false)}}}
	score, _ := Score(cases, resp, ScoreOptions{})
	assert.Nil(t, score.TriggerPrecision)
	assert.Nil(t, score.TriggerRecall)
	assert.InDelta(t, 1.0, score.PassRate, 1e-9)
}

func TestScore_MissingResultsAndErrorsCountAsFailures(t *testing.T) {
	cases := []Case{mkCase("a", true), mkCase("b", true), mkCase("c", true)}
	resp := &Response{Version: 1, Results: []Result{
		{Case: "a", Arm: ArmWith, Triggered: bp(true)},
		{Case: "b", Arm: ArmWith, Error: "boom"},
	}}
	score, scores := Score(cases, resp, ScoreOptions{})
	assert.Equal(t, 2, score.Errors)
	assert.Equal(t, 1, score.Passed)
	assert.InDelta(t, 0.3333, score.PassRate, 1e-9)
	assert.Equal(t, StatusError, scores[1].Status)
	assert.Contains(t, scores[2].Reason, "no result")
}

func TestScore_AblationDelta(t *testing.T) {
	contains := Assertion{Type: AssertContains, Value: "ok"}
	cases := []Case{
		mkCase("helps", true, contains),     // with pass, without fail
		mkCase("neutral", true, contains),   // both pass
		mkCase("hurts", true, contains),     // with fail, without pass
		mkCase("helps2", true, contains),    // with pass, without fail
		mkCase("no-outcome", true),          // not outcome graded: excluded from the delta
		mkCase("negative", false, contains), // negatives are excluded
	}
	pass, fail := "ok", "bad"
	resp := &Response{Version: 1, Results: []Result{
		{Case: "helps", Arm: ArmWith, Triggered: bp(true), Output: pass},
		{Case: "helps", Arm: ArmWithout, Output: fail},
		{Case: "neutral", Arm: ArmWith, Triggered: bp(true), Output: pass},
		{Case: "neutral", Arm: ArmWithout, Output: pass},
		{Case: "hurts", Arm: ArmWith, Triggered: bp(true), Output: fail},
		{Case: "hurts", Arm: ArmWithout, Output: pass},
		{Case: "helps2", Arm: ArmWith, Triggered: bp(true), Output: pass},
		{Case: "helps2", Arm: ArmWithout, Output: fail},
		{Case: "no-outcome", Arm: ArmWith, Triggered: bp(true)},
		{Case: "no-outcome", Arm: ArmWithout},
		{Case: "negative", Arm: ArmWith, Triggered: bp(false), Output: pass},
		{Case: "negative", Arm: ArmWithout, Output: pass},
	}}
	score, scores := Score(cases, resp, ScoreOptions{})
	require.NotNil(t, score.AblationDelta)
	assert.InDelta(t, 0.75, *score.OutcomeWith, 1e-9)   // helps, neutral, helps2 of 4
	assert.InDelta(t, 0.5, *score.OutcomeWithout, 1e-9) // neutral, hurts of 4
	assert.InDelta(t, 0.25, *score.AblationDelta, 1e-9)
	assert.Nil(t, scores[4].WithoutPassed)
}

func TestScore_RunnerVerdictAndRubric(t *testing.T) {
	withRubric := Case{ID: "r", Prompt: "p", ExpectTrigger: bp(true), Rubric: "be good"}
	ok := Case{ID: "v", Prompt: "p", ExpectTrigger: bp(true), Assertions: []Assertion{{Type: AssertContains, Value: "never present"}}}
	resp := &Response{Version: 1, Results: []Result{
		{Case: "r", Arm: ArmWith, Triggered: bp(true), RubricScore: fp(0.69)},
		{Case: "v", Arm: ArmWith, Triggered: bp(true), Passed: bp(true)}, // verdict wins over local grading
	}}
	score, scores := Score([]Case{withRubric, ok}, resp, ScoreOptions{})
	assert.Equal(t, StatusFailed, scores[0].Status)
	assert.Contains(t, scores[0].Failures[0], "rubric score 0.69 is below 0.70")
	assert.Equal(t, StatusPassed, scores[1].Status)
	assert.Equal(t, 1, score.Passed)

	resp.Results[0].RubricScore = fp(0.7)
	_, scores = Score([]Case{withRubric}, resp, ScoreOptions{})
	assert.Equal(t, StatusPassed, scores[0].Status)

	resp.Results[0].RubricScore = nil
	_, scores = Score([]Case{withRubric}, resp, ScoreOptions{})
	assert.Contains(t, scores[0].Failures[0], "not graded")
}

func TestGradeOutcome_Assertions(t *testing.T) {
	work := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(work, "out.txt"), []byte("hello world"), 0o600))
	r := &Result{Output: "deployed 3 services", WorkDir: work}
	tests := []struct {
		name   string
		a      Assertion
		opts   GradeOptions
		passes bool
	}{
		{"contains", Assertion{Type: AssertContains, Value: "services"}, GradeOptions{}, true},
		{"contains miss", Assertion{Type: AssertContains, Value: "nope"}, GradeOptions{}, false},
		{"not_contains", Assertion{Type: AssertNotContains, Value: "error"}, GradeOptions{}, true},
		{"not_contains hit", Assertion{Type: AssertNotContains, Value: "deployed"}, GradeOptions{}, false},
		{"regex", Assertion{Type: AssertRegex, Value: `deployed \d+ services`}, GradeOptions{}, true},
		{"regex miss", Assertion{Type: AssertRegex, Value: `^\d+`}, GradeOptions{}, false},
		{"file contains", Assertion{Type: AssertContains, Value: "hello", Path: "out.txt"}, GradeOptions{}, true},
		{"file missing", Assertion{Type: AssertContains, Value: "hello", Path: "gone.txt"}, GradeOptions{}, false},
		{"file_exists", Assertion{Type: AssertFileExists, Path: "out.txt"}, GradeOptions{}, true},
		{"file_exists absent", Assertion{Type: AssertFileExists, Path: "gone.txt"}, GradeOptions{}, false},
		{"file_exists exists false", Assertion{Type: AssertFileExists, Path: "gone.txt", Exists: bp(false)}, GradeOptions{}, true},
		{"command ok", Assertion{Type: AssertCommandExit, Command: "test -f out.txt"}, GradeOptions{AllowExec: true}, true},
		{"command exit code", Assertion{Type: AssertCommandExit, Command: "exit 3", ExitCode: intPtr(3)}, GradeOptions{AllowExec: true}, true},
		{"command wrong code", Assertion{Type: AssertCommandExit, Command: "exit 3"}, GradeOptions{AllowExec: true}, false},
		{"command gated", Assertion{Type: AssertCommandExit, Command: "true"}, GradeOptions{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Case{ID: "x", Assertions: []Assertion{tt.a}}
			grade := GradeOutcome(c, r, tt.opts)
			assert.Equal(t, tt.passes, grade.Passed, grade.Failures)
		})
	}
}

func intPtr(i int) *int { return &i }

func TestGradeOutcome_FilePathCannotEscapeWorkDir(t *testing.T) {
	work := t.TempDir()
	c := &Case{ID: "x", Assertions: []Assertion{{Type: AssertFileExists, Path: "../etc"}}}
	grade := GradeOutcome(c, &Result{WorkDir: work}, GradeOptions{})
	assert.False(t, grade.Passed)
}
