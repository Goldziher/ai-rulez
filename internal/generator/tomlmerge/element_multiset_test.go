package tomlmerge_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/generator/tomlmerge"
)

func TestUnmerge_HandWrittenDuplicateOfAClaimedElementSurvives(t *testing.T) {
	// Arrange: ai-rulez claimed one "x"; the document holds two.
	doc := "# mine\nlist = [\"x\", \"y\", \"x\"]\n"
	claims := []tomlmerge.Claim{{Path: []string{"list"}, Elements: []any{"x"}}}

	// Act
	result, err := tomlmerge.UnmergeDocument("config.toml", doc, claims)

	// Assert
	require.NoError(t, err)
	assert.True(t, result.Changed)
	assert.Contains(t, result.Body, "# mine")
	assert.Equal(t, 1, strings.Count(result.Body, `"x"`), result.Body)
	assert.Contains(t, result.Body, `"y"`)
}
