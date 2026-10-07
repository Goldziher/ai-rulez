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

func TestClaudeVersionedPrices(t *testing.T) {
	tests := map[string]Price{
		"claude-opus-5-5":           {4, 20},
		"claude-opus-4-8":           {5, 25},
		"claude-opus-4-1-20250805":  {15, 75},
		"claude-sonnet-5-5":         {2, 10},
		"claude-sonnet-4-6":         {3, 15},
		"claude-haiku-5-5":          {0.5, 2.5},
		"claude-haiku-4-5-20251001": {1, 5},
	}
	for model, want := range tests {
		got, ok := Lookup(model)
		if !ok || got != want {
			t.Errorf("Lookup(%q) = %v, %v; want %v", model, got, ok, want)
		}
	}
}
