package skillsearch

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func calibrationCases() *CaseFile {
	return &CaseFile{Version: 1, Cases: []Case{
		{ID: "p1", Query: "money back", Expect: rel("issue-refund")},
		{ID: "p2", Query: "roll out to the cluster", Expect: rel("deploy-staging")},
		{ID: "p3", Query: "chargeback from the bank", Expect: rel("dispute-charge")},
		{ID: "n1", Query: "weather today", Expect: nil},
		{ID: "n2", Query: "sourdough recipe", Expect: nil},
	}}
}

func TestEval_CalibratesTheAbstentionThreshold(t *testing.T) {
	t.Parallel()
	// Arrange
	env := hybridEnv(t)

	// Act
	res := mustEval(t, env, calibrationCases(), 3, ModeVector)

	// Assert: a threshold exists that keeps every positive and abstains on every negative
	require.NotNil(t, res.Calibration)
	c := res.Calibration
	assert.Equal(t, ModeVector, c.Mode)
	assert.Equal(t, 3, c.Positives)
	assert.Equal(t, 2, c.Negatives)
	assert.Equal(t, 3, c.PositivesKept)
	assert.Equal(t, 2, c.NegativesAbstained)
	assert.Greater(t, c.MinSim, 0.0)
	for _, n := range res.Negatives {
		assert.Less(t, n.TopSim, c.MinSim)
	}
	for _, p := range res.Cases {
		assert.GreaterOrEqual(t, p.TopSim, c.MinSim)
	}
}

func TestEval_NoCalibrationWithoutBothKindsOfCase(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		cases []Case
		modes []string
	}{
		{"no negatives", calibrationCases().Cases[:3], []string{ModeVector}},
		{"no positives", calibrationCases().Cases[3:], []string{ModeVector}},
		{"lexical only", calibrationCases().Cases, []string{ModeLexical}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := hybridEnv(t)

			res := mustEval(t, env, &CaseFile{Version: 1, Cases: tt.cases}, 3, tt.modes...)

			assert.Nil(t, res.Calibration)
		})
	}
}

func TestEval_ConfiguredThresholdMakesNegativesAbstain(t *testing.T) {
	t.Parallel()
	// Arrange
	env := hybridEnv(t)
	env.Cfg.VectorMinSim = 0.5

	// Act
	res := mustEval(t, env, calibrationCases(), 3, ModeVector)

	// Assert
	require.Len(t, res.Negatives, 2)
	for _, n := range res.Negatives {
		assert.True(t, n.Abstained, n.ID)
		assert.Empty(t, n.Top)
	}
	m := res.Modes[ModeVector]
	require.NotNil(t, m.AbstainRate)
	assert.InDelta(t, 1.0, *m.AbstainRate, 1e-9)
	assert.InDelta(t, 1.0, m.HitAt, 1e-9, "the positives still answer")
}

func TestCalibrate_PicksTheBestSeparatingThreshold(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		pos, neg  []float64
		wantMin   float64
		wantKept  int
		wantAbst  int
		wantFound bool
	}{
		{"clean gap", []float64{0.8, 0.7, 0.9}, []float64{0.2, 0.3}, 0.5, 3, 2, true},
		{"a tie keeps more positives answered", []float64{0.8, 0.45, 0.9}, []float64{0.5, 0.2}, 0.325, 3, 1, true},
		{"negatives above every positive abstain on nothing useful", []float64{0.3, 0.4}, []float64{0.8, 0.9}, 0, 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			c := calibrate(tt.pos, tt.neg)

			// Assert
			if !tt.wantFound {
				assert.Nil(t, c)
				return
			}
			require.NotNil(t, c)
			assert.InDelta(t, tt.wantMin, c.MinSim, 0.0006)
			assert.Equal(t, tt.wantKept, c.PositivesKept)
			assert.Equal(t, tt.wantAbst, c.NegativesAbstained)
		})
	}
}
