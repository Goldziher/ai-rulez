package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func chatReq(text string) ChatRequest {
	return ChatRequest{Messages: []Message{{Role: RoleUser, Content: text}}, PromptVersion: "t/v1"}
}

func TestFakeIsDeterministic(t *testing.T) {
	f := NewFake()
	a, err := f.Chat(context.Background(), chatReq("hello"))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := f.Chat(context.Background(), chatReq("hello"))
	c, _ := f.Chat(context.Background(), chatReq("other"))
	if a.Text != b.Text || a.Text == c.Text || !strings.HasPrefix(a.Text, "fake:") {
		t.Fatalf("not deterministic: %q %q %q", a.Text, b.Text, c.Text)
	}
	e1, _ := f.Embed(context.Background(), EmbedRequest{Input: []string{"x", "y"}})
	e2, _ := f.Embed(context.Background(), EmbedRequest{Input: []string{"x", "y"}})
	if len(e1.Vectors) != 2 || len(e1.Vectors[0]) != 8 || fmt.Sprint(e1.Vectors) != fmt.Sprint(e2.Vectors) {
		t.Fatalf("embeddings differ: %v %v", e1.Vectors, e2.Vectors)
	}
	if len(f.ChatCalls()) != 3 || len(f.EmbedCalls()) != 2 {
		t.Fatal("calls not recorded")
	}
}

// allowed enables the network and pins the pure-Go backend, so the tests mean the
// same thing in a -tags literllm build where auto would pick the native one.
func allowed(cfg Config) Config {
	cfg.AllowNetwork = true
	if cfg.Backend == "" {
		cfg.Backend = BackendOpenAICompat
	}
	return cfg
}

func TestNetworkGateRefusesByDefault(t *testing.T) {
	m, err := New(Config{Model: "gpt-4o-mini"}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.Chat(context.Background(), chatReq("x"))
	if !errors.Is(err, ErrNetworkDisabled) || !strings.Contains(err.Error(), "allow_network") {
		t.Fatalf("want network-disabled error naming allow_network, got %v", err)
	}
	if _, err = m.Embed(context.Background(), EmbedRequest{Input: []string{"x"}}); !errors.Is(err, ErrNetworkDisabled) {
		t.Fatalf("embed: %v", err)
	}
}

func TestBudgetFailsClosed(t *testing.T) {
	ctx := context.Background()
	t.Run("max_calls", func(t *testing.T) {
		f := NewFake()
		m := Wrap(f, allowed(Config{Model: "gpt-4o-mini", MaxCalls: 2, Cache: ptr(false)}), Options{Retry: &RetryPolicy{}})
		for i := range 2 {
			if _, err := m.Chat(ctx, chatReq(fmt.Sprint(i))); err != nil {
				t.Fatal(err)
			}
		}
		_, err := m.Chat(ctx, chatReq("3"))
		if !errors.Is(err, ErrBudget) || len(f.ChatCalls()) != 2 {
			t.Fatalf("third call must be refused before reaching the backend: %v, calls=%d", err, len(f.ChatCalls()))
		}
	})
	t.Run("max_tokens", func(t *testing.T) {
		f := NewFake()
		m := Wrap(f, allowed(Config{Model: "gpt-4o-mini", MaxTokens: 50}), Options{})
		req := chatReq(strings.Repeat("word ", 200))
		if _, err := m.Chat(ctx, req); !errors.Is(err, ErrBudget) || len(f.ChatCalls()) != 0 {
			t.Fatalf("over-token request must be refused: %v", err)
		}
	})
	t.Run("max_cost_usd", func(t *testing.T) {
		f := NewFake()
		cfg := allowed(Config{Model: "gpt-4o", MaxCostUSD: 0.0001, Cache: ptr(false)})
		m := Wrap(f, cfg, Options{})
		req := chatReq("hi")
		req.MaxTokens = 1000 // worst case 1000*10/1e6 = $0.01 > $0.0001
		if _, err := m.Chat(ctx, req); !errors.Is(err, ErrBudget) || len(f.ChatCalls()) != 0 {
			t.Fatalf("cost cap: %v", err)
		}
	})
	t.Run("unknown price with cost cap", func(t *testing.T) {
		f := NewFake()
		m := Wrap(f, allowed(Config{Model: "my-local-model", MaxCostUSD: 1}), Options{})
		_, err := m.Chat(ctx, chatReq("hi"))
		if !errors.Is(err, ErrBudget) || !strings.Contains(err.Error(), "no price is known") {
			t.Fatalf("unknown price must fail closed: %v", err)
		}
	})
	t.Run("missing usage charges the worst case", func(t *testing.T) {
		f := &zeroUsage{}
		m := Wrap(f, allowed(Config{Model: "gpt-4o-mini", MaxTokens: 1500, Cache: ptr(false)}), Options{})
		if _, err := m.Chat(ctx, chatReq("a")); err != nil {
			t.Fatal(err)
		}
		if got := m.Spent().Tokens; got < DefaultCompletionCap {
			t.Fatalf("expected worst-case charge, got %d", got)
		}
		if _, err := m.Chat(ctx, chatReq("b")); !errors.Is(err, ErrBudget) {
			t.Fatalf("second call must exceed the token budget: %v", err)
		}
	})
	t.Run("default completion cap applied", func(t *testing.T) {
		f := NewFake()
		m := Wrap(f, allowed(Config{Model: "gpt-4o-mini", MaxCalls: 5, Cache: ptr(false)}), Options{})
		if _, err := m.Chat(ctx, chatReq("a")); err != nil {
			t.Fatal(err)
		}
		if got := f.ChatCalls()[0].MaxTokens; got != DefaultCompletionCap {
			t.Fatalf("budgeted call must be capped, got max_tokens=%d", got)
		}
	})
}

type zeroUsage struct{ Fake }

func (z *zeroUsage) Chat(ctx context.Context, r ChatRequest) (ChatResponse, error) {
	resp, err := z.Fake.Chat(ctx, r)
	resp.Usage = Usage{}
	return resp, err
}

func ptr[T any](v T) *T { return &v }

func TestCacheHitMissAndVersionBump(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	f := NewFake()
	cfg := allowed(Config{Model: "gpt-4o-mini", MaxCalls: 10})
	m := Wrap(f, cfg, Options{ConfigDir: dir, Retry: &RetryPolicy{}})

	first, err := m.Chat(ctx, chatReq("same"))
	if err != nil || first.Cached {
		t.Fatalf("first: %v cached=%v", err, first.Cached)
	}
	second, _ := m.Chat(ctx, chatReq("same"))
	if !second.Cached || second.Text != first.Text || second.CostUSD != 0 || len(f.ChatCalls()) != 1 {
		t.Fatalf("expected cache hit without a backend call: %+v calls=%d", second, len(f.ChatCalls()))
	}
	if m.Spent().Calls != 1 {
		t.Fatalf("a hit must not consume budget, calls=%d", m.Spent().Calls)
	}
	bumped := chatReq("same")
	bumped.PromptVersion = "t/v2"
	if r, _ := m.Chat(ctx, bumped); r.Cached || len(f.ChatCalls()) != 2 {
		t.Fatal("prompt version bump must miss")
	}
	other := chatReq("same")
	other.Model = "gpt-4o"
	if r, _ := m.Chat(ctx, other); r.Cached {
		t.Fatal("model change must miss")
	}
	nc := chatReq("same")
	nc.NoCache = true
	if r, _ := m.Chat(ctx, nc); r.Cached || len(f.ChatCalls()) != 4 {
		t.Fatal("NoCache must bypass")
	}
	// a different endpoint identity never shares entries
	m2 := Wrap(NewFake(), allowed(Config{Model: "gpt-4o-mini", BaseURL: "https://other.example/v1"}), Options{ConfigDir: dir})
	if r, _ := m2.Chat(ctx, chatReq("same")); r.Cached {
		t.Fatal("different base_url must not share the cache")
	}
	// corrupt entry is a miss, cache dir location and permissions
	cacheDir := CacheDirFor(Options{ConfigDir: dir})
	entries, _ := filepath.Glob(filepath.Join(cacheDir, "*", "*.json"))
	if len(entries) == 0 {
		t.Fatal("no cache files written under the user cache directory")
	}
	if err := os.WriteFile(entries[0], []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := m.Cache().Clear(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cacheDir); !os.IsNotExist(err) {
		t.Fatal("Clear must remove the directory")
	}
}

func TestCacheOptOut(t *testing.T) {
	dir := t.TempDir()
	for name, m := range map[string]*Managed{
		"config":  Wrap(NewFake(), allowed(Config{Model: "m", Cache: ptr(false)}), Options{ConfigDir: dir}),
		"flag":    Wrap(NewFake(), allowed(Config{Model: "m"}), Options{ConfigDir: dir, NoCache: true}),
		"no-root": Wrap(NewFake(), allowed(Config{Model: "m"}), Options{}),
	} {
		m.Chat(context.Background(), chatReq("x"))
		r, _ := m.Chat(context.Background(), chatReq("x"))
		if r.Cached || m.Cache() != nil {
			t.Fatalf("%s: cache must be off", name)
		}
	}
	if _, err := os.Stat(CacheDirFor(Options{ConfigDir: dir})); err == nil {
		t.Fatal("nothing should be written when the cache is off")
	}
}

func TestEmbedCache(t *testing.T) {
	f := NewFake()
	m := Wrap(f, allowed(Config{EmbeddingModel: "text-embedding-3-small"}), Options{ConfigDir: t.TempDir()})
	a, _ := m.Embed(context.Background(), EmbedRequest{Input: []string{"a"}})
	b, _ := m.Embed(context.Background(), EmbedRequest{Input: []string{"a"}})
	if !b.Cached || len(f.EmbedCalls()) != 1 || fmt.Sprint(a.Vectors) != fmt.Sprint(b.Vectors) {
		t.Fatal("embedding cache miss")
	}
}

func TestRedaction(t *testing.T) {
	secret := "PROMPT-CONTENT-DO-NOT-LOG"
	sum := chatReq(secret).Summary()
	if strings.Contains(sum, secret) || !strings.Contains(sum, "messages=1") {
		t.Fatalf("summary leaks content: %s", sum)
	}
	for _, in := range []string{"Authorization: Bearer abcdefghijklmnop", "key sk-abcdefghijklmnop here", "api_key=supersecretvalue1", "AKIAABCDEFGHIJKLMNOP"} {
		if out := RedactSecrets(in); strings.Contains(out, "abcdefghij") || strings.Contains(out, "supersecret") || strings.Contains(out, "AKIAABCD") {
			t.Fatalf("not redacted: %q -> %q", in, out)
		}
	}
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	c := WithLogging(&failing{err: &Error{Kind: KindProvider, Message: "echo sk-abcdefghijklmnop"}}, log)
	c.Chat(context.Background(), chatReq(secret))
	c2 := WithLogging(NewFake(), log)
	c2.Chat(context.Background(), chatReq(secret))
	c2.Embed(context.Background(), EmbedRequest{Input: []string{secret}})
	if out := buf.String(); strings.Contains(out, secret) || strings.Contains(out, "sk-abcdefghij") || !strings.Contains(out, "llm chat") {
		t.Fatalf("log leaks or is empty: %s", out)
	}
}

type failing struct {
	Fake
	err   error
	calls atomic.Int32
}

func (f *failing) Chat(context.Context, ChatRequest) (ChatResponse, error) {
	f.calls.Add(1)
	return ChatResponse{}, f.err
}

func TestRetry(t *testing.T) {
	ctx := context.Background()
	var slept []time.Duration
	policy := RetryPolicy{Retries: 3, Rand: func() float64 { return 1 }, Sleep: func(_ context.Context, d time.Duration) error { slept = append(slept, d); return nil }}

	flaky := &flaky{failUntil: 2}
	resp, err := WithRetry(flaky, policy).Chat(ctx, chatReq("x"))
	if err != nil || resp.Text != "ok" || flaky.n != 3 || len(slept) != 2 {
		t.Fatalf("resp=%+v err=%v calls=%d sleeps=%v", resp, err, flaky.n, slept)
	}
	if slept[1] <= slept[0] || slept[0] > 500*time.Millisecond {
		t.Fatalf("backoff should grow and stay within the jitter ceiling: %v", slept)
	}

	for _, e := range []*Error{{Kind: KindAuth}, {Kind: KindContextLength}, {Kind: KindBudget}, {Kind: KindProvider, Status: 400}} {
		f := &failing{err: e}
		if _, err := WithRetry(f, policy).Chat(ctx, chatReq("x")); err == nil || f.calls.Load() != 1 {
			t.Fatalf("%s must not be retried, calls=%d", e.Kind, f.calls.Load())
		}
	}

	exhausted := &failing{err: &Error{Kind: KindRateLimit, Status: 429, RetryAfter: 2 * time.Second}}
	slept = nil
	if _, err := WithRetry(exhausted, policy).Chat(ctx, chatReq("x")); !errors.Is(err, ErrRateLimit) || exhausted.calls.Load() != 4 {
		t.Fatalf("want 4 attempts then rate limit error, got %v after %d", err, exhausted.calls.Load())
	}
	if slept[0] != 2*time.Second {
		t.Fatalf("Retry-After must be honored, slept %v", slept[0])
	}

	cctx, cancel := context.WithCancel(ctx)
	cancel()
	f := &failing{err: &Error{Kind: KindRateLimit}}
	WithRetry(f, policy).Chat(cctx, chatReq("x"))
	if f.calls.Load() != 1 {
		t.Fatal("canceled context must stop retries")
	}
}

type flaky struct {
	Fake
	failUntil, n int
}

func (f *flaky) Chat(context.Context, ChatRequest) (ChatResponse, error) {
	f.n++
	if f.n <= f.failUntil {
		return ChatResponse{}, &Error{Kind: KindProvider, Status: 503, Message: "unavailable"}
	}
	return ChatResponse{Text: "ok"}, nil
}

func TestRetriesCountAgainstBudget(t *testing.T) {
	f := &flaky{failUntil: 10}
	m := Wrap(f, allowed(Config{Model: "gpt-4o-mini", MaxCalls: 2, Cache: ptr(false)}), Options{Retry: &RetryPolicy{Retries: 5, Sleep: func(context.Context, time.Duration) error { return nil }}})
	_, err := m.Chat(context.Background(), chatReq("x"))
	if !errors.Is(err, ErrBudget) || f.n != 2 {
		t.Fatalf("retries must be bounded by max_calls: err=%v calls=%d", err, f.n)
	}
}

func TestTimeout(t *testing.T) {
	slow := &slowClient{}
	c := withGate(slow, true, 0)
	req := chatReq("x")
	req.Timeout = 20 * time.Millisecond
	_, err := c.Chat(context.Background(), req)
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("want timeout, got %v", err)
	}
}

type slowClient struct{ Fake }

func (s *slowClient) Chat(ctx context.Context, _ ChatRequest) (ChatResponse, error) {
	<-ctx.Done()
	return ChatResponse{}, ctx.Err()
}

func TestDryRunSendsNothing(t *testing.T) {
	var out bytes.Buffer
	m, err := New(Config{Model: "gpt-4o-mini", APIKeyEnv: "SOME_KEY"}, Options{DryRun: &out})
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.Chat(context.Background(), chatReq("super secret prompt"))
	if !errors.Is(err, ErrDryRun) || !strings.Contains(out.String(), "dry-run chat") || strings.Contains(out.String(), "super secret prompt") {
		t.Fatalf("err=%v out=%q", err, out.String())
	}
	out.Reset()
	m, _ = New(Config{Model: "gpt-4o-mini"}, Options{DryRun: &out, ShowContent: true})
	m.Chat(context.Background(), chatReq("show me"))
	if !strings.Contains(out.String(), "show me") || !strings.Contains(out.String(), "estimated cost") {
		t.Fatalf("ShowContent output: %q", out.String())
	}
}

func TestOpenAICompat(t *testing.T) {
	var gotAuth, gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath = r.Header.Get("Authorization"), r.URL.Path
		gotBody = nil
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		switch {
		case strings.HasSuffix(r.URL.Path, "/embeddings"):
			fmt.Fprint(w, `{"model":"text-embedding-3-small","data":[{"index":1,"embedding":[3,4]},{"index":0,"embedding":[1,2]}],"usage":{"prompt_tokens":4,"total_tokens":4}}`)
		default:
			fmt.Fprint(w, `{"model":"gpt-4o-mini","choices":[{"message":{"role":"assistant","content":"{\"score\":0.5}"}}],"usage":{"prompt_tokens":1000000,"completion_tokens":1000000}}`)
		}
	}))
	defer srv.Close()
	env := map[string]string{"MY_KEY": "sk-live-very-secret-123456"}
	opts := Options{Getenv: func(k string) string { return env[k] }, HTTPClient: srv.Client()}
	cfg := allowed(Config{Model: "gpt-4o-mini", EmbeddingModel: "text-embedding-3-small", BaseURL: srv.URL + "/v1/", APIKeyEnv: "MY_KEY", Cache: ptr(false)})
	m, err := New(cfg, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	req := chatReq("grade this")
	req.ResponseFormat = &JSONSchemaFormat{Name: "verdict", Schema: map[string]any{"type": "object"}}
	req.MaxTokens = 50
	resp, err := m.Chat(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v1/chat/completions" || gotAuth != "Bearer "+env["MY_KEY"] || gotBody["model"] != "gpt-4o-mini" || gotBody["max_tokens"] != float64(50) {
		t.Fatalf("request: path=%s auth=%s body=%v", gotPath, gotAuth, gotBody)
	}
	rf := gotBody["response_format"].(map[string]any)
	if rf["type"] != "json_schema" || rf["json_schema"].(map[string]any)["name"] != "verdict" {
		t.Fatalf("response_format: %v", rf)
	}
	// 1M prompt + 1M completion tokens at gpt-4o-mini prices = $0.15 + $0.60
	if resp.Text != `{"score":0.5}` || !resp.CostKnown || resp.CostUSD < 0.7499 || resp.CostUSD > 0.7501 || resp.Usage.PromptTokens != 1000000 {
		t.Fatalf("response: %+v", resp)
	}

	emb, err := m.Embed(context.Background(), EmbedRequest{Input: []string{"a", "b"}})
	if err != nil || gotPath != "/v1/embeddings" || fmt.Sprint(emb.Vectors) != "[[1 2] [3 4]]" {
		t.Fatalf("embed: %v %v %s", emb, err, gotPath)
	}
	if _, err := m.Embed(context.Background(), EmbedRequest{Input: []string{"a", "b", "c"}}); !errors.Is(err, ErrProvider) {
		t.Fatalf("count mismatch must be a provider error: %v", err)
	}
}

func TestOpenAICompatErrors(t *testing.T) {
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
		{503, "", `upstream down`, ErrProvider, "upstream down"},
		{400, "", `{"error":{"message":"bad field"}}`, ErrProvider, "bad field"},
	}
	for _, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if tc.header != "" {
				w.Header().Set("Retry-After", tc.header)
			}
			w.WriteHeader(tc.status)
			fmt.Fprint(w, tc.body)
		}))
		cfg := allowed(Config{Model: "m", BaseURL: srv.URL, APIKeyEnv: "K", Cache: ptr(false), MaxRetries: -1})
		m, err := New(cfg, Options{Getenv: func(string) string { return "sk-live-very-secret-123456" }, HTTPClient: srv.Client()})
		if err != nil {
			t.Fatal(err)
		}
		_, err = m.Chat(context.Background(), chatReq("x"))
		srv.Close()
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
	}
}

func TestOpenAICompatRetriesTransient(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if n.Add(1) < 3 {
			w.WriteHeader(502)
			return
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"done"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	}))
	defer srv.Close()
	m, _ := New(allowed(Config{Model: "m", BaseURL: srv.URL, Cache: ptr(false)}), Options{HTTPClient: srv.Client(), Retry: &RetryPolicy{Retries: 3, Sleep: func(context.Context, time.Duration) error { return nil }}})
	resp, err := m.Chat(context.Background(), chatReq("x"))
	if err != nil || resp.Text != "done" || n.Load() != 3 {
		t.Fatalf("resp=%v err=%v attempts=%d", resp, err, n.Load())
	}
}

func TestOpenAICompatConfigErrors(t *testing.T) {
	if _, err := New(allowed(Config{Model: "m", APIKeyEnv: "UNSET_KEY_VAR", BaseURL: "https://x.example"}), Options{Getenv: func(string) string { return "" }}); !errors.Is(err, ErrAuth) || !strings.Contains(err.Error(), "UNSET_KEY_VAR") {
		t.Fatalf("unset key var: %v", err)
	}
	if _, err := New(allowed(Config{Model: "m", Provider: "bedrock"}), Options{}); !errors.Is(err, ErrConfig) || !strings.Contains(err.Error(), "base_url") {
		t.Fatalf("non-openai provider without base_url: %v", err)
	}
	if _, err := New(allowed(Config{Backend: BackendLiterLLM, Model: "m"}), Options{}); !NativeAvailable() && (!errors.Is(err, ErrConfig) || !strings.Contains(err.Error(), "-tags literllm")) {
		t.Fatalf("literllm not compiled in: %v", err)
	}
	// keyless local endpoint (Ollama style): no api_key_env and a base_url means no auth header
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		fmt.Fprint(w, `{"choices":[{"message":{"content":"hi"}}]}`)
	}))
	defer srv.Close()
	m, err := New(allowed(Config{Model: "llama3", BaseURL: srv.URL}), Options{HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Chat(context.Background(), chatReq("x")); err != nil || auth != "" {
		t.Fatalf("keyless: err=%v auth=%q", err, auth)
	}
}

func TestConfigValidation(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want string // substring of a problem; empty means valid
	}{
		{"empty is valid", Config{}, ""},
		{"full valid", Config{Backend: "openaicompat", BaseURL: "https://gw.internal/v1", APIKeyEnv: "OPENAI_API_KEY", MaxCostUSD: 1}, ""},
		{"unknown backend", Config{Backend: "litellm"}, "backend"},
		{"literal openai key", Config{APIKeyEnv: "sk-proj-abc123def456ghi789"}, "literal API key"},
		{"literal aws key", Config{APIKeyEnv: "AKIAIOSFODNN7EXAMPLE1"}, "literal API key"},
		{"mixed-case blob", Config{APIKeyEnv: "aB3dE5fG7hI9jK1lM3nO5pQ7rS9"}, "literal API key"},
		{"not an env name", Config{APIKeyEnv: "my key"}, "environment variable name"},
		{"creds in url", Config{BaseURL: "https://user:pw@gw.example/v1"}, "credentials"},
		{"token in query", Config{BaseURL: "https://gw.example/v1?key=abc"}, "query string"},
		{"not a url", Config{BaseURL: "gw.example"}, "http(s) URL"},
		{"negative budget", Config{MaxCostUSD: -1}, "max_cost_usd"},
		{"negative tokens", Config{MaxTokens: -5}, "max_tokens"},
	}
	for _, tc := range cases {
		problems := tc.cfg.Validate()
		switch {
		case tc.want == "" && len(problems) > 0:
			t.Errorf("%s: unexpected %v", tc.name, problems)
		case tc.want != "" && !strings.Contains(strings.Join(problems, "|"), tc.want):
			t.Errorf("%s: want %q in %v", tc.name, tc.want, problems)
		}
	}
	err := Config{Backend: "x"}.Err()
	if err == nil || !strings.Contains(err.Error(), "AR9L0") || !errors.Is(err, ErrConfig) {
		t.Fatalf("Err must carry AR9L0: %v", err)
	}
	if _, err := New(Config{Backend: "x", AllowNetwork: true}, Options{}); err == nil {
		t.Fatal("New must reject an invalid config")
	}
}

func TestEnvOverrides(t *testing.T) {
	env := map[string]string{
		"AI_RULEZ_LLM_MODEL": "o", "AI_RULEZ_LLM_ALLOW_NETWORK": "true", "AI_RULEZ_LLM_MAX_COST_USD": "2.5",
		"AI_RULEZ_LLM_MAX_CALLS": "7", "AI_RULEZ_LLM_CACHE": "false", "AI_RULEZ_LLM_BASE_URL": "https://x.example/v1",
	}
	got, err := Config{Model: "base"}.WithEnv(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if got.Model != "o" || !got.AllowNetwork || got.MaxCostUSD != 2.5 || got.MaxCalls != 7 || got.CacheEnabled() || got.BaseURL != "https://x.example/v1" {
		t.Fatalf("%+v", got)
	}
	if _, err := (Config{}).WithEnv(func(k string) string {
		if k == "AI_RULEZ_LLM_MAX_CALLS" {
			return "many"
		}
		return ""
	}); err == nil || !strings.Contains(err.Error(), "AR9L0") {
		t.Fatalf("bad env value: %v", err)
	}
	if !(Config{}).CacheEnabled() || (Config{}).AllowNetwork {
		t.Fatal("defaults: cache on, network off")
	}
}

func TestJudge(t *testing.T) {
	f := NewFake()
	f.ChatFunc = func(r ChatRequest) (string, error) {
		if r.ResponseFormat == nil || r.PromptVersion != JudgePromptVersion || r.Temperature != 0 {
			return "", errors.New("judge must use structured output, temperature 0 and its prompt version")
		}
		return "```json\n{\"score\":0.75,\"rationale\":\"mostly\"}\n```", nil
	}
	v, err := Judge(context.Background(), f, "must say hi", "assistant: hi")
	if err != nil || v.Score != 0.75 || v.Rationale != "mostly" {
		t.Fatalf("%+v %v", v, err)
	}
	for _, bad := range []string{`not json`, `{"score":2,"rationale":"x"}`, `{"score":1,"rationale":"x","extra":1}`} {
		f.ChatFunc = func(ChatRequest) (string, error) { return bad, nil }
		if _, err := Judge(context.Background(), f, "r", "t"); err == nil {
			t.Fatalf("%q should be rejected", bad)
		}
	}
}

type stubNative struct {
	chat, embed func([]byte) ([]byte, error)
	freed       bool
}

func (s *stubNative) ChatJSON(_ context.Context, b []byte) ([]byte, error)  { return s.chat(b) }
func (s *stubNative) EmbedJSON(_ context.Context, b []byte) ([]byte, error) { return s.embed(b) }
func (s *stubNative) Free()                                                 { s.freed = true }

// stubNativeErr is a typed native failure.
type stubNativeErr struct {
	variant    string
	msg        string
	status     int
	transient  bool
	retryAfter time.Duration
}

func (e *stubNativeErr) Error() string                   { return e.msg }
func (e *stubNativeErr) NativeVariant() string           { return e.variant }
func (e *stubNativeErr) NativeStatus() int               { return e.status }
func (e *stubNativeErr) NativeTransient() bool           { return e.transient }
func (e *stubNativeErr) NativeRetryAfter() time.Duration { return e.retryAfter }

func TestLiterLLMAdapterWithStub(t *testing.T) {
	stub := &stubNative{
		chat: func(b []byte) ([]byte, error) {
			var w wireChatRequest
			if err := json.Unmarshal(b, &w); err != nil || w.Model != "openai/gpt-4o-mini" {
				return nil, fmt.Errorf("bad request %s", b)
			}
			return []byte(`{"model":"gpt-4o-mini","choices":[{"message":{"content":[{"type":"text","text":"he"},{"type":"text","text":"llo"}]}}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`), nil
		},
		embed: func([]byte) ([]byte, error) {
			return nil, fmt.Errorf("embed: %w", &stubNativeErr{variant: "RateLimited", msg: "rate limited: slow down sk-abcdefghijklmnop", status: 429, transient: true})
		},
	}
	nativeMu.RLock()
	previous := nativeFactory
	nativeMu.RUnlock()
	RegisterNative(func(NativeConfig) (NativeClient, error) { return stub, nil })
	t.Cleanup(func() { RegisterNative(previous) })
	if !NativeAvailable() || ResolveBackend("auto") != BackendLiterLLM || ResolveBackend("openaicompat") != BackendOpenAICompat {
		t.Fatal("auto must pick literllm when compiled in")
	}
	m, err := New(allowed(Config{Backend: BackendAuto, Provider: "openai", Model: "gpt-4o-mini", EmbeddingModel: "text-embedding-3-small", Cache: ptr(false)}), Options{Retry: &RetryPolicy{}})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := m.Chat(context.Background(), chatReq("hi"))
	if err != nil || resp.Text != "hello" || resp.Usage.Total() != 5 {
		t.Fatalf("%+v %v", resp, err)
	}
	_, err = m.Embed(context.Background(), EmbedRequest{Input: []string{"a"}})
	if !errors.Is(err, ErrRateLimit) || strings.Contains(err.Error(), "abcdefghij") {
		t.Fatalf("native errors must be classified and redacted: %v", err)
	}
	m.Close()
	if !stub.freed {
		t.Fatal("Close must free the native client")
	}
}

func TestNativeCallHonoursContext(t *testing.T) {
	// Arrange: a native client that, like liter-llm 2.1.3, returns the context error when ctx ends.
	stub := &stubNative{chat: nil}
	l := &literLLM{native: &ctxNative{stubNative: stub}, model: "m"}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	// Act / Assert
	if _, err := l.Chat(ctx, chatReq("x")); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want deadline, got %v", err)
	}
}

type ctxNative struct{ *stubNative }

func (c *ctxNative) ChatJSON(ctx context.Context, _ []byte) ([]byte, error) {
	<-ctx.Done()
	return nil, fmt.Errorf("native aborted: %w", ctx.Err())
}

func TestDiagnoseAndEstimate(t *testing.T) {
	env := map[string]string{"MY_KEY": "sk-should-never-appear"}
	cfg := Config{Provider: "openai", Model: "gpt-4o-mini", BaseURL: "https://gw.internal:8443/v1", APIKeyEnv: "MY_KEY", MaxCostUSD: 3}
	d := Diagnose(cfg, Options{ConfigDir: "/repo/.ai-rulez", Getenv: func(k string) string { return env[k] }})
	var buf bytes.Buffer
	d.WriteText(&buf)
	out := buf.String()
	if strings.Contains(out, "should-never-appear") || !strings.Contains(out, "gw.internal:8443") || strings.Contains(out, "/v1") ||
		!strings.Contains(out, "network allowed: false") || !strings.Contains(out, filepath.Join("ai-rulez", "llm")) || !d.APIKeySet {
		t.Fatalf("diagnosis:\n%s", out)
	}
	if err := Ping(context.Background(), cfg, Options{}); !errors.Is(err, ErrNetworkDisabled) {
		t.Fatalf("ping must refuse without allow_network: %v", err)
	}
	est := Estimate(cfg, strings.Repeat("a", 4000), 100)
	if !est.CostKnown || est.PromptTokens < 1000 || est.MaxOutput != 100 || est.CostUSD <= 0 {
		t.Fatalf("%+v", est)
	}
	if Estimate(Config{Model: "mystery"}, "x", 0).CostKnown {
		t.Fatal("unknown model has no price")
	}
	custom := Estimate(Config{Model: "mystery", PriceInputPerMTok: 1, PriceOutputPerMTok: 2}, strings.Repeat("a", 4000), 1000)
	if !custom.CostKnown {
		t.Fatal("price override must make the cost known")
	}
}

func TestErrorKinds(t *testing.T) {
	if !errors.Is(fmt.Errorf("wrap: %w", &Error{Kind: KindAuth, Status: 401, Message: "x"}), ErrAuth) {
		t.Fatal("errors.Is must match by kind through wrapping")
	}
	if errors.Is(&Error{Kind: KindAuth}, ErrRateLimit) {
		t.Fatal("different kinds must not match")
	}
	if IsTransient(io.EOF) || !IsTransient(&Error{Kind: KindProvider}) || IsTransient(&Error{Kind: KindProvider, Status: 400}) {
		t.Fatal("transient classification")
	}
}

// Gemini's native route behind liter-llm answers a batch embed with one vector
// whatever the batch size; the backend must still return one vector per input.
func TestLiterLLMEmbedShouldFallBackToPerInputCallsWhenBatchIsCollapsed(t *testing.T) {
	// Arrange
	calls := 0
	stub := &stubNative{embed: func(b []byte) ([]byte, error) {
		calls++
		var in struct {
			Input []string `json:"input"`
		}
		if err := json.Unmarshal(b, &in); err != nil {
			return nil, err
		}
		// Collapses to the first input, like the native Gemini route.
		return []byte(fmt.Sprintf(`{"data":[{"index":0,"embedding":[%d,0]}],"usage":{"prompt_tokens":2}}`, len(in.Input[0]))), nil
	}}
	nativeMu.Lock()
	prev := nativeFactory
	nativeFactory = func(NativeConfig) (NativeClient, error) { return stub, nil }
	nativeMu.Unlock()
	t.Cleanup(func() { nativeMu.Lock(); nativeFactory = prev; nativeMu.Unlock() })
	cfg := Config{Backend: BackendLiterLLM, Provider: "gemini", Model: "m", EmbeddingModel: "e", APIKeyEnv: "K", AllowNetwork: true, Cache: ptr(false)}
	c, err := newLiterLLM(cfg, func(string) string { return "k" })
	if err != nil {
		t.Fatal(err)
	}

	// Act
	resp, err := c.Embed(context.Background(), EmbedRequest{Input: []string{"a", "bb", "ccc"}})

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
	if calls != 4 {
		t.Errorf("native calls = %d, want 1 batch + 3 singles", calls)
	}
}

func TestRedactSecretsShouldKeepEnvironmentLookupsAndMaskLiterals(t *testing.T) {
	tests := []struct {
		name, in string
		changed  bool
	}{
		{"python environ lookup", `token = os.environ["SERVICE_TOKEN"]`, false},
		{"go getenv", `apiKey := os.Getenv("SERVICE_TOKEN")`, false},
		{"node env", `const token = process.env.SERVICE_TOKEN`, false},
		{"shell variable", `API_KEY=$SERVICE_TOKEN_VALUE`, false},
		{"literal token", `token = "abcdef0123456789"`, true},
		{"literal key", `api_key: hunter2hunter2`, true},
		{"literal that starts like a lookup", `secret = osprey-secret-value-123`, true},
		{"shell default literal key", `OPENAI_API_KEY=${OPENAI_API_KEY:-sk-abcdefghijklmnopqrstuv}`, true},
		{"shell default literal aws key", `api_key: ${KEY:-AKIAABCDEFGHIJKLMNOP}`, true},
		{"env call with literal inside", `api_key=env(sk-abcdefghijklmnopqrstuv)`, true},
		{"variable followed by literal", `token = $HOME-hunter2hunter2`, true},
		{"environ get with literal default", `token = os.environ.get("X", "hunter2hunter2")`, true},
		{"prefix that is a variable", `token = $ghp_realtokenvalue1234567890`, false},
		{"braced variable", `API_KEY=${SERVICE_TOKEN}`, false},
		{"environ get lookup", `token = os.environ.get("SERVICE_TOKEN")`, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			got := RedactSecrets(tc.in)

			// Assert
			if changed := got != tc.in; changed != tc.changed {
				t.Errorf("RedactSecrets(%q) = %q, changed=%v want %v", tc.in, got, changed, tc.changed)
			}
		})
	}
}

// Once a batch has come back collapsed, later batches skip the wasted batch request, and the
// budget counts every provider request against max_calls.
func TestLiterLLMEmbedShouldRememberCollapsedBatchesAndCountEveryRequest(t *testing.T) {
	// Arrange
	calls := 0
	stub := &stubNative{embed: func(b []byte) ([]byte, error) {
		calls++
		var in struct {
			Input []string `json:"input"`
		}
		if err := json.Unmarshal(b, &in); err != nil {
			return nil, err
		}
		return []byte(`{"data":[{"index":0,"embedding":[1,0]}],"usage":{"prompt_tokens":2}}`), nil
	}}
	l := &literLLM{native: stub, provider: "gemini", model: "gemini/m", embedModel: "gemini/e"}
	budget := NewBudget(Limits{MaxCalls: 100}, NewPricing(Config{}))
	c := WithBudget(l, budget, "gemini/m", "gemini/e")
	req := EmbedRequest{Input: []string{"a", "b", "c"}}

	// Act
	first, err1 := c.Embed(context.Background(), req)
	callsAfterFirst := calls
	second, err2 := c.Embed(context.Background(), req)

	// Assert
	if err1 != nil || err2 != nil {
		t.Fatalf("embed: %v %v", err1, err2)
	}
	if callsAfterFirst != 4 || calls != 7 {
		t.Errorf("native calls = %d after the first batch and %d after the second, want 4 and 7 (the second skips the batch request)", callsAfterFirst, calls)
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
