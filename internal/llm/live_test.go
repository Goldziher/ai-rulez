package llm

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"
)

// Live checks of the liter-llm client against Gemini through its native route. Skipped
// unless AI_RULEZ_LIVE_LLM=1 and GEMINI_API_KEY is set. Spend is a handful of tiny requests.
const (
	liveChatModel  = "gemini-2.5-flash-lite"
	liveEmbedModel = "gemini-embedding-001"
	liveKeyEnv     = "GEMINI_API_KEY"
)

// liveBackend labels the one backend the live tests run (subtests are named after it).
const liveBackend = "literllm"

func liveConfig(string) Config {
	return Config{
		Provider: "gemini", Model: liveChatModel, EmbeddingModel: liveEmbedModel,
		APIKeyEnv: liveKeyEnv, AllowNetwork: true, Cache: ptr(false), MaxRetries: -1, TimeoutSeconds: 60,
	}
}

func liveBackends(t *testing.T) []string {
	t.Helper()
	if os.Getenv("AI_RULEZ_LIVE_LLM") != "1" {
		t.Skip("set AI_RULEZ_LIVE_LLM=1 to run live provider tests")
	}
	if os.Getenv(liveKeyEnv) == "" {
		t.Skipf("%s is not set", liveKeyEnv)
	}
	return []string{liveBackend}
}

func liveClient(t *testing.T, backend string, getenv func(string) string) *Managed {
	t.Helper()
	m, err := New(liveConfig(backend), Options{Getenv: getenv})
	if err != nil {
		t.Fatalf("%s: %v", backend, err)
	}
	t.Cleanup(func() { _ = m.Close() })
	return m
}

func TestLiveChatSchemaEmbed(t *testing.T) {
	for _, backend := range liveBackends(t) {
		t.Run(backend, func(t *testing.T) {
			// Arrange
			m := liveClient(t, backend, os.Getenv)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			// Act / Assert: plain chat
			resp, err := m.Chat(ctx, ChatRequest{Messages: []Message{{Role: RoleUser, Content: "Reply with the single word: pong"}}, MaxTokens: 64})
			if err != nil || resp.Text == "" {
				t.Fatalf("chat: %q %v", resp.Text, err)
			}

			// JSON-schema response_format
			schema := map[string]any{
				// No additionalProperties: Gemini's native API (which literllm routes gemini/ to) rejects it.
				"type": "object", "required": []string{"answer"},
				"properties": map[string]any{"answer": map[string]any{"type": "string"}},
			}
			resp, err = m.Chat(ctx, ChatRequest{
				Messages:       []Message{{Role: RoleUser, Content: `Answer with the JSON object {"answer": "ok"}.`}},
				MaxTokens:      64,
				ResponseFormat: &JSONSchemaFormat{Name: "reply", Schema: schema},
			})
			var got struct {
				Answer string `json:"answer"`
			}
			if err != nil || json.Unmarshal([]byte(resp.Text), &got) != nil || got.Answer == "" {
				t.Fatalf("schema chat: %q %v", resp.Text, err)
			}

			// Embeddings
			emb, err := m.Embed(ctx, EmbedRequest{Input: []string{"hello"}})
			if err != nil || len(emb.Vectors) != 1 || len(emb.Vectors[0]) == 0 {
				t.Fatalf("embed: %v (%d vectors)", err, len(emb.Vectors))
			}
		})
	}
}

// errorShape is what must agree between the backends for the same failure.
type errorShape struct {
	kind      Kind
	transient bool
	deadline  bool
}

func shapeOf(err error) errorShape {
	var e *Error
	s := errorShape{deadline: errors.Is(err, context.DeadlineExceeded), transient: IsTransient(err)}
	if errors.As(err, &e) {
		s.kind = e.Kind
	}
	return s
}

func TestLiveErrorClassification(t *testing.T) {
	backends := liveBackends(t)
	chat := func(m *Managed, timeout time.Duration) error {
		_, err := m.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}, MaxTokens: 8, Timeout: timeout})
		return err
	}
	shapes := map[string]map[string]errorShape{"bad key": {}, "timeout": {}}
	for _, backend := range backends {
		bad := liveClient(t, backend, func(string) string { return "AIza-invalid-key-for-parity-test" })
		shapes["bad key"][backend] = shapeOf(chat(bad, 0))
		ok := liveClient(t, backend, os.Getenv)
		shapes["timeout"][backend] = shapeOf(chat(ok, time.Millisecond))
	}
	for name, byBackend := range shapes {
		for backend, s := range byBackend {
			t.Logf("%s / %s: kind=%q transient=%v deadline=%v", name, backend, s.kind, s.transient, s.deadline)
		}
	}
	for backend, s := range shapes["bad key"] {
		// Gemini answers a bad key with HTTP 400 INVALID_ARGUMENT, not 401: a permanent provider error.
		if s.transient || s.deadline {
			t.Errorf("%s: a bad key must be a permanent error, got %+v", backend, s)
		}
	}
	for backend, s := range shapes["timeout"] {
		if !s.deadline && s.kind != KindTimeout {
			t.Errorf("%s: a 1ms timeout must surface as a deadline or timeout, got %+v", backend, s)
		}
	}
}

// The built-in judge must work against Gemini; its schema once used
// additionalProperties, which Gemini's native API rejects.
func TestLiveJudge(t *testing.T) {
	for _, backend := range liveBackends(t) {
		t.Run(backend, func(t *testing.T) {
			// Arrange
			c := liveClient(t, backend, os.Getenv)
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()

			// Act
			v, err := Judge(ctx, c, "The assistant must greet the user.", "user: hi\nassistant: Hello! How can I help?")

			// Assert
			if err != nil {
				t.Fatalf("judge: %v", err)
			}
			if v.Score < 0.5 {
				t.Errorf("a plain greeting scored %.2f (%s)", v.Score, v.Rationale)
			}
		})
	}
}

// A per-request model naming another provider must not send the Gemini key there; a bare
// override stays on the configured provider.
func TestLiveRequestModelCannotRerouteKey(t *testing.T) {
	for _, backend := range liveBackends(t) {
		t.Run(backend, func(t *testing.T) {
			// Arrange
			m := liveClient(t, backend, os.Getenv)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			msgs := []Message{{Role: RoleUser, Content: "Reply with the single word: pong"}}

			// Act
			_, rerouted := m.Chat(ctx, ChatRequest{Model: "openai/gpt-4o-mini", Messages: msgs, MaxTokens: 16})
			bare, err := m.Chat(ctx, ChatRequest{Model: liveChatModel, Messages: msgs, MaxTokens: 64})

			// Assert
			var e *Error
			if !errors.As(rerouted, &e) || e.Kind != KindConfig {
				t.Fatalf("a request model on another provider must be refused locally, got %v", rerouted)
			}
			if err != nil || bare.Text == "" {
				t.Fatalf("a bare request model must stay on the configured provider: %q %v", bare.Text, err)
			}
		})
	}
}

// A connection failure is transient.
func TestLiveConnectionFailureIsTransient(t *testing.T) {
	for _, backend := range liveBackends(t) {
		t.Run(backend, func(t *testing.T) {
			// Arrange
			cfg := liveConfig(backend)
			cfg.BaseURL = "http://127.0.0.1:1/v1"
			m, err := New(cfg, Options{Getenv: os.Getenv})
			if err != nil {
				t.Fatalf("%s: %v", backend, err)
			}
			t.Cleanup(func() { _ = m.Close() })

			// Act
			_, err = m.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}, MaxTokens: 8})

			// Assert
			if err == nil || !IsTransient(err) {
				t.Fatalf("a refused connection must be transient: %v", err)
			}
		})
	}
}

// A thinking model bills its thinking tokens at the output rate. liter-llm 2.2.0 and later put them in
// completion_tokens and total_tokens, so the budget settles on the real usage (RV-LLM-1). One
// gemini-2.5-flash call, well under $0.01. There is no hook on liter-llm's HTTP client to read the
// provider's own total_tokens, so this checks that thinking tokens show up at all: a visible answer of a
// few characters cannot account for the charged completion.
func TestLiveThinkingTokensAreCharged(t *testing.T) {
	for _, backend := range liveBackends(t) {
		t.Run(backend, func(t *testing.T) {
			// Arrange
			cfg := liveConfig(backend)
			cfg.Model = "gemini-2.5-flash"
			cfg.MaxCostUSD = 0.05
			m, err := New(cfg, Options{NoCache: true})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = m.Close() })

			// Act
			resp, err := m.Chat(context.Background(), ChatRequest{MaxTokens: 800, Messages: []Message{{Role: RoleUser,
				Content: "A bat and ball cost 1.10 total; the bat costs 1.00 more than the ball. Ball price? Answer with just the number."}}})

			// Assert
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("charged usage=%+v cost=$%.6f spent=%+v", resp.Usage, resp.CostUSD, m.Spent())
			if resp.Usage.CompletionTokens <= len(resp.Text) || !resp.CostKnown || resp.CostUSD <= 0 {
				t.Errorf("thinking tokens not charged: %+v", resp)
			}
			if m.Spent().Tokens != resp.Usage.Total() {
				t.Errorf("spent %d tokens, want the charged %d", m.Spent().Tokens, resp.Usage.Total())
			}
		})
	}
}
