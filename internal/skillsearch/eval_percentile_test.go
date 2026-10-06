package skillsearch

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPercentileBounds(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		n            int
		wantLo, want int
	}{
		{"the default resample count", 1000, 25, 974},
		{"two hundred resamples", 200, 5, 194},
		{"forty resamples", 40, 1, 38},
		{"a single resample stays in range", 1, 0, 0},
		{"no resamples", 0, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			lo, hi := percentileBounds(tt.n)

			// Assert
			assert.Equal(t, tt.wantLo, lo)
			assert.Equal(t, tt.want, hi)
		})
	}
}

func TestIntervalOf_UsesTheBounds(t *testing.T) {
	t.Parallel()
	// Arrange: 0..999 sorted, so index i holds i/1000
	means := make([]float64, bootstrapResamples)
	for i := range means {
		means[i] = float64(i) / float64(bootstrapResamples)
	}

	// Act
	low, high := intervalOf(means)

	// Assert
	assert.InDelta(t, 0.025, low, 1e-9)
	assert.InDelta(t, 0.974, high, 1e-9)
}
