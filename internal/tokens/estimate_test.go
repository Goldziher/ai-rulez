package tokens_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
)

func TestEstimate(t *testing.T) {
	assert.Equal(t, 0, tokens.Estimate(""))
	assert.Equal(t, 1, tokens.Estimate("a"))
	assert.Equal(t, 1, tokens.Estimate("abc"))
	assert.Equal(t, 2, tokens.Estimate("abcd"))
	assert.GreaterOrEqual(t, tokens.Estimate(strings.Repeat("世", 100)), 100, "a CJK rune is never undercounted")
}

// TestEstimatorsDiverge documents the two byte-ratio estimators on purpose:
// Estimate gates budgets (3 bytes per token, rounded up) while ByteRatio labels
// reports (3.94 bytes per token, rounded to nearest). Estimate is never lower.
func TestEstimatorsDiverge(t *testing.T) {
	report := tokens.ByteRatio(tokens.EstimateBytesPerToken)
	for _, n := range []int{1, 2, 3, 4, 5, 7, 10, 100, 1000, 19230} {
		text := strings.Repeat("a", n)
		assert.GreaterOrEqual(t, tokens.Estimate(text), report.Count(text), "budget estimate must not undercut the report estimate at %d bytes", n)
	}
	text := strings.Repeat("a", 19230)
	assert.Equal(t, 6410, tokens.Estimate(text))
	assert.Equal(t, 4881, report.Count(text))
	// Rounding differs too: ceil versus half-up, and the report floors a non-empty text at one.
	assert.Equal(t, 1, tokens.Estimate("ab"))
	assert.Equal(t, 1, report.Count("ab"))
	assert.Equal(t, 0, tokens.Estimate(""))
	assert.Equal(t, 0, report.Count(""))
}
