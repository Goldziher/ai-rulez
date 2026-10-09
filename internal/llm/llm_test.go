package llm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
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

// allowed enables the network.
func allowed(cfg Config) Config {
	cfg.AllowNetwork = true
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
		m := Wrap(f, allowed(Config{Model: "gpt-4o-mini", MaxCalls: 2, Cache: ptr(false)}), Options{})
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
	m := Wrap(f, cfg, Options{ConfigDir: dir})

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

func TestConfigValidation(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want string // substring of a problem; empty means valid
	}{
		{"empty is valid", Config{}, ""},
		{"full valid", Config{BaseURL: "https://gw.internal/v1", APIKeyEnv: "OPENAI_API_KEY", MaxCostUSD: 1}, ""},
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
	err := Config{APIKeyEnv: "my key"}.Err()
	if err == nil || !strings.Contains(err.Error(), "AR9L0") || !errors.Is(err, ErrConfig) {
		t.Fatalf("Err must carry AR9L0: %v", err)
	}
	if _, err := New(Config{APIKeyEnv: "my key", AllowNetwork: true}, Options{}); err == nil {
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
