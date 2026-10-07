package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

// collapsingNative answers every embed with one vector, like Gemini's native route behind
// liter-llm, and fails the request numbered failAt (1-based; 0 never fails).
type collapsingNative struct {
	calls  int
	failAt int
}

func (c *collapsingNative) ChatJSON(context.Context, []byte) ([]byte, error) { return nil, nil }
func (c *collapsingNative) Free()                                            {}
func (c *collapsingNative) EmbedJSON(context.Context, []byte) ([]byte, error) {
	c.calls++
	if c.calls == c.failAt {
		return nil, &stubNativeErr{variant: "ServiceUnavailable", msg: "down", status: 503}
	}
	return json.Marshal(map[string]any{"data": []any{map[string]any{"index": 0, "embedding": []float32{1, 0}}}, "usage": map[string]any{"prompt_tokens": 5}})
}

func embedInputs(n int) []string {
	in := make([]string, n)
	for i := range in {
		in[i] = fmt.Sprintf("doc %d", i)
	}
	return in
}

// The one-request-per-input fallback made 21 provider requests under max_calls = 2: one
// reservation covered the batch and the singles were counted only afterwards (RV-LLM-8).
func TestEmbedFallbackShouldAdmitEveryRequestAgainstTheBudget(t *testing.T) {
	tests := []struct {
		name         string
		limits       Limits
		inputs       int
		wantErr      bool
		wantRequests int
		wantCalls    int
	}{
		{"max_calls stops the singles", Limits{MaxCalls: 2}, 20, true, 2, 2},
		{"max_tokens stops the singles", Limits{MaxTokens: 50}, 20, true, 3, 3},
		{"a budget that fits admits them all", Limits{MaxCalls: 21}, 20, false, 21, 21},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			n := &collapsingNative{}
			l := &literLLM{native: n, provider: "gemini", model: "gemini/m", embedModel: "gemini/e"}
			b := NewBudget(tc.limits, NewPricing(Config{}))
			c := WithBudget(l, b, "gemini/m", "gemini/e")

			// Act
			_, err := c.Embed(context.Background(), EmbedRequest{Input: embedInputs(tc.inputs)})

			// Assert
			if (err != nil) != tc.wantErr || (err != nil && !errors.Is(err, ErrBudget)) {
				t.Fatalf("err = %v, want budget error %v", err, tc.wantErr)
			}
			if n.calls != tc.wantRequests || b.Spent().Calls != tc.wantCalls {
				t.Errorf("provider requests = %d, budget calls = %d; want %d and %d", n.calls, b.Spent().Calls, tc.wantRequests, tc.wantCalls)
			}
		})
	}
}

// A failure partway through the singles charges the requests already made at their reported
// usage, plus the failed one's worst case, not just one estimate for the whole batch.
func TestEmbedFallbackShouldChargeTheRequestsMadeBeforeAFailure(t *testing.T) {
	// Arrange: the batch and two singles succeed (5 tokens each), the third single fails.
	n := &collapsingNative{failAt: 4}
	l := &literLLM{native: n, provider: "gemini", model: "gemini/m", embedModel: "gemini/e"}
	b := NewBudget(Limits{MaxCalls: 100}, NewPricing(Config{}))
	c := WithBudget(l, b, "gemini/m", "gemini/e")
	in := embedInputs(5)
	failedWorst := EstimateTokens(in[2])

	// Act
	_, err := c.Embed(context.Background(), EmbedRequest{Input: in})

	// Assert
	if err == nil {
		t.Fatal("want the 503")
	}
	if got, want := b.Spent().Tokens, 3*5+failedWorst; got != want || b.Spent().Calls != 4 {
		t.Errorf("spent = %+v, want %d tokens (batch and two singles at 5, the failed single at its estimate) over 4 calls", b.Spent(), want)
	}
}
