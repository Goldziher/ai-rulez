package llm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
)

// collapsingServer answers every embed with one vector, like Gemini's native route behind
// liter-llm, and fails the request numbered failAt (1-based; 0 never fails). It returns the
// client and the request counter.
func collapsingServer(t *testing.T, failAt int32) (*literLLM, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == failAt {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, `{"error":{"message":"down"}}`)
			return
		}
		fmt.Fprint(w, `{"object":"list","model":"e","data":[{"object":"embedding","index":0,"embedding":[1,0]}],"usage":{"prompt_tokens":5,"total_tokens":5}}`)
	})
	return newLocal(t, localConfig(srv, Config{Provider: "gemini", Model: "m", EmbeddingModel: "e", MaxRetries: -1}), nil), &calls
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
			l, calls := collapsingServer(t, 0)
			b := NewBudget(tc.limits, NewPricing(Config{}))
			c := WithBudget(l, b, "gemini/m", "gemini/e")

			// Act
			_, err := c.Embed(context.Background(), EmbedRequest{Input: embedInputs(tc.inputs)})

			// Assert
			if (err != nil) != tc.wantErr || (err != nil && !errors.Is(err, ErrBudget)) {
				t.Fatalf("err = %v, want budget error %v", err, tc.wantErr)
			}
			if int(calls.Load()) != tc.wantRequests || b.Spent().Calls != tc.wantCalls {
				t.Errorf("provider requests = %d, budget calls = %d; want %d and %d", calls.Load(), b.Spent().Calls, tc.wantRequests, tc.wantCalls)
			}
		})
	}
}

// A failure partway through the singles charges the requests already made at their reported
// usage, plus the failed one's worst case, not just one estimate for the whole batch.
func TestEmbedFallbackShouldChargeTheRequestsMadeBeforeAFailure(t *testing.T) {
	// Arrange: the batch and two singles succeed (5 tokens each), the third single fails.
	l, _ := collapsingServer(t, 4)
	b := NewBudget(Limits{MaxCalls: 100}, NewPricing(Config{}))
	c := WithBudget(l, b, "gemini/m", "gemini/e")
	in := embedInputs(5)
	failedWorst := tokens.Estimate(in[2])

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
