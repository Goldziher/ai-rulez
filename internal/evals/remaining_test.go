package evals

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExhausted_ARemainderThatRoundsToZeroIsNoBudget(t *testing.T) {
	tests := []struct {
		name          string
		limit, spent  float64
		wantExhausted bool
		wantRemaining float64
	}{
		{"no limit", 0, 5, false, 0},
		{"plenty left", 1, 0.25, false, 0.75},
		{"a sub-1e-4 remainder that rounds to zero", 1, 0.99996, true, 0},
		{"a sub-1e-4 remainder that rounds up", 1, 0.99994, false, 0.0001},
		{"overspent", 1, 1.2, true, -0.2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act and Assert: 0 means unlimited to a runner, so a limited run never hands it out.
			assert.Equal(t, tt.wantExhausted, exhausted(tt.limit, tt.spent))
			assert.InDelta(t, tt.wantRemaining, remaining(tt.limit, tt.spent), 1e-12)
		})
	}
}

func TestRun_ASubCentRemainderStopsInsteadOfRunningUnlimited(t *testing.T) {
	// Arrange: the first skill spends all but $0.00004 of the limit.
	cfg := projectWithSkills(t)
	runner := &fakeRunner{fn: func(req *Request) (*Response, error) {
		resp := &Response{Version: ProtocolVersion}
		for i := range req.Cases {
			c := &req.Cases[i]
			resp.Results = append(resp.Results, Result{Case: c.ID, Arm: ArmWith, Triggered: bp(c.Expects()), Output: "ok", CostUSD: 0.00498})
		}
		return resp, nil
	}}
	opts := baseOptions(cfg, runner)
	opts.Ablation, opts.MaxCostUSD = false, 0.01
	opts.Price = Price{InPerMTok: 0.001, OutPerMTok: 0.001} // the estimate fits; the actual spend leaves $0.00004

	// Act
	report, err := Run(context.Background(), opts)

	// Assert
	require.NoError(t, err)
	statuses := map[string]string{}
	for i := range report.Skills {
		statuses[report.Skills[i].ID] = report.Skills[i].Status
	}
	assert.Equal(t, 1, runner.calls, "the second skill is never handed max_cost_usd 0")
	assert.Contains(t, statuses, "beta")
	assert.Equal(t, RunOverBudget, statuses["beta"])
	for _, req := range runner.reqs {
		assert.Positive(t, req.MaxCostUSD)
	}
}
