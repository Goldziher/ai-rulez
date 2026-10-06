package yamlmerge_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/generator/yamlmerge"
)

func TestUnmerge_HandWrittenDuplicateOfAClaimedElementSurvives(t *testing.T) {
	// Arrange: ai-rulez claimed one "x"; the document holds two.
	doc := "list:\n  - x # mine\n  - y\n  - x\n"
	claims := []yamlmerge.Claim{{Path: []string{"list"}, Elements: []any{"x"}}}

	// Act
	result, err := yamlmerge.UnmergeDocument("config.yaml", doc, claims)

	// Assert
	require.NoError(t, err)
	assert.True(t, result.Changed)
	assert.Equal(t, 1, countOccurrences(result.Body, "- x"), result.Body)
	assert.Contains(t, result.Body, "- y")
}

func countOccurrences(body, needle string) int {
	n := 0
	for i := 0; i+len(needle) <= len(body); i++ {
		if body[i:i+len(needle)] == needle {
			n++
		}
	}
	return n
}
