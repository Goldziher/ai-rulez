package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
)

// An embed batch is a single provider request: it counts once against max_calls.
func TestEmbedShouldCountOneRequestAgainstTheBudget(t *testing.T) {
	// Arrange
	var calls atomic.Int32
	srv := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		fmt.Fprint(w, `{"object":"list","model":"e","data":[{"object":"embedding","index":0,"embedding":[1,0]}],"usage":{"prompt_tokens":5,"total_tokens":5}}`)
	})
	l := newLocal(t, localConfig(srv, Config{Provider: "openai", Model: "m", EmbeddingModel: "e", MaxRetries: -1}), nil)
	b := NewBudget(Limits{MaxCalls: 1}, NewPricing(Config{}))
	c := WithBudget(l, b, "m", "e")

	// Act
	_, err1 := c.Embed(context.Background(), EmbedRequest{Input: []string{"a"}})
	_, err2 := c.Embed(context.Background(), EmbedRequest{Input: []string{"b"}})

	// Assert
	if err1 != nil {
		t.Fatalf("first embed: %v", err1)
	}
	if !errors.Is(err2, ErrBudget) {
		t.Fatalf("second embed = %v, want the max_calls refusal", err2)
	}
	if got := b.Spent().Calls; got != 1 {
		t.Errorf("budget calls = %d, want 1", got)
	}
	if calls.Load() != 1 {
		t.Errorf("provider requests = %d, want 1", calls.Load())
	}
}

// A reply that reports no usage is charged the reserved estimate, so spend stays conservative.
func TestEmbedShouldChargeTheEstimateWhenNoUsageIsReported(t *testing.T) {
	// Arrange
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		parts := make([]string, len(in.Input))
		for i := range in.Input {
			parts[i] = fmt.Sprintf(`{"object":"embedding","index":%d,"embedding":[1,0]}`, i)
		}
		fmt.Fprintf(w, `{"object":"list","model":"e","data":[%s]}`, strings.Join(parts, ","))
	})
	l := newLocal(t, localConfig(srv, Config{Provider: "openai", Model: "m", EmbeddingModel: "e", MaxRetries: -1}), nil)
	b := NewBudget(Limits{MaxCalls: 10}, NewPricing(Config{}))
	c := WithBudget(l, b, "m", "e")
	in := []string{"a", "bb", "ccc"}
	want := tokens.Estimate("a") + tokens.Estimate("bb") + tokens.Estimate("ccc")

	// Act
	_, err := c.Embed(context.Background(), EmbedRequest{Input: in})

	// Assert
	if err != nil {
		t.Fatalf("embed: %v", err)
	}
	if got := b.Spent().Tokens; got != want {
		t.Errorf("spent tokens = %d, want the %d-token estimate when no usage is reported", got, want)
	}
}
