package review

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
)

// TestRunNoteCodeFiresOnlyForUnscoredItems covers AR9G0: an item that is withheld
// is reported as a run note, and a scored item produces none.
func TestRunNoteCodeFiresOnlyForUnscoredItems(t *testing.T) {
	leak := skill("leak", "Use when rotating keys")
	tests := []struct {
		name     string
		findings []lint.Finding
		wantNote bool
	}{
		{"withheld item", []lint.Finding{finding("AR001", lint.SeverityError, leak, 3, "key")}, true},
		{"clean item", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			rb := builtin(t)

			// Act
			var notes []Finding
			for _, f := range Run(Input{Rubric: rb, Items: []Item{leak}, Findings: tt.findings}).Findings(rb) {
				if f.Code == "AR9G0" {
					notes = append(notes, f)
				}
			}

			// Assert
			if !tt.wantNote {
				assert.Empty(t, notes)
				return
			}
			require.Len(t, notes, 1)
			assert.Equal(t, "skill:leak", notes[0].ItemID)
			assert.Equal(t, string(lint.SeverityInfo), notes[0].Severity)
		})
	}
}

// TestDimensionCodesFireFromTheJudge covers AR9G3, AR9G5 and AR9G6: each is reported
// when the judge fails its dimension and none is reported when the judge passes all.
func TestDimensionCodesFireFromTheJudge(t *testing.T) {
	tests := []struct {
		code      string
		dimension string
	}{
		{"AR9G3", "body-accuracy"},
		{"AR9G5", "scope-creep"},
		{"AR9G6", "instruction-conflict"},
	}
	for _, tt := range tests {
		for _, failing := range []bool{true, false} {
			name := tt.code + " quiet on a passing judge"
			if failing {
				name = tt.code + " fires when " + tt.dimension + " fails"
			}
			t.Run(name, func(t *testing.T) {
				// Arrange
				rb, res := twoSkills(t)
				sj := &scriptedJudge{verdict: func(_, dim string, _ int) string {
					return pick(failing && dim == tt.dimension, VerdictFail)
				}}
				client, _ := newClient(t, sj, llm.Config{})
				mustRun(t, SemanticInput{Rubric: rb, Results: res, Options: SemanticOptions{Client: client, K: 3, Content: config.ReviewContentFull}})

				// Act
				var got []Finding
				for _, f := range res.Findings(rb) {
					if f.Code == tt.code {
						got = append(got, f)
					}
				}

				// Assert
				if !failing {
					assert.Empty(t, got)
					return
				}
				require.NotEmpty(t, got)
				for _, f := range got {
					assert.Equal(t, OriginLLMJudge, f.Origin)
					assert.Equal(t, tt.dimension, f.Dimension)
					assert.Equal(t, "warning", f.Severity)
				}
			})
		}
	}
}
