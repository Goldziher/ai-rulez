package llm

import (
	"context"
	"encoding/json"
	"errors"
	"os"
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
