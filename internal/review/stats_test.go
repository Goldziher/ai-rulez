package review

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestQuadraticKappa(t *testing.T) {
	tests := []struct {
		name string
		a, b []int
		want float64
	}{
		{"perfect agreement", []int{0, 1, 2, 0}, []int{0, 1, 2, 0}, 1},
		{"one adjacent disagreement (worked by hand: 1 - 0.25/1.25)", []int{0, 1, 2, 2}, []int{0, 1, 2, 1}, 0.8},
		{"everything one category and equal is perfect", []int{0, 0, 0}, []int{0, 0, 0}, 1},
		{"one rater constant and unequal is chance", []int{0, 0, 0, 0}, []int{1, 1, 1, 1}, 0},
		{"opposite ends are worse than chance", []int{0, 2, 0, 2}, []int{2, 0, 2, 0}, -1},
		{"empty", nil, nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.InDelta(t, tt.want, quadraticKappa(tt.a, tt.b, 3), 0.001)
		})
	}
}

func TestWilson(t *testing.T) {
	tests := []struct {
		s, n         int
		wantLo, want float64
	}{
		{10, 10, 0.722, 1},
		{5, 10, 0.237, 0.763},
		{0, 10, 0, 0.278},
		{0, 0, 0, 1},
	}
	for _, tt := range tests {
		lo, hi := wilson(tt.s, tt.n)
		assert.InDelta(t, tt.wantLo, lo, 0.002, "%d/%d lower", tt.s, tt.n)
		assert.InDelta(t, tt.want, hi, 0.002, "%d/%d upper", tt.s, tt.n)
	}
}

func TestFleissKappa(t *testing.T) {
	tests := []struct {
		name   string
		counts [][]int
		want   float64
	}{
		{"every subject unanimous across categories", [][]int{{3, 0, 0}, {0, 3, 0}}, 1},
		{"all raters in one category for every subject", [][]int{{3, 0, 0}, {3, 0, 0}}, 1},
		{"even split everywhere is worse than chance", [][]int{{1, 1, 1}, {1, 1, 1}}, -0.5},
		{"no subjects", nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.InDelta(t, tt.want, fleissKappa(tt.counts), 0.001)
		})
	}
}
