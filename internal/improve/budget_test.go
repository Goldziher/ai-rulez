package improve

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBudgetUSD_NeverRoundsAPositiveBudgetToUnlimited(t *testing.T) {
	tests := []struct {
		name string
		left float64
		want float64
	}{
		{"nothing left", 0, 0},
		{"plenty left", 0.36, 0.36},
		{"a remainder that rounds to zero", 0.00004, 0.0001},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act and Assert: max_cost_usd 0 means unlimited to a runner or an optimizer.
			assert.InDelta(t, tt.want, budgetUSD(tt.left), 1e-12)
		})
	}
}
