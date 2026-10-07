package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"
)

// Live parity between the openaicompat and literllm backends against Gemini's
// OpenAI-compatible endpoint. Skipped unless AI_RULEZ_LIVE_LLM=1 and
// GEMINI_API_KEY is set; the literllm half also needs a -tags literllm build.
// Spend is a handful of tiny requests.
const (
	liveBaseURL    = "https://generativelanguage.googleapis.com/v1beta/openai"
	liveChatModel  = "gemini-2.5-flash-lite"
	liveEmbedModel = "gemini-embedding-001"
	liveKeyEnv     = "GEMINI_API_KEY"
)

func liveConfig(backend string) Config {
	cfg := Config{
		Backend: backend, Provider: "gemini", Model: liveChatModel, EmbeddingModel: liveEmbedModel,
		APIKeyEnv: liveKeyEnv, AllowNetwork: true, Cache: ptr(false), MaxRetries: -1, TimeoutSeconds: 60,
	}
	if backend == BackendOpenAICompat {
		cfg.BaseURL = liveBaseURL // openaicompat needs the endpoint; literllm routes on gemini/<model>
	}
	return cfg
}

func liveBackends(t *testing.T) []string {
	t.Helper()
	if os.Getenv("AI_RULEZ_LIVE_LLM") != "1" {
		t.Skip("set AI_RULEZ_LIVE_LLM=1 to run live provider tests")
	}
	if os.Getenv(liveKeyEnv) == "" {
		t.Skipf("%s is not set", liveKeyEnv)
	}
	backends := []string{BackendOpenAICompat}
	if NativeAvailable() {
		backends = append(backends, BackendLiterLLM)
	} else {
		t.Log("literllm is not compiled in (-tags literllm); checking openaicompat only")
	}
	return backends
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

func TestLiveErrorClassificationParity(t *testing.T) {
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
	if s := shapes["bad key"]; len(s) == 2 && s[BackendOpenAICompat] != s[BackendLiterLLM] {
		t.Errorf("bad-key classification differs: %+v", s)
	}
	for backend, s := range shapes["bad key"] {
		// Gemini answers a bad key with HTTP 400 INVALID_ARGUMENT, not 401: a permanent provider error on both backends.
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

// The built-in judge must work on every backend; its schema once used
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
		if backend != BackendLiterLLM {
			continue
		}
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

// A connection failure is transient for both backends.
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

// usageTap records the provider's own token totals from every reply that passes it.
type usageTap struct {
	mu     sync.Mutex
	prompt int
	total  int
}

func (u *usageTap) record(body []byte) {
	var w struct {
		Usage struct {
			Prompt int `json:"prompt_tokens"`
			Total  int `json:"total_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(body, &w) == nil {
		u.mu.Lock()
		u.prompt += w.Usage.Prompt
		u.total += w.Usage.Total
		u.mu.Unlock()
	}
}

func (u *usageTap) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := http.DefaultTransport.RoundTrip(r)
	if err != nil {
		return resp, err
	}
	b, rerr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(b))
	if rerr == nil {
		u.record(b)
	}
	return resp, rerr
}

// tappedNative passes native replies through a usageTap.
type tappedNative struct {
	NativeClient
	tap *usageTap
}

func (n *tappedNative) ChatJSON(ctx context.Context, req []byte) ([]byte, error) {
	out, err := n.NativeClient.ChatJSON(ctx, req)
	n.tap.record(out)
	return out, err
}

// A thinking model bills its thinking tokens at the output rate but leaves them out of
// completion_tokens; the budget must charge what the provider's total_tokens says (RV-LLM-1).
// One gemini-2.5-flash call per backend, well under $0.01.
func TestLiveThinkingTokensAreCharged(t *testing.T) {
	for _, backend := range liveBackends(t) {
		t.Run(backend, func(t *testing.T) {
			// Arrange
			tap := &usageTap{}
			opts := Options{NoCache: true, HTTPClient: &http.Client{Transport: tap}}
			if backend == BackendLiterLLM {
				nativeMu.RLock()
				prev := nativeFactory
				nativeMu.RUnlock()
				RegisterNative(func(c NativeConfig) (NativeClient, error) {
					n, err := prev(c)
					if err != nil {
						return nil, err
					}
					return &tappedNative{NativeClient: n, tap: tap}, nil
				})
				t.Cleanup(func() { RegisterNative(prev) })
			}
			cfg := liveConfig(backend)
			cfg.Model = "gemini-2.5-flash"
			cfg.MaxCostUSD = 0.05
			m, err := New(cfg, opts)
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
			want, known := NewPricing(cfg).Cost(cfg.FullModel(), Usage{PromptTokens: tap.prompt, CompletionTokens: tap.total - tap.prompt})
			t.Logf("provider prompt=%d total=%d; charged usage=%+v cost=$%.6f (provider total at list price $%.6f) spent=%+v",
				tap.prompt, tap.total, resp.Usage, resp.CostUSD, want, m.Spent())
			if tap.total == 0 || !known {
				t.Fatalf("no provider total_tokens seen (total=%d) or no price (known=%v)", tap.total, known)
			}
			if backend == BackendLiterLLM {
				// liter-llm's Gemini route hides the thinking tokens even from total_tokens, so the
				// call is charged its completion cap, the upper bound of what was billed.
				if resp.Usage.CompletionTokens != 800 || resp.Usage.PromptTokens != tap.prompt || m.Spent().Tokens != tap.prompt+800 {
					t.Errorf("charged %+v (spent %d), want prompt %d plus the 800-token cap", resp.Usage, m.Spent().Tokens, tap.prompt)
				}
				return
			}
			if resp.Usage.Total() != tap.total || m.Spent().Tokens != tap.total {
				t.Errorf("charged %d tokens (spent %d), want the provider's total_tokens %d", resp.Usage.Total(), m.Spent().Tokens, tap.total)
			}
			if diff := resp.CostUSD - want; diff > 1e-9 || diff < -1e-9 {
				t.Errorf("cost = %.9f, want %.9f", resp.CostUSD, want)
			}
		})
	}
}
