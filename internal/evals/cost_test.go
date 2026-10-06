package evals

import (
	"context"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/pricing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPriceFor(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		want      Price
		wantKnown bool
	}{
		{"short haiku", "haiku", Price{InPerMTok: 1, OutPerMTok: 5}, true},
		{"full haiku name", "Claude-Haiku-4", Price{InPerMTok: 1, OutPerMTok: 5}, true},
		{"opus", "opus", Price{InPerMTok: 15, OutPerMTok: 75}, true},
		{"empty model is the sonnet tier", "", Price{InPerMTok: 3, OutPerMTok: 15}, true},
		{"default model is the sonnet tier", "default", Price{InPerMTok: 3, OutPerMTok: 15}, true},
		{"unknown model falls back and is flagged", "mystery-9", Price{InPerMTok: 3, OutPerMTok: 15}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got, known := PriceFor(tt.model)

			// Assert
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantKnown, known)
		})
	}
}

func TestPriceFor_UsesTheSharedTable(t *testing.T) {
	// Arrange
	for _, model := range []string{"claude-opus-4", "gpt-4o-mini", "sonnet"} {
		want, ok := pricing.Lookup(model)
		require.True(t, ok, model)

		// Act
		got, known := PriceFor(model)

		// Assert
		assert.True(t, known)
		assert.Equal(t, want, got, model)
	}
}

func rangeCorpus() *Request {
	return &Request{Cases: []Case{
		{ID: "plain", Prompt: "deploy the billing service to staging", ExpectTrigger: bp(true)},
		{ID: "fixtures", Prompt: "fix the failing build", ExpectTrigger: bp(true),
			Files: []Fixture{{Path: "main.go", Content: "package main\n\nfunc main() { println(\"hello\") }\n"}}},
		{ID: "rubric", Prompt: "write release notes", ExpectTrigger: bp(true), Rubric: "Mentions every merged change and stays under 200 words."},
	}}
}

func TestEstimateRun_RangeBracketsTheExpectedFigure(t *testing.T) {
	// Arrange
	price := Price{InPerMTok: 3, OutPerMTok: 15}

	// Act
	est := EstimateRun(rangeCorpus(), 500, 3, price, mustCounter(t))

	// Assert
	assert.Less(t, est.InputTokensLow, est.InputTokens)
	assert.Less(t, est.InputTokens, est.InputTokensHigh)
	assert.Less(t, est.OutputTokensLow, est.OutputTokens)
	assert.Less(t, est.OutputTokens, est.OutputTokensHigh)
	assert.Less(t, est.CostLowUSD, est.CostUSD)
	assert.Less(t, est.CostUSD, est.CostHighUSD)
	assert.Equal(t, est, EstimateRun(rangeCorpus(), 500, 3, price, mustCounter(t)), "deterministic")
}

func TestEstimateRun_PinsTheDocumentedMultipliers(t *testing.T) {
	// Arrange: one case, one run, no skill, no fixtures, a one-token-ish prompt
	req := &Request{Cases: []Case{{ID: "a", Prompt: "hi", ExpectTrigger: bp(true)}}}
	counter := mustCounter(t)
	in := harnessOverheadTokens + counter.Count("hi")

	// Act
	est := EstimateRun(req, 0, 1, Price{}, counter)

	// Assert: low = 0.8 in / 0.5 out, high = 1.3 in + 1.0 (in - overhead) / 3.0 out
	assert.Equal(t, in, est.InputTokens)
	assert.Equal(t, int(0.8*float64(in)+0.5), est.InputTokensLow)
	assert.Equal(t, int(1.3*float64(in)+float64(in-harnessOverheadTokens)+0.5), est.InputTokensHigh)
	assert.Equal(t, assumedOutputTokens/2, est.OutputTokensLow)
	assert.Equal(t, assumedOutputTokens*3, est.OutputTokensHigh)
}

func TestEstimateRun_AddKeepsTheRange(t *testing.T) {
	// Arrange
	a := EstimateRun(rangeCorpus(), 100, 1, Price{InPerMTok: 3, OutPerMTok: 15}, mustCounter(t))

	// Act
	sum := a.Add(a)

	// Assert
	assert.Equal(t, 2*a.InputTokensHigh, sum.InputTokensHigh)
	assert.InDelta(t, 2*a.CostLowUSD, sum.CostLowUSD, 1e-9)
	assert.InDelta(t, 2*a.CostHighUSD, sum.CostHighUSD, 1e-9)
}

// scaledRunner reports token counts derived from the same inputs the estimate
// uses, scaled by fixed factors, so the test is deterministic and offline.
func scaledRunner(t *testing.T, runs int, inFactor, outFactor float64) *fakeRunner {
	t.Helper()
	counter := mustCounter(t)
	return &fakeRunner{fn: func(req *Request) (*Response, error) {
		resp := &Response{Version: ProtocolVersion}
		for i := range req.Cases {
			c := &req.Cases[i]
			in := harnessOverheadTokens + counter.Count(c.Prompt)
			resp.Results = append(resp.Results, Result{
				Case: c.ID, Arm: ArmWith, Triggered: bp(c.Expects()), Output: "ok",
				InputTokens: int(inFactor * float64(in*runs)), OutputTokens: int(outFactor * float64(assumedOutputTokens*runs)),
			})
		}
		return resp, nil
	}}
}

func TestRun_RecordsEstimateAgainstActual(t *testing.T) {
	tests := []struct {
		name      string
		in, out   float64
		wantRange bool
	}{
		{"actual near the assumptions", 1.1, 0.9, true},
		{"cheaper than expected", 0.85, 0.6, true},
		{"far above the high figure", 4, 6, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := t.TempDir()
			writeSkill(t, cfg, "alpha", "---\nname: alpha\ndescription: a\n---\nbody\n", "cases:\n  - {id: one, prompt: do the thing, expect_trigger: true}\n  - {id: two, prompt: another thing, expect_trigger: true}\n")
			opts := baseOptions(cfg, scaledRunner(t, 3, tt.in, tt.out))
			opts.Ablation, opts.EstimateRuns, opts.Model = false, 3, "sonnet"

			// Act
			report, err := Run(context.Background(), opts)
			require.NoError(t, err)

			// Assert
			run := report.Skills[0]
			require.NotNil(t, run.EstimateVsActual)
			rec := run.EstimateVsActual
			assert.Greater(t, rec.ActualUSD, 0.0)
			require.NotNil(t, rec.InRange)
			assert.Equal(t, tt.wantRange, *rec.InRange)
			require.NotNil(t, rec.Error)
			if tt.wantRange {
				assert.InDelta(t, 0, *rec.Error, 0.35, "the expected figure is within the documented tolerance of the fake runner")
			}
			stored, ok := opts.Store.Get("alpha")
			require.True(t, ok)
			assert.Equal(t, rec, stored.Estimate, "the results store keeps the pair")
		})
	}
}

func TestRun_NoActualWhenTheRunnerReportsNoCost(t *testing.T) {
	// Arrange
	cfg := projectWithSkills(t)
	runner := &fakeRunner{fn: func(req *Request) (*Response, error) {
		resp := &Response{Version: ProtocolVersion}
		for i := range req.Cases {
			resp.Results = append(resp.Results, Result{Case: req.Cases[i].ID, Arm: ArmWith, Triggered: bp(req.Cases[i].Expects()), Output: "ok"})
		}
		return resp, nil
	}}
	opts := baseOptions(cfg, runner)
	opts.Ablation = false

	// Act
	report, err := Run(context.Background(), opts)
	require.NoError(t, err)

	// Assert
	rec := report.Skills[0].EstimateVsActual
	require.NotNil(t, rec)
	assert.Zero(t, rec.ActualUSD)
	assert.Nil(t, rec.Error)
	assert.Nil(t, rec.InRange)
}

func TestRun_MaxCostRefusesAModelWithoutAPrice(t *testing.T) {
	tests := []struct {
		name    string
		model   string
		price   Price
		maxCost float64
		wantErr bool
		known   bool
	}{
		{"unknown model with a cap", "mystery-9", Price{}, 5, true, false},
		{"unknown model, no cap", "mystery-9", Price{}, 0, false, false},
		{"unknown model, explicit price", "mystery-9", Price{InPerMTok: 1, OutPerMTok: 2}, 5, false, true},
		{"known model with a cap", "haiku", Price{}, 5, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := projectWithSkills(t)
			opts := baseOptions(cfg, goodRunner())
			opts.DryRun, opts.Model, opts.Price, opts.MaxCostUSD = true, tt.model, tt.price, tt.maxCost

			// Act
			report, err := Run(context.Background(), opts)

			// Assert
			if tt.wantErr {
				assert.ErrorContains(t, err, "no built-in price")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.known, report.PriceKnown)
		})
	}
}
