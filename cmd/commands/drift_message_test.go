package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDriftMessage(t *testing.T) {
	const fix = "run `ai-rulez generate` and commit the result"
	tests := []struct {
		name              string
		differing         int
		blocked           int
		wantContains      []string
		wantNotContaining []string
	}{
		{"plain drift", 3, 0, []string{"3 generated file(s) differ from their sources; " + fix}, []string{"blocked", "--force"}},
		{"only blocked files", 1, 1,
			[]string{"1 generated file(s) differ", "1 blocked", "ai-rulez convert --write", "--force"},
			[]string{"run `ai-rulez generate`"}},
		{"blocked among drift", 3, 1,
			[]string{"3 generated file(s) differ", fix, "1 blocked", "ai-rulez convert --write"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := driftMessage(tt.differing, tt.blocked, fix)

			// Assert
			for _, want := range tt.wantContains {
				assert.Contains(t, got, want)
			}
			for _, not := range tt.wantNotContaining {
				assert.NotContains(t, got, not)
			}
		})
	}
}
