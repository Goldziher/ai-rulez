package adapter

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/improve"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
)

// fixerPrompts records the user message of every fixer call and answers with the default edit.
func fixerPrompts(sm *scriptedModel, into *[]string) {
	sm.fix = func(req llm.ChatRequest) string {
		*into = append(*into, req.Messages[len(req.Messages)-1].Content)
		b, _ := json.Marshal(map[string]any{"edits": []map[string]string{{"old": "Helps with deployments", "new": fixedDesc}}, "note": "named the trigger"})
		return string(b)
	}
}

func TestReviewFix_UsesThePreviousRoundOfTheRequest(t *testing.T) {
	// Arrange: round 2 of a loop whose first attempt was rejected before the held-out set was consulted
	ws := workspaceWith(t, vagueSkill)
	sm := &scriptedModel{}
	var prompts []string
	fixerPrompts(sm, &prompts)
	in := requestJSON(t, func(r *improve.OptimizerRequest) {
		r.Round = 2
		r.Previous = &improve.PreviousRound{
			Round: 1, Decision: "rejected: regression", WorkspaceKept: false,
			Reasons: []string{"regression: AR9J4 sibling rollback lost trigger recall 100% -> 50% (stolen prompt(s): rb-one)"},
			Summary: "review-fix: widened the trigger",
		}
	})

	// Act
	resp, err := serve(t, ReviewFix, ws, in, reviewFixOptions(sm))

	// Assert: the fixer was told, from its first attempt, how the last round ended and why
	require.NoError(t, err)
	assert.Equal(t, []string{"SKILL.md"}, resp.Changed)
	require.NotEmpty(t, prompts)
	first := prompts[0]
	for _, want := range []string{"Round 1 ended: rejected: regression", "sibling rollback lost trigger recall", "widened the trigger", "reset to the state before"} {
		assert.Contains(t, first, want)
	}
}

func TestReviewFix_FirstRoundHasNoFeedback(t *testing.T) {
	// Arrange
	ws := workspaceWith(t, vagueSkill)
	sm := &scriptedModel{}
	var prompts []string
	fixerPrompts(sm, &prompts)

	// Act
	_, err := serve(t, ReviewFix, ws, requestJSON(t, nil), reviewFixOptions(sm))

	// Assert
	require.NoError(t, err)
	require.NotEmpty(t, prompts)
	assert.NotContains(t, prompts[0], "FEEDBACK")
}

func TestPreviousRoundFeedback(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		prev *improve.PreviousRound
		want []string
		none bool
	}{
		{"no previous round", nil, nil, true},
		{"a kept workspace says the attempt is still in the file", &improve.PreviousRound{Round: 2, Decision: "rejected: below gain", WorkspaceKept: true},
			[]string{"Round 2 ended: rejected: below gain", "still holds that attempt"}, false},
		{"a reset workspace says so", &improve.PreviousRound{Round: 1, Decision: "rejected: policy", WorkspaceKept: false},
			[]string{"reset to the state before it"}, false},
		{"reasons and summary are listed", &improve.PreviousRound{Round: 1, Decision: "rejected: policy", Reasons: []string{"a", "b"}, Summary: "s"},
			[]string{"Reasons:", "- a", "- b", "Its own summary: s"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := previousRoundFeedback(tt.prev)

			// Assert
			if tt.none {
				assert.Empty(t, got)
				return
			}
			for _, w := range tt.want {
				assert.Contains(t, got, w)
			}
			assert.False(t, strings.Contains(got, "held-out"), "nothing about held-out cases is invented here")
		})
	}
}
