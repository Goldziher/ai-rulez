package evals

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRemaining_NeverRoundsAPositiveBudgetToUnlimited(t *testing.T) {
	tests := []struct {
		name         string
		limit, spent float64
		want         float64
	}{
		{"no limit", 0, 5, 0},
		{"plenty left", 1, 0.25, 0.75},
		{"a remainder that rounds to zero", 1, 0.99996, minBudgetUSD},
		{"a remainder that rounds up", 1, 0.99994, 0.0001},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act and Assert: 0 means unlimited to a runner, so a limited run never hands it out.
			assert.InDelta(t, tt.want, remaining(tt.limit, tt.spent), 1e-12)
		})
	}
}
