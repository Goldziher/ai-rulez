package llm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJudge_CompletionBudget(t *testing.T) {
	tests := []struct {
		name string
		opts JudgeOptions
		want int
	}{
		{name: "the default leaves room for a thinking model", opts: JudgeOptions{}, want: DefaultJudgeCompletionTokens},
		{name: "a larger floor is honored", opts: JudgeOptions{MinCompletionTokens: 8000}, want: 8000},
		{name: "a smaller floor never goes below the verdict itself", opts: JudgeOptions{MinCompletionTokens: 50}, want: DefaultJudgeCompletionTokens},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			fake := &Fake{ChatFunc: func(ChatRequest) (string, error) { return `{"score":1,"rationale":"ok"}`, nil }}

			// Act
			_, err := JudgeWith(context.Background(), fake, "rubric", "transcript", tt.opts)

			// Assert
			require.NoError(t, err)
			calls := fake.ChatCalls()
			require.Len(t, calls, 1)
			assert.Equal(t, tt.want, calls[0].MaxTokens)
		})
	}
}

func TestJudge_PlainJudgeUsesTheDefaultBudget(t *testing.T) {
	fake := &Fake{ChatFunc: func(ChatRequest) (string, error) { return `{"score":0.5,"rationale":"half"}`, nil }}

	_, err := Judge(context.Background(), fake, "rubric", "transcript")

	require.NoError(t, err)
	assert.Equal(t, DefaultJudgeCompletionTokens, fake.ChatCalls()[0].MaxTokens)
}
