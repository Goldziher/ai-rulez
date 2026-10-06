package evals

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun_RubricItemsReachARunnerAsAFreeTextRubricToo(t *testing.T) {
	// Arrange
	cfg := t.TempDir()
	writeSkill(t, cfg, "alpha", "---\nname: alpha\ndescription: a\n---\nbody\n", `cases:
  - id: graded
    prompt: do it
    expect_trigger: true
    rubric_items:
      - {text: names the cluster, weight: 3}
      - {text: no new dependency}
    rubric_min_score: 0.5
`)
	score := 0.6
	runner := &fakeRunner{fn: func(req *Request) (*Response, error) {
		return &Response{Version: ProtocolVersion, Results: []Result{{Case: "graded", Arm: ArmWith, Triggered: bp(true), RubricScore: &score}}}, nil
	}}

	// Act
	report, err := Run(context.Background(), &RunOptions{ConfigDir: cfg, Runner: runner, Harness: "claude", Store: NewStore()})

	// Assert
	require.NoError(t, err)
	require.Len(t, runner.reqs, 1)
	sent := runner.reqs[0].Cases[0]
	assert.Contains(t, sent.Rubric, "1. (weight 3) names the cluster")
	assert.Len(t, sent.RubricItems, 2, "the checklist travels too")
	assert.False(t, report.Failed, "a score of 0.6 clears the 0.5 pass mark")
}
