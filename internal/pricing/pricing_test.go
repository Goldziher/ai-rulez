package pricing

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLookup(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		want      Price
		wantKnown bool
	}{
		{"catalog row", "gpt-4o", Price{2.50, 10.00}, true},
		{"catalog prefix fallback keeps a snapshot at the base price", "gpt-4o-mini-2024-07-18", Price{0.15, 0.60}, true},
		{"provider prefix is dropped", "anthropic/claude-sonnet-5", Price{2, 10}, true},
		{"gemini prefix is dropped (catalog misses it)", "gemini/gemini-2.5-flash-lite", Price{0.10, 0.40}, true},
		{"case is ignored", "GPT-4O", Price{2.50, 10.00}, true},
		{"short eval name", "haiku", Price{1, 5}, true},
		{"family floor", "claude-opus-4", Price{15, 75}, true},
		{"gemini embedding", "gemini-embedding-001", Price{0.15, 0}, true},
		{"unknown", "mystery", Price{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, known := Lookup(tt.model)

			assert.Equal(t, tt.wantKnown, known)
			assert.InDelta(t, tt.want.InPerMTok, got.InPerMTok, 1e-9)
			assert.InDelta(t, tt.want.OutPerMTok, got.OutPerMTok, 1e-9)
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
		require.True(t, ok, model)
		assert.InDelta(t, want.InPerMTok, got.InPerMTok, 1e-9, model)
		assert.InDelta(t, want.OutPerMTok, got.OutPerMTok, 1e-9, model)
	}
}

func TestCostAppliesCacheReadRate(t *testing.T) {
	full, known := Cost("gpt-4o", Tokens{Prompt: 1_000_000})
	require.True(t, known)
	cached, known := Cost("gpt-4o", Tokens{Prompt: 1_000_000, Cached: 1_000_000})
	require.True(t, known)

	assert.InDelta(t, 2.50, full, 1e-9)
	assert.Less(t, cached, full, "cache-read tokens are billed at the discounted rate")
}

func TestCostFloorIsFlat(t *testing.T) {
	got, known := Cost("sonnet", Tokens{Prompt: 1_000_000, Completion: 1_000_000})

	assert.True(t, known)
	assert.InDelta(t, 18.0, got, 1e-9)
}

func TestVersionNamesCatalogAndFloors(t *testing.T) {
	assert.True(t, strings.HasPrefix(Version(), "literllm-"))
	assert.Contains(t, Version(), "+floors-")
}

// A paid model listed at 0/0 would let any amount of use through a cost limit; it is unknown instead.
func TestZeroPricedListingsAreUnknown(t *testing.T) {
	for _, name := range []string{"gpt-image-1", "veo-3.1-generate-preview"} {
		_, known := Lookup(name)
		assert.False(t, known, name)
		_, known = Cost(name, Tokens{Prompt: 1_000_000, Completion: 1_000_000})
		assert.False(t, known, name)
	}
}
