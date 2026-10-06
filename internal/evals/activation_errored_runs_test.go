package evals

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAggregateActivation_CountsErroredRuns(t *testing.T) {
	// Arrange
	c := &Case{ID: "c", Target: "deploy-staging"}
	outs := []activationOutcome{
		{fired: map[string]bool{"deploy-staging": true}},
		{err: "rate limited"},
		{err: "rate limited"},
	}

	// Act
	res := aggregateActivation(c, outs)

	// Assert
	assert.Equal(t, 1, res.Runs)
	assert.Equal(t, 2, res.ErroredRuns)
}

func TestRunActivationNative_MostRunsErroredIsNoResult(t *testing.T) {
	tests := []struct {
		name       string
		runs       int
		errored    int
		wantStatus string
	}{
		{"no errors", 5, 0, PromptPassed},
		{"a minority errored keeps the prompt", 5, 2, PromptPassed},
		{"a majority errored is no result", 5, 3, PromptError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := activationProject(t)
			r := nativeRunner{&fakeRunner{fn: func(req *Request) (*Response, error) {
				resp := &Response{Version: ProtocolVersion}
				for i := range req.Cases {
					c := &req.Cases[i]
					usable := tt.runs - tt.errored
					counts := map[string]int{"deploy-staging": usable}
					if !c.Expects() {
						counts = map[string]int{firedNone: usable}
					}
					resp.Results = append(resp.Results, Result{Case: c.ID, Arm: ArmWith, Runs: usable, ErroredRuns: tt.errored, FiredCounts: counts})
				}
				return resp, nil
			}}}

			// Act
			report, err := RunActivation(context.Background(), nativeOptions(cfg, r))

			// Assert
			require.NoError(t, err)
			require.Len(t, report.Skills, 1)
			for _, p := range report.Skills[0].Prompts {
				assert.Equal(t, tt.wantStatus, p.Status, p.Case)
				if tt.wantStatus == PromptError {
					assert.Contains(t, p.Error, "errored")
				}
			}
		})
	}
}
