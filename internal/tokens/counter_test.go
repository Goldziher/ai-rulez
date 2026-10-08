package tokens_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		estimate bool
		wantErr  bool
	}{
		{name: "empty defaults to the offline tokenizer", input: "", expected: tokens.CounterCL100KBase},
		{name: "explicit cl100k", input: tokens.CounterCL100KBase, expected: tokens.CounterCL100KBase},
		{name: "byte ratio estimate", input: tokens.CounterEstimate, expected: tokens.CounterEstimate, estimate: true},
		{name: "unknown name", input: "gpt-9", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			counter, err := tokens.New(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.input)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, counter.Name())
			assert.Equal(t, tt.estimate, counter.IsEstimate())
		})
	}
}

func TestCL100KBase_Count(t *testing.T) {
	counter := tokens.CL100KBase()

	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{name: "empty", text: "", expected: 0},
		{name: "single word", text: "tokens", expected: 1},
		{name: "short sentence", text: "Reply with the single word: ok", expected: 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, counter.Count(tt.text))
		})
	}

	assert.False(t, counter.IsEstimate())
}

// TestCL100KBase_HexDigestsAreExpensive pins the reason per-file provenance is
// reported as its own line: a blake3 hex digest is incompressible, so the two hash
// lines ai-rulez injects into every artifact cost far more than their two lines of
// text suggest.
func TestCL100KBase_HexDigestsAreExpensive(t *testing.T) {
	digest := strings.Repeat("ab3f", 16)
	require.Len(t, digest, 64)
	assert.Greater(t, tokens.CL100KBase().Count("# Content-Hash: blake3:"+digest), 15)
}

func TestByteRatio(t *testing.T) {
	counter := tokens.ByteRatio(4)
	assert.True(t, counter.IsEstimate())
	assert.Equal(t, tokens.CounterEstimate, counter.Name())
	assert.Equal(t, 0, counter.Count(""))
	assert.Equal(t, 2, counter.Count("12345678"))

	// A non-positive ratio would divide by zero or return a negative count; the
	// counter falls back to the measured default rather than producing nonsense.
	assert.Positive(t, tokens.ByteRatio(0).Count("12345678"))
}

func TestNames(t *testing.T) {
	assert.Equal(t, []string{tokens.CounterCL100KBase, tokens.CounterEstimate}, tokens.Names())
}

func TestCL100KBoundsAVeryLongLine(t *testing.T) {
	counter := tokens.CL100KBase()
	long := strings.Repeat("​‮", 100000)
	start := time.Now()

	n := counter.Count("# title\n" + long + "\ntail\n")

	assert.Less(t, time.Since(start), 20*time.Second)
	assert.Greater(t, n, 1000)
	assert.Equal(t, 2, counter.Count("hello world"), "below the bound the count is exact")
}
