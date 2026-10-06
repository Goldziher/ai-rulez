package pricing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLookup(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		want      Price
		wantKnown bool
	}{
		{"exact", "gpt-4o", Price{2.50, 10.00}, true},
		{"longest prefix wins", "gpt-4o-mini-2024", Price{0.15, 0.60}, true},
		{"provider prefix is dropped", "anthropic/claude-sonnet-4", Price{3, 15}, true},
		{"case is ignored", "Claude-Opus-4", Price{15, 75}, true},
		{"short eval name", "haiku", Price{1, 5}, true},
		{"gemini flash-lite beats flash by prefix", "gemini/gemini-2.5-flash-lite", Price{0.10, 0.40}, true},
		{"gemini flash", "gemini-2.5-flash-preview", Price{0.30, 2.50}, true},
		{"gemini embedding", "gemini-embedding-001", Price{0.15, 0}, true},
		{"unknown", "mystery", Price{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got, known := Lookup(tt.model)

			// Assert
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantKnown, known)
		})
	}
}
