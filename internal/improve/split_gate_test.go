package improve

import (
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/evals"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func tr(b bool) *bool { return &b }

func TestSplitCases(t *testing.T) {
	cases := []evals.Case{
		{ID: "a", ExpectTrigger: tr(true), Tags: []string{"holdout"}, NearMiss: []string{"near a"}},
		{ID: "b", ExpectTrigger: tr(true)},
		{ID: "c", ExpectTrigger: tr(false), Tags: []string{"holdout", "x"}},
	}
	t.Run("tag wins", func(t *testing.T) {
		s := SplitCases(cases, "holdout", 0.9)
		assert.Equal(t, SplitByTag, s.Method)
		assert.Equal(t, []string{"a", "a.near-miss-1", "c"}, IDs(s.Held))
		assert.Equal(t, []string{"b"}, IDs(s.Train))
	})
	t.Run("fraction is deterministic and keeps near misses with their parent", func(t *testing.T) {
		var many []evals.Case
		for _, id := range []string{"c1", "c2", "c3", "c4", "c5", "c6", "c7", "c8", "c9", "c10"} {
			many = append(many, evals.Case{ID: id, ExpectTrigger: tr(true), NearMiss: []string{"n"}})
		}
		first, second := SplitCases(many, "holdout", 0.5), SplitCases(many, "holdout", 0.5)
		assert.Equal(t, SplitByFraction, first.Method)
		assert.Equal(t, IDs(first.Held), IDs(second.Held))
		assert.Equal(t, len(many), len(first.Held)+len(first.Train))
		assert.NotEmpty(t, first.Held)
		assert.NotEmpty(t, first.Train)
		for _, id := range IDs(first.Held) {
			parent, _, isNear := cutNear(id)
			if isNear {
				assert.Contains(t, IDs(first.Held), parent)
			}
		}
		assert.Empty(t, SplitCases(many, "holdout", 0).Held, "fraction 0 holds nothing out")
	})
	t.Run("duplicates are reported", func(t *testing.T) {
		held := []evals.Case{{ID: "h", Prompt: "Deploy  the API"}}
		train := []evals.Case{{ID: "t", Prompt: "deploy the api"}, {ID: "u", Prompt: "other"}}
		assert.Equal(t, []string{"t"}, DuplicatePrompts(train, held))
	})
}

func cutNear(id string) (parent string, n string, ok bool) {
	for i := len(id) - 1; i > 0; i-- {
		if id[i] == '.' && len(id) > i+11 && id[i+1:i+6] == "near-" {
			return id[:i], id[i+1:], true
		}
	}
	return id, "", false
}

func outcome(id string, pass bool, opts ...func(*CaseOutcome)) CaseOutcome {
	o := CaseOutcome{Case: id, Pass: pass, Expect: true, Triggered: true}
	for _, f := range opts {
		f(&o)
	}
	return o
}

func TestGate(t *testing.T) {
	neg := func(o *CaseOutcome) { o.Expect, o.Triggered = false, false }
	stolen := func(o *CaseOutcome) { o.Expect, o.Triggered, o.NearMiss = false, true, true }
	missed := func(o *CaseOutcome) { o.Triggered = false }
	unstable := func(o *CaseOutcome) { o.Unstable = true }
	tests := []struct {
		name     string
		base     []CaseOutcome
		cand     []CaseOutcome
		gate     Gate
		accept   bool
		decision string
	}{
		{"clear win", []CaseOutcome{outcome("a", false), outcome("b", true), outcome("c", true, neg)},
			[]CaseOutcome{outcome("a", true), outcome("b", true), outcome("c", true, neg)}, Gate{MinGain: 0.05}, true, "accepted"},
		{"below gain", []CaseOutcome{outcome("a", true), outcome("b", true), outcome("c", true, neg)},
			[]CaseOutcome{outcome("a", true), outcome("b", true), outcome("c", true, neg)}, Gate{MinGain: 0.05}, false, "rejected: below gain"},
		{"regression despite gain", []CaseOutcome{outcome("a", false), outcome("b", false), outcome("c", true), outcome("d", true, neg)},
			[]CaseOutcome{outcome("a", true), outcome("b", true), outcome("c", false), outcome("d", true, neg)}, Gate{MinGain: 0.05}, false, "rejected: regression"},
		{"one regression allowed", []CaseOutcome{outcome("a", false), outcome("b", false), outcome("c", true), outcome("d", true, neg)},
			[]CaseOutcome{outcome("a", true), outcome("b", true), outcome("c", false), outcome("d", true, neg)}, Gate{MinGain: 0.05, MaxRegressions: 1}, true, "accepted"},
		{"unstable cases are not wins", []CaseOutcome{outcome("a", false, unstable), outcome("b", true), outcome("c", true, neg)},
			[]CaseOutcome{outcome("a", true, unstable), outcome("b", true), outcome("c", true, neg)}, Gate{MinGain: 0}, false, "rejected: below gain"},
		{"an unstable pass-to-fail flip is still a regression",
			[]CaseOutcome{outcome("a", false), outcome("b", false), outcome("c", true), outcome("d", true, neg)},
			[]CaseOutcome{outcome("a", true), outcome("b", true), outcome("c", false, unstable), outcome("d", true, neg)}, Gate{MinGain: 0.05}, false, "rejected: regression"},
		{"zero gain with min-gain 0 is no evidence", []CaseOutcome{outcome("a", true), outcome("b", true), outcome("c", true, neg)},
			[]CaseOutcome{outcome("a", true), outcome("b", true), outcome("c", true, neg)}, Gate{MinGain: 0}, false, "rejected: below gain"},
		{"min-gain 0 still takes a real win", []CaseOutcome{outcome("a", false), outcome("b", true), outcome("c", true, neg)},
			[]CaseOutcome{outcome("a", true), outcome("b", true), outcome("c", true, neg)}, Gate{MinGain: 0}, true, "accepted"},
		{"near-miss false positive", []CaseOutcome{outcome("a", false), outcome("n", true, neg)},
			[]CaseOutcome{outcome("a", true), outcome("n", true, stolen)}, Gate{MinGain: 0.05}, false, "rejected: regression"},
		{"recall drop", []CaseOutcome{outcome("a", false), outcome("b", true), outcome("n", true, neg)},
			[]CaseOutcome{outcome("a", true), outcome("b", true, missed), outcome("n", true, neg)}, Gate{MinGain: 0.05, MaxRegressions: 5}, false, "rejected: regression"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			cmp := Compare(tt.base, tt.cand)
			v := tt.gate.Decide(cmp)

			// Assert
			assert.Equal(t, tt.accept, v.Accept, "%v", v.Reasons)
			assert.Equal(t, tt.decision, v.Decision())
		})
	}
}

func TestCompare_PairsOnCommonCasesAndFlagsUnderpowered(t *testing.T) {
	// Act
	cmp := Compare([]CaseOutcome{outcome("a", false), outcome("only-base", true)}, []CaseOutcome{outcome("a", true), outcome("only-cand", true)})

	// Assert
	require.Len(t, cmp.Table, 1)
	assert.Equal(t, []string{"a"}, cmp.Wins)
	assert.InDelta(t, 1.0, cmp.Gain, 1e-9)
	assert.True(t, cmp.Underpowered)
}

func TestGate_RequiresEvidence(t *testing.T) {
	tests := []struct {
		name string
		base []CaseOutcome
		cand []CaseOutcome
		text string
	}{
		{"no overlap", []CaseOutcome{outcome("a", true)}, []CaseOutcome{outcome("b", true)}, "no held-out case"},
		{"nothing scored", nil, nil, "no held-out case"},
		{"candidate skipped a case", []CaseOutcome{outcome("a", false), outcome("b", true)}, []CaseOutcome{outcome("a", true)}, "skipped"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			cmp := Compare(tt.base, tt.cand)
			v := Gate{MinGain: 0, MaxRegressions: 10}.Decide(cmp)

			// Assert
			assert.False(t, v.Accept)
			assert.Contains(t, strings.Join(v.Reasons, ";"), tt.text)
		})
	}
	t.Run("a skipped case is a loss", func(t *testing.T) {
		cmp := Compare([]CaseOutcome{outcome("a", true), outcome("b", true)}, []CaseOutcome{outcome("a", true)})
		assert.Equal(t, []string{"b"}, cmp.Losses)
	})
}
