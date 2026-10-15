package pricing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBedrockInferenceProfileIDsArePricedLikeTheirModel(t *testing.T) {
	tests := []struct {
		name, id, plain string
	}{
		{"global profile", "bedrock/global.anthropic.claude-sonnet-5-5", "claude-sonnet-5-5"},
		{"global haiku", "global.anthropic.claude-haiku-5-5", "claude-haiku-5-5"},
		{"regional profile with date and revision", "eu.anthropic.claude-haiku-4-5-20251001-v1:0", "claude-haiku-4-5-20251001"},
		{"bare vendor id", "bedrock/anthropic.claude-sonnet-4-20250514-v1:0", "claude-sonnet-4-20250514"},
		{"us-gov profile", "us-gov.anthropic.claude-sonnet-5-5", "claude-sonnet-5-5"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			want, wantKnown := Lookup(tc.plain)
			tok := Tokens{Prompt: 1000, Completion: 500}
			wantCost, _ := Cost(tc.plain, tok)

			// Act
			got, known := Lookup(tc.id)
			cost, costKnown := Cost(tc.id, tok)

			// Assert
			assert.True(t, wantKnown, "the plain name must be priced for this test to mean anything")
			assert.True(t, known)
			assert.Equal(t, want, got)
			assert.True(t, costKnown)
			assert.InDelta(t, wantCost, cost, 1e-12)
		})
	}
}

func TestUnknownBedrockModelStaysUnpriced(t *testing.T) {
	_, known := Lookup("bedrock/global.anthropic.not-a-model")
	assert.False(t, known)
	_, known = Cost("bedrock/zz.vendor.model-v1:0", Tokens{Prompt: 1})
	assert.False(t, known)
}
