package verifiers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClampReplay(t *testing.T) {
	t.Parallel()
	tests := []struct{ in, want int }{{0, 0}, {-3, -3}, {10, 10}, {MaxReplay, MaxReplay}, {MaxReplay + 1, MaxReplay}, {1 << 30, MaxReplay}}
	for _, tt := range tests {
		assert.Equal(t, tt.want, ClampReplay(tt.in), "%d", tt.in)
	}
}
