package improve

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTailIndexes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		n            int
		wantLo, want int
	}{
		{"the design's resample count", BootstrapResamples, 250, 9749},
		{"a thousand resamples", 1000, 25, 974},
		{"a count the percentile does not divide", 7, 0, 6},
		{"a single resample stays in range", 1, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			lo, hi := tailIndexes(tt.n)

			// Assert
			assert.Equal(t, tt.wantLo, lo)
			assert.Equal(t, tt.want, hi)
		})
	}
}
