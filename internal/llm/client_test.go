package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// The tests in this file run the real liter-llm client against a local
// OpenAI-compatible server: no network and no key. With a base_url liter-llm
// treats every model as generic OpenAI-compatible, so provider-specific
// transforms are liter-llm's own tests' job; the live tests cover them.

// serve starts a local server for the duration of the test.
func serve(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

// localConfig points cfg at srv with the network allowed and no cache.
func localConfig(srv *httptest.Server, cfg Config) Config {
	cfg.BaseURL = srv.URL + "/v1"
	cfg.AllowNetwork = true
	cfg.Cache = ptr(false)
	return cfg
}

func newLocal(t *testing.T, cfg Config, getenv func(string) string) *literLLM {
	t.Helper()
	l, err := newLiterLLM(cfg, getenv)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() }) //nolint:errcheck // test cleanup
	return l
}

func readBody(r *http.Request) map[string]any {
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	return body
}

const chatReply = `{"id":"x","object":"chat.completion","created":1,"model":"gpt-4o-mini","choices":[{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`

func TestLiterLLMChatAndEmbedAgainstALocalServer(t *testing.T) {
	// Arrange
	var gotAuth, gotPath string
	var gotBody map[string]any
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath, gotBody = r.Header.Get("Authorization"), r.URL.Path, readBody(r)
		if strings.HasSuffix(r.URL.Path, "/embeddings") {
			fmt.Fprint(w, `{"object":"list","model":"text-embedding-3-small","data":[{"object":"embedding","index":1,"embedding":[3,4]},{"object":"embedding","index":0,"embedding":[1,2]}],"usage":{"prompt_tokens":4,"total_tokens":4}}`)
			return
		}
		fmt.Fprint(w, `{"id":"x","object":"chat.completion","created":1,"model":"gpt-4o-mini","choices":[{"index":0,"message":{"role":"assistant","content":"{\"score\":0.5}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1000000,"completion_tokens":1000000,"total_tokens":2000000}}`)
	})
	env := map[string]string{"MY_KEY": "sk-live-very-secret-123456"}
	cfg := localConfig(srv, Config{Provider: "openai", Model: "gpt-4o-mini", EmbeddingModel: "text-embedding-3-small", APIKeyEnv: "MY_KEY"})
	m, err := New(cfg, Options{Getenv: func(k string) string { return env[k] }})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	// Act
	req := chatReq("grade this")
	req.ResponseFormat = &JSONSchemaFormat{Name: "verdict", Schema: map[string]any{"type": "object"}}
	req.MaxTokens = 50
	resp, err := m.Chat(context.Background(), req)

	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v1/chat/completions" || gotAuth != "Bearer "+env["MY_KEY"] || gotBody["model"] != "gpt-4o-mini" || gotBody["max_tokens"] != float64(50) {
		t.Fatalf("request: path=%s auth=%s body=%v", gotPath, gotAuth, gotBody)
	}
	rf, _ := gotBody["response_format"].(map[string]any)
	if schema, _ := rf["json_schema"].(map[string]any); rf["type"] != "json_schema" || schema["name"] != "verdict" || schema["strict"] != true {
		t.Fatalf("response_format: %v", rf)
	}
	// 1M prompt + 1M completion tokens at gpt-4o-mini prices = $0.15 + $0.60
	if resp.Text != `{"score":0.5}` || !resp.CostKnown || resp.CostUSD < 0.7499 || resp.CostUSD > 0.7501 || resp.Usage.PromptTokens != 1000000 || resp.FinishReason != "stop" {
		t.Fatalf("response: %+v", resp)
	}

	emb, err := m.Embed(context.Background(), EmbedRequest{Input: []string{"a", "b"}})
	if err != nil || gotPath != "/v1/embeddings" || fmt.Sprint(emb.Vectors) != "[[1 2] [3 4]]" || emb.Usage.PromptTokens != 4 {
		t.Fatalf("embed: %v %v %s", emb, err, gotPath)
	}
}

func TestLiterLLMEmbedCountMismatchIsAProviderError(t *testing.T) {
	// Arrange: the server always answers with two vectors, whatever the batch size.
	srv := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"object":"list","model":"m","data":[{"object":"embedding","index":0,"embedding":[1]},{"object":"embedding","index":1,"embedding":[2]}],"usage":{"prompt_tokens":1,"total_tokens":1}}`)
	})
	l := newLocal(t, localConfig(srv, Config{Provider: "openai", EmbeddingModel: "m"}), nil)

	// Act
	_, err := l.Embed(context.Background(), EmbedRequest{Input: []string{"a", "b", "c"}})

	// Assert
	if !errors.Is(err, ErrProvider) || IsTransient(err) {
		t.Fatalf("a wrong vector count must be a permanent provider error: %v", err)
	}
}

func TestLiterLLMClassifiesProviderFailures(t *testing.T) {
	cases := []struct {
		status  int
		header  string
		body    string
		want    *Error
		wantMsg string
	}{
		{401, "", `{"error":{"message":"Incorrect API key provided: sk-live-very-secret-123456"}}`, ErrAuth, ""},
		{429, "3", `{"error":{"message":"slow down"}}`, ErrRateLimit, ""},
		{400, "", `{"error":{"message":"This model's maximum context length is 8192 tokens","code":"context_length_exceeded"}}`, ErrContextLength, ""},
		{503, "", `{"error":{"message":"upstream down"}}`, ErrProvider, "upstream down"},
		{400, "", `{"error":{"message":"bad field"}}`, ErrProvider, "bad field"},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.status, "/", tc.want.Kind), func(t *testing.T) {
			srv := serve(t, func(w http.ResponseWriter, _ *http.Request) {
				if tc.header != "" {
					w.Header().Set("Retry-After", tc.header)
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			})
			cfg := localConfig(srv, Config{Provider: "openai", Model: "m", APIKeyEnv: "K", MaxRetries: -1})
			m, err := New(cfg, Options{Getenv: func(string) string { return "sk-live-very-secret-123456" }})
			if err != nil {
				t.Fatal(err)
			}
			defer m.Close()

			_, err = m.Chat(context.Background(), chatReq("x"))

			var e *Error
			if !errors.Is(err, tc.want) || !errors.As(err, &e) || e.Status != tc.status {
				t.Fatalf("status %d: got %v", tc.status, err)
			}
			if strings.Contains(err.Error(), "very-secret") {
				t.Fatalf("error leaks the key: %v", err)
			}
			if tc.wantMsg != "" && !strings.Contains(err.Error(), tc.wantMsg) {
				t.Fatalf("message %q missing %q", err, tc.wantMsg)
			}
			if tc.status == 429 && e.RetryAfter != 3*time.Second {
				t.Fatalf("retry-after: %v", e.RetryAfter)
			}
		})
	}
}

// liter-llm retries 429 and 5xx inside one call; the budget counts the call once and a
// permanent rejection is sent exactly once.
func TestLiterLLMRetriesTransientFailuresAndCountsOneBudgetCall(t *testing.T) {
	var hits atomic.Int32
	srv := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) < 3 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		fmt.Fprint(w, chatReply)
	})
	cfg := localConfig(srv, Config{Provider: "openai", Model: "gpt-4o-mini", MaxRetries: 3, MaxCalls: 10})
	m, err := New(cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	resp, err := m.Chat(context.Background(), chatReq("x"))

	if err != nil || resp.Text != "done" || hits.Load() != 3 {
		t.Fatalf("resp=%v err=%v attempts=%d", resp, err, hits.Load())
	}
	if got := m.Spent().Calls; got != 1 {
		t.Errorf("budget calls = %d, want the logical call counted once", got)
	}
}

func TestLiterLLMAttemptCountsPerFailureKind(t *testing.T) {
	cases := []struct {
		name         string
		status       int
		maxRetries   int
		wantAttempts int32
	}{
		{"rate limit is retried", 429, 2, 3},
		{"server error is retried", 500, 2, 3},
		{"bad request is sent once", 400, 2, 1},
		{"auth failure is sent once", 401, 2, 1},
		{"not found is sent once", 404, 2, 1},
		{"retries disabled", 503, -1, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var hits atomic.Int32
			srv := serve(t, func(w http.ResponseWriter, _ *http.Request) {
				hits.Add(1)
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(tc.status)
				fmt.Fprint(w, `{"error":{"message":"no"}}`)
			})
			l := newLocal(t, localConfig(srv, Config{Provider: "openai", Model: "m", MaxRetries: tc.maxRetries}), nil)

			_, err := l.Chat(context.Background(), chatReq("x"))

			if err == nil || hits.Load() != tc.wantAttempts {
				t.Fatalf("err=%v attempts=%d, want %d", err, hits.Load(), tc.wantAttempts)
			}
		})
	}
}

func TestLiterLLMConfigErrors(t *testing.T) {
	_, err := New(allowed(Config{Model: "m", APIKeyEnv: "UNSET_KEY_VAR", BaseURL: "https://x.example"}), Options{Getenv: func(string) string { return "" }})
	if !errors.Is(err, ErrAuth) || !strings.Contains(err.Error(), "UNSET_KEY_VAR") {
		t.Fatalf("unset key var: %v", err)
	}
	l := newLocal(t, Config{BaseURL: "http://127.0.0.1:1/v1"}, nil)
	if _, err := l.Chat(context.Background(), chatReq("x")); !errors.Is(err, ErrConfig) || !strings.Contains(err.Error(), "[llm] model") {
		t.Fatalf("no model: %v", err)
	}
	if _, err := l.Embed(context.Background(), EmbedRequest{Input: []string{"x"}}); !errors.Is(err, ErrConfig) || !strings.Contains(err.Error(), "embedding_model") {
		t.Fatalf("no embedding model: %v", err)
	}
}

// A keyless local endpoint (Ollama style) has no api_key_env: no key is sent.
func TestLiterLLMKeylessEndpointSendsNoCredential(t *testing.T) {
	var auth string
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		fmt.Fprint(w, chatReply)
	})
	l := newLocal(t, localConfig(srv, Config{Model: "llama3"}), nil)

	// TODO(liter-llm#269): liter-llm sends an empty "Authorization: Bearer" header without a key; it carries no credential.
	if _, err := l.Chat(context.Background(), chatReq("x")); err != nil || strings.TrimSpace(strings.TrimPrefix(auth, "Bearer")) != "" {
		t.Fatalf("keyless: err=%v auth=%q", err, auth)
	}
}

// A provider that answers with a list of text parts has them joined.
func TestLiterLLMJoinsTextParts(t *testing.T) {
	srv := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"id":"x","object":"chat.completion","created":1,"model":"gpt-4o-mini","choices":[{"index":0,"message":{"role":"assistant","content":[{"type":"text","text":"he"},{"type":"text","text":"llo"}]},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`)
	})
	l := newLocal(t, localConfig(srv, Config{Provider: "openai", Model: "gpt-4o-mini"}), nil)

	resp, err := l.Chat(context.Background(), chatReq("hi"))

	if err != nil || resp.Text != "hello" || resp.Usage.Total() != 5 {
		t.Fatalf("%+v %v", resp, err)
	}
}

func TestLiterLLMReplyWithoutChoicesIsPermanent(t *testing.T) {
	srv := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"id":"x","object":"chat.completion","created":1,"model":"m","choices":[]}`)
	})
	l := newLocal(t, localConfig(srv, Config{Provider: "openai", Model: "m", MaxRetries: -1}), nil)

	_, err := l.Chat(context.Background(), chatReq("x"))

	if !errors.Is(err, ErrProvider) || IsTransient(err) {
		t.Fatalf("an empty reply must be a permanent provider error: %v", err)
	}
}

func TestLiterLLMHonoursContext(t *testing.T) {
	block := make(chan struct{})
	srv := serve(t, func(http.ResponseWriter, *http.Request) { <-block })
	t.Cleanup(func() { close(block) })
	l := newLocal(t, localConfig(srv, Config{Provider: "openai", Model: "m", MaxRetries: -1}), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := l.Chat(ctx, chatReq("x"))

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want deadline, got %v", err)
	}
}

func TestLiterLLMCloseIsIdempotentAndLaterCallsFail(t *testing.T) {
	srv := serve(t, func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, chatReply) })
	l := newLocal(t, localConfig(srv, Config{Provider: "openai", Model: "m"}), nil)

	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Chat(context.Background(), chatReq("x")); !errors.Is(err, ErrProvider) || IsTransient(err) {
		t.Fatalf("a closed client must return an error, not crash: %v", err)
	}
}

// embedServer answers every embeddings request with one vector, whatever the batch
// size (like Gemini's route behind liter-llm), numbering the vector by the length of the
// first input.
func embedServer(t *testing.T, calls *atomic.Int32) *httptest.Server {
	return serve(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var in struct {
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		fmt.Fprintf(w, `{"object":"list","model":"e","data":[{"object":"embedding","index":0,"embedding":[%d,0]}],"usage":{"prompt_tokens":2,"total_tokens":2}}`, len(in.Input[0]))
	})
}

// Gemini's native route behind liter-llm answers a batch embed with one vector
// whatever the batch size; the backend must still return one vector per input.
func TestLiterLLMEmbedShouldFallBackToPerInputCallsWhenBatchIsCollapsed(t *testing.T) {
	// Arrange
	var calls atomic.Int32
	l := newLocal(t, localConfig(embedServer(t, &calls), Config{Provider: "gemini", Model: "m", EmbeddingModel: "e"}), nil)

	// Act
	resp, err := l.Embed(context.Background(), EmbedRequest{Input: []string{"a", "bb", "ccc"}})

	// Assert
	if err != nil {
		t.Fatalf("embed: %v", err)
	}
	if len(resp.Vectors) != 3 || resp.Vectors[0][0] != 1 || resp.Vectors[1][0] != 2 || resp.Vectors[2][0] != 3 {
		t.Errorf("vectors = %v, want one per input in order", resp.Vectors)
	}
	if resp.Usage.PromptTokens != 8 || resp.Requests != 4 {
		t.Errorf("usage = %+v requests = %d, want the 3 singles and the collapsed batch charged (8 tokens, 4 requests)", resp.Usage, resp.Requests)
	}
	if calls.Load() != 4 {
		t.Errorf("requests = %d, want 1 batch + 3 singles", calls.Load())
	}
}

// Once a batch has come back collapsed, later batches skip the wasted batch request, and the
// budget counts every provider request against max_calls.
func TestLiterLLMEmbedShouldRememberCollapsedBatchesAndCountEveryRequest(t *testing.T) {
	// Arrange
	var calls atomic.Int32
	l := newLocal(t, localConfig(embedServer(t, &calls), Config{Provider: "gemini", Model: "m", EmbeddingModel: "e"}), nil)
	budget := NewBudget(Limits{MaxCalls: 100}, NewPricing(Config{}))
	c := WithBudget(l, budget, "gemini/m", "gemini/e")
	req := EmbedRequest{Input: []string{"a", "b", "c"}}

	// Act
	first, err1 := c.Embed(context.Background(), req)
	callsAfterFirst := calls.Load()
	second, err2 := c.Embed(context.Background(), req)

	// Assert
	if err1 != nil || err2 != nil {
		t.Fatalf("embed: %v %v", err1, err2)
	}
	if callsAfterFirst != 4 || calls.Load() != 7 {
		t.Errorf("requests = %d after the first batch and %d after the second, want 4 and 7 (the second skips the batch request)", callsAfterFirst, calls.Load())
	}
	if len(first.Vectors) != 3 || len(second.Vectors) != 3 {
		t.Errorf("vectors = %d and %d, want 3 each", len(first.Vectors), len(second.Vectors))
	}
	if first.Usage.PromptTokens != 8 || second.Usage.PromptTokens != 6 {
		t.Errorf("tokens = %d and %d, want 8 (the collapsed batch is charged) and 6", first.Usage.PromptTokens, second.Usage.PromptTokens)
	}
	if got := budget.Spent().Calls; got != 7 {
		t.Errorf("budget calls = %d, want every provider request counted (7)", got)
	}
}
