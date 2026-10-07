package improve

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

func TestSpentOut_ARemainderThatRoundsToZeroIsNoBudget(t *testing.T) {
	tests := []struct {
		name string
		left float64
		want bool
	}{
		{"nothing left", 0, true},
		{"plenty left", 0.36, false},
		{"a sub-1e-4 remainder that rounds to zero", 0.00004, true},
		{"a sub-1e-4 remainder that rounds up", 0.00006, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act and Assert: max_cost_usd 0 means unlimited to a runner or an optimizer.
			assert.Equal(t, tt.want, spentOut(tt.left))
		})
	}
}

func TestExecute_ASubCentRemainderEndsTheRunInsteadOfAnUnlimitedRound(t *testing.T) {
	// Arrange: baseline evals cost 0.04 and the first round reports 0.35996, leaving $0.00004 of 0.40.
	root, configDir := project(t)
	var handed []float64
	opt := &runner.Fake{Handle: func(s runner.Spec) runner.Result {
		var req OptimizerRequest
		if err := json.Unmarshal(s.Stdin, &req); err != nil {
			return runner.Result{Status: runner.StatusError, Err: err}
		}
		handed = append(handed, req.Budget.MaxCostUSD)
		return runner.Result{Status: runner.StatusOK, Stdout: []byte(`{"version":1,"summary":"no change","changed":[],"cost_usd":0.35996}`)}
	}}
	o := baseOptions(root, configDir, goodEval(), opt)
	o.MaxRounds, o.MaxCostUSD = 3, 0.40
	plan := mustPrepare(t, &o)

	// Act
	report, err := plan.Execute(context.Background())

	// Assert
	require.NoError(t, err)
	assert.Equal(t, []float64{0.36}, handed, "no round is handed max_cost_usd 0")
	assert.Contains(t, report.Reason, "over budget")
}
