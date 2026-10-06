package improve

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

func pairs(n, wins, losses int) []PairRow {
	rows := make([]PairRow, n)
	for i := range rows {
		rows[i] = PairRow{Case: fmt.Sprintf("c%02d", i), Base: true, Cand: true}
		switch {
		case i < wins:
			rows[i].Base = false
		case i < wins+losses:
			rows[i].Cand = false
		}
	}
	return rows
}

func TestBootstrapGain(t *testing.T) {
	tests := []struct {
		name        string
		rows        []PairRow
		wantNil     bool
		wantZero    bool
		wantLowPos  bool
		wantHighNeg bool
	}{
		{name: "no pairs", wantNil: true},
		{name: "all wins excludes zero", rows: pairs(20, 20, 0), wantLowPos: true},
		{name: "no change is exactly zero", rows: pairs(10, 0, 0), wantZero: true},
		{name: "one win in six includes zero", rows: pairs(6, 1, 0), wantZero: true},
		{name: "many losses excludes zero from above", rows: pairs(20, 0, 20), wantHighNeg: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			ci := BootstrapGain(tt.rows, 2000)

			// Assert
			if tt.wantNil {
				assert.Nil(t, ci)
				return
			}
			require.NotNil(t, ci)
			assert.Equal(t, tt.wantZero, ci.IncludesZero(), "%+v", ci)
			assert.LessOrEqual(t, ci.Low, ci.High)
			if tt.wantLowPos {
				assert.Greater(t, ci.Low, 0.0)
			}
			if tt.wantHighNeg {
				assert.Less(t, ci.High, 0.0)
			}
		})
	}
}

func TestBootstrapGain_IsDeterministicAndOrderedByTheData(t *testing.T) {
	// Arrange
	rows := pairs(12, 5, 1)

	// Act
	a, b := BootstrapGain(rows, BootstrapResamples), BootstrapGain(rows, BootstrapResamples)
	shifted := BootstrapGain(pairs(12, 6, 1), BootstrapResamples)

	// Assert
	require.NotNil(t, a)
	assert.Equal(t, a, b, "the same run reports the same interval")
	assert.Equal(t, BootstrapResamples, a.Resamples)
	assert.InDelta(t, 4.0/12, (a.Low+a.High)/2, 0.2, "the interval is centred near the observed gain")
	assert.Greater(t, shifted.High, a.Low)
}

func TestCompare_ReportsTheIntervalAndFlagsAnInterval_ThatIncludesZero(t *testing.T) {
	// Arrange: eight cases (not underpowered by size), one win.
	base := make([]CaseOutcome, 8)
	cand := make([]CaseOutcome, 8)
	for i := range base {
		id := fmt.Sprintf("c%d", i)
		base[i] = CaseOutcome{Case: id, Pass: true, Triggered: true, Expect: true}
		cand[i] = base[i]
	}
	base[0].Pass = false

	// Act
	cmp := Compare(base, cand)

	// Assert
	require.NotNil(t, cmp.CI)
	assert.True(t, cmp.CI.IncludesZero())
	assert.True(t, cmp.Underpowered, "an interval through zero is weak evidence even with eight cases")
	assert.Contains(t, underpoweredWarning(&cmp), "bootstrap interval")
}

func TestGate_RequireCIAboveZero(t *testing.T) {
	tests := []struct {
		name    string
		rows    []PairRow
		require bool
		accept  bool
		decide  string
	}{
		{name: "off by default accepts a weak gain", rows: pairs(6, 1, 0), accept: true, decide: "accepted"},
		{name: "on rejects an interval through zero", rows: pairs(6, 1, 0), require: true, decide: "rejected: below confidence"},
		{name: "on accepts a clear gain", rows: pairs(20, 14, 0), require: true, accept: true, decide: "accepted"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			base, cand := outcomesOf(tt.rows)
			cmp := Compare(base, cand)

			// Act
			v := Gate{MinGain: 0.05, RequireCIAboveZero: tt.require}.Decide(cmp)

			// Assert
			assert.Equal(t, tt.accept, v.Accept, "%v", v.Reasons)
			assert.Equal(t, tt.decide, v.Decision())
		})
	}
}

func outcomesOf(rows []PairRow) (base, cand []CaseOutcome) {
	for _, r := range rows {
		base = append(base, CaseOutcome{Case: r.Case, Pass: r.Base, Triggered: true, Expect: true})
		cand = append(cand, CaseOutcome{Case: r.Case, Pass: r.Cand, Triggered: true, Expect: true})
	}
	return base, cand
}

func TestBootstrapGain_UnstableRowsAreNeitherWinsNorLosses(t *testing.T) {
	// Arrange: ten cases whose only pass-to-fail flips are unstable, so a "gain" there is run-to-run noise.
	rows := pairs(10, 10, 0)
	for i := range rows {
		rows[i].Unstable = true
	}

	// Act
	ci := BootstrapGain(rows, 2000)

	// Assert: the interval is exactly zero, as for no change, instead of claiming a clear gain.
	require.NotNil(t, ci)
	assert.InDelta(t, 0.0, ci.Low, 1e-9)
	assert.InDelta(t, 0.0, ci.High, 1e-9)
}

func TestExecute_UnderpoweredWarningOnlyForAnAcceptedRound(t *testing.T) {
	// Arrange: a candidate nothing improves (rejected), and one that wins (accepted) on a 3-case held-out set.
	tests := []struct {
		name     string
		appended string
		warns    bool
	}{
		{"rejected round", "\nno effect\n", false},
		{"accepted round", "\nGOOD advice.\n", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, configDir := project(t)
			opt := optimizer(t, func(dir string, _ *OptimizerRequest, _ runner.Spec) { appendSkill(t, dir, tt.appended) })
			o := baseOptions(root, configDir, goodEval(), opt)
			o.MaxRounds = 1
			plan := mustPrepare(t, &o)

			// Act
			report, err := plan.Execute(context.Background())

			// Assert
			require.NoError(t, err)
			require.Len(t, report.Rounds, 1)
			assert.Equal(t, tt.warns, strings.Contains(strings.Join(report.Rounds[0].Warnings, ";"), CodeUnderpowered), "%v", report.Rounds[0].Warnings)
		})
	}
}
