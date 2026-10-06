package jsonmerge_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
)

// TestUnmerge_HandWrittenDuplicateOfAClaimedElementSurvives pins that a claim
// is a multiset: ai-rulez wrote one "x", so one "x" leaves and the copy the user
// wrote stays, in strict JSON and in JSONC.
func TestUnmerge_HandWrittenDuplicateOfAClaimedElementSurvives(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		want string
	}{
		{
			name: "strict json",
			doc:  "{\n  \"list\": [\n    \"x\",\n    \"mine\",\n    \"x\"\n  ]\n}\n",
			want: "{\n  \"list\": [\n    \"mine\",\n    \"x\"\n  ]\n}\n",
		},
		{
			name: "jsonc with comments",
			doc:  "{\n  // keep\n  \"list\": [\n    \"x\",\n    \"mine\",\n    \"x\"\n  ]\n}\n",
			want: "{\n  // keep\n  \"list\": [\n    \"mine\",\n    \"x\"\n  ]\n}\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			claims := []jsonmerge.Claim{{Path: []string{"list"}, Elements: []any{"x"}}}

			// Act
			result, err := jsonmerge.UnmergeDocument("settings.json", tt.doc, claims)

			// Assert
			require.NoError(t, err)
			assert.True(t, result.Changed)
			assert.Equal(t, tt.want, result.Body)
		})
	}
}

func TestElementsIn_ReturnsOnlyAsManyCopiesAsClaimed(t *testing.T) {
	// Arrange
	claim := jsonmerge.Claim{Path: []string{"list"}, Elements: []any{"x"}}

	// Act
	owned := claim.ElementsIn([]any{"x", "y", "x"})

	// Assert
	assert.Equal(t, []any{"x"}, owned)
}
