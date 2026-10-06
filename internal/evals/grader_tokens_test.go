package evals

import (
	"context"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A reasoning model spends the judge's 300-token completion budget on thinking and
// the verdict is cut off, so the grader raises the budget.
func TestJudgeGrader_RaisesTheCompletionBudgetOfTheJudgeCall(t *testing.T) {
	tests := []struct {
		name  string
		floor int
		want  int
	}{
		{name: "default", floor: 0, want: DefaultJudgeCompletionTokens},
		{name: "explicit", floor: 4096, want: 4096},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			fake := llm.NewFake()
			var got int
			fake.ChatFunc = func(req llm.ChatRequest) (string, error) {
				got = req.MaxTokens
				return `{"score":1,"rationale":"ok"}`, nil
			}

			// Act
			_, err := (&JudgeGrader{Client: fake, MinCompletionTokens: tt.floor}).Grade(context.Background(), "r", "t")

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCompletionFloor_NeverLowersABiggerBudget(t *testing.T) {
	fake := llm.NewFake()
	var got int
	fake.ChatFunc = func(req llm.ChatRequest) (string, error) {
		got = req.MaxTokens
		return "x", nil
	}
	c := completionFloor{Client: fake, min: 100}

	_, err := c.Chat(context.Background(), llm.ChatRequest{MaxTokens: 5000})
	require.NoError(t, err)
	assert.Equal(t, 5000, got)

	_, err = c.Chat(context.Background(), llm.ChatRequest{MaxTokens: 10})
	require.NoError(t, err)
	assert.Equal(t, 100, got)
}
