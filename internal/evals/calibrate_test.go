package evals

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// estimateRecord builds a record the way a run does: an estimate under the default
// assumptions next to what the runner reported.
func estimateRecord(runs, expIn, expOut, actIn, actOut int, errRatio float64) *EstimateRecord {
	e := errRatio
	return &EstimateRecord{
		AgentRuns: runs, ExpectedInputTokens: expIn, ExpectedOutputTokens: expOut, ActualInputTokens: actIn, ActualOutputTokens: actOut,
		ExpectedTokens: expIn + expOut, ActualTokens: actIn + actOut, Error: &e,
	}
}

func storeWith(skills ...SkillRecord) *Store {
	s := NewStore()
	for _, r := range skills {
		s.Put(r)
	}
	return s
}

func TestCalibrate_MovesTheOverheadAndOutputByTheMedianGapPerRun(t *testing.T) {
	// Arrange: five agent runs per skill; the harness really needs 25,000 overhead
	// (the estimate assumed 2,000) and answers in 100 tokens (assumed 600).
	rec := func(id string, extraIn int) SkillRecord {
		return SkillRecord{ID: id, Harness: "claude", Model: "haiku", Estimate: estimateRecord(5, 5*2500, 5*600, 5*(2500+23000+extraIn), 5*100, 8.0)}
	}
	store := storeWith(rec("a", 0), rec("b", 500), rec("c", -500))

	// Act
	cal := Calibrate(store, &CalibrateOptions{})

	// Assert
	require.Len(t, cal.Groups, 1)
	g := cal.Groups[0]
	assert.Equal(t, KindCases, g.Kind)
	assert.Equal(t, 3, g.Samples)
	assert.False(t, g.LowConfidence)
	assert.Equal(t, harnessOverheadTokens, g.Current.OverheadTokens)
	assert.Equal(t, 25000, g.Proposed.OverheadTokens, "the median of 25,000 and 25,000 plus and minus 100 per run")
	assert.Equal(t, 100, g.Proposed.AssumedOutputTokens)
	assert.Greater(t, g.TokenErrorBefore, 3.0)
	assert.InDelta(t, 0, g.TokenErrorAfter, 0.05, "applying the proposal removes the median token error")
	assert.InDelta(t, 8.0, g.CostErrorMedian, 1e-9)
}

func TestCalibrate_ActivationGroupsFitTheActivationOutput(t *testing.T) {
	rec := SkillRecord{ID: "a", Harness: "claude", Model: "haiku", Activation: &ActivationRecord{Harness: "claude", Model: "haiku",
		Estimate: estimateRecord(20, 20*3000, 20*150, 20*26000, 20*40, 6.0)}}
	cal := Calibrate(storeWith(rec), &CalibrateOptions{})

	require.Len(t, cal.Groups, 1)
	g := cal.Groups[0]
	assert.Equal(t, KindActivation, g.Kind)
	assert.Equal(t, 40, g.Proposed.ActivationOutputTokens)
	assert.Equal(t, 150, g.Current.ActivationOutputTokens)
	assert.Zero(t, g.Proposed.AssumedOutputTokens)
	assert.True(t, g.LowConfidence, "one run is too few to trust")
	assert.Equal(t, 26000-3000+2000, g.Proposed.OverheadTokens, "the gap per run is added to the overhead the estimate assumed")
}

func TestCalibrate_UsesTheAssumptionsARecordWasMadeUnder(t *testing.T) {
	rec := estimateRecord(4, 4*30000, 4*600, 4*30000, 4*600, 0)
	rec.Params = &EstimateParams{OverheadTokens: 28000}
	cal := Calibrate(storeWith(SkillRecord{ID: "a", Harness: "claude", Estimate: rec}), &CalibrateOptions{})

	g := cal.Groups[0]
	assert.Equal(t, 28000, g.Current.OverheadTokens)
	assert.Equal(t, 28000, g.Proposed.OverheadTokens, "the estimate was already right")
	assert.Zero(t, g.TokenErrorAfter)
}

func TestCalibrate_KeepsModelsAndHarnessesApart(t *testing.T) {
	haiku := SkillRecord{ID: "a", Harness: "claude", Model: "haiku", Estimate: estimateRecord(2, 4000, 1200, 9000, 400, 1)}
	opus := SkillRecord{ID: "b", Harness: "claude", Model: "opus", Estimate: estimateRecord(2, 4000, 1200, 20000, 800, 4)}
	codex := SkillRecord{ID: "c", Harness: "codex", Model: "haiku", Estimate: estimateRecord(2, 4000, 1200, 6000, 400, 0.3)}
	store := storeWith(haiku, opus, codex)

	all := Calibrate(store, &CalibrateOptions{})
	onlyOpus := Calibrate(store, &CalibrateOptions{Model: "opus"})
	onlyCodex := Calibrate(store, &CalibrateOptions{Harness: "codex"})

	require.Len(t, all.Groups, 3)
	assert.Less(t, all.Groups[0].Proposed.OverheadTokens, all.Groups[1].Proposed.OverheadTokens)
	require.Len(t, onlyOpus.Groups, 1)
	assert.Equal(t, "opus", onlyOpus.Groups[0].Model)
	assert.Equal(t, 2, onlyOpus.Skipped.Filtered)
	require.Len(t, onlyCodex.Groups, 1)
	assert.Equal(t, "codex", onlyCodex.Groups[0].Harness)
}

func TestCalibrate_SkipsRecordsItCannotUse(t *testing.T) {
	noSplit := SkillRecord{ID: "a", Harness: "claude", Estimate: &EstimateRecord{AgentRuns: 3, ExpectedInputTokens: 100}}
	noRuns := SkillRecord{ID: "b", Harness: "claude", Estimate: &EstimateRecord{ActualInputTokens: 100}}
	good := SkillRecord{ID: "c", Harness: "claude", Estimate: estimateRecord(2, 4000, 1200, 9000, 400, 1)}
	store := storeWith(noSplit, noRuns, good)
	unverified := NewStore()
	unverified.Skills = []SkillRecord{{ID: "u", Harness: "claude", Estimate: estimateRecord(2, 4000, 1200, 9000, 400, 1)}}

	cal := Calibrate(store, &CalibrateOptions{})
	forged := Calibrate(unverified, &CalibrateOptions{})

	require.Len(t, cal.Groups, 1)
	assert.Equal(t, 2, cal.Skipped.NoUsage)
	assert.Empty(t, forged.Groups, "a record not signed with the user's key is not evidence")
	assert.Equal(t, 1, forged.Skipped.Unverified)
}

func TestCalibrate_Deterministic(t *testing.T) {
	store := storeWith(
		SkillRecord{ID: "b", Harness: "claude", Model: "sonnet", Estimate: estimateRecord(2, 4000, 1200, 9000, 400, 1)},
		SkillRecord{ID: "a", Harness: "claude", Model: "haiku", Estimate: estimateRecord(2, 4000, 1200, 9000, 400, 1)},
	)

	var first, second bytes.Buffer
	require.NoError(t, Calibrate(store, &CalibrateOptions{}).WriteText(&first))
	require.NoError(t, Calibrate(store, &CalibrateOptions{}).WriteText(&second))

	assert.Equal(t, first.String(), second.String())
	assert.Contains(t, first.String(), "[lint.evals.estimate]")
	assert.Contains(t, first.String(), "overhead_tokens = ")
}

func TestCalibrate_EmptyStoreSaysSo(t *testing.T) {
	var out bytes.Buffer

	require.NoError(t, Calibrate(NewStore(), &CalibrateOptions{}).WriteText(&out))

	assert.Contains(t, out.String(), "No recorded run has a token split")
}

func TestQuantile(t *testing.T) {
	tests := []struct {
		name string
		in   []float64
		q    float64
		want float64
	}{
		{"empty", nil, 0.5, 0},
		{"one", []float64{4}, 0.9, 4},
		{"odd median", []float64{3, 1, 2}, 0.5, 2},
		{"even median interpolates", []float64{1, 2, 3, 4}, 0.5, 2.5},
		{"p90", []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, 0.9, 9.1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.InDelta(t, tt.want, quantile(tt.in, tt.q), 1e-9)
		})
	}
}

// TestCalibration_ShrinksTheErrorOfTheNextEstimate runs a native activation through a
// fake runner whose usage is far from the defaults, calibrates from the stored
// record, and shows the next estimate of the same run is much closer.
func TestCalibration_ShrinksTheErrorOfTheNextEstimate(t *testing.T) {
	// Arrange
	cfg := activationProject(t)
	price, _ := PriceFor("haiku")
	perRun := (26000*price.InPerMTok + 40*price.OutPerMTok) / 1e6
	runner := nativeRunner{&fakeRunner{fn: func(req *Request) (*Response, error) {
		resp := &Response{Version: ProtocolVersion}
		for i := range req.Cases {
			c := &req.Cases[i]
			resp.Results = append(resp.Results, Result{Case: c.ID, Arm: ArmWith, Runs: req.Runs, FiredCounts: map[string]int{"deploy-staging": req.Runs},
				InputTokens: 26000 * req.Runs, OutputTokens: 40 * req.Runs, CostUSD: perRun * float64(req.Runs)})
		}
		return resp, nil
	}}}
	store := NewStore()
	opts := nativeOptions(cfg, runner)
	opts.Store = store
	first := mustActivation(t, opts)
	require.NotNil(t, first.Skills[0].EstimateVsActual)
	before := first.Skills[0].EstimateVsActual

	// Act
	cal := Calibrate(store, &CalibrateOptions{})
	require.Len(t, cal.Groups, 1)
	opts.Force = true
	opts.Params = cal.Groups[0].Proposed
	second := mustActivation(t, opts)
	after := second.Skills[0].EstimateVsActual

	// Assert
	require.NotNil(t, before.Error)
	require.NotNil(t, after.Error)
	assert.Greater(t, *before.Error, 3.0, "the default estimate is far below a 26k-token harness")
	assert.Less(t, absf(*after.Error), 0.35, "the calibrated estimate is within the documented tolerance")
	assert.True(t, *after.InRange, "and the actual cost falls inside [low, high]")
}

func absf(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
