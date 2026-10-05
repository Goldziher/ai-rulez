package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLiteralKeyInAPIKeyEnvIsNeverEchoed(t *testing.T) {
	for _, secret := range []string{"Zq9-Lk2mNp4RsT7vWx0Yb3Cd", "sk-proj-abc123def456ghi789", "my secret key 123"} {
		cfg := Config{Model: "x", APIKeyEnv: secret, AllowNetwork: true, Backend: BackendOpenAICompat}
		var all strings.Builder
		for _, p := range cfg.Validate() {
			all.WriteString(p + "\n")
		}
		if err := cfg.Err(); err != nil {
			all.WriteString(err.Error() + "\n")
		}
		d := Diagnose(cfg, Options{Getenv: func(string) string { return "set" }})
		var text bytes.Buffer
		d.WriteText(&text)
		all.Write(text.Bytes())
		js, err := json.Marshal(d)
		if err != nil {
			t.Fatal(err)
		}
		all.Write(js)
		if _, err := New(cfg, Options{}); err != nil {
			all.WriteString(err.Error())
		}
		if strings.Contains(all.String(), secret) {
			t.Errorf("literal key %q echoed:\n%s", secret, all.String())
		}
		if d.APIKeySet {
			t.Errorf("an invalid variable name must not be looked up")
		}
	}
	// invalid values of other keys are not echoed either
	cfg := Config{Backend: "sk-live-abcdef0123456789", Model: "a b"}
	if strings.Contains(strings.Join(cfg.Validate(), "|"), "sk-live") {
		t.Error("backend value echoed")
	}
	if _, err := (Config{}).WithEnv(func(k string) string {
		if k == "AI_RULEZ_LLM_MAX_CALLS" {
			return "sk-live-abcdef0123456789"
		}
		return ""
	}); err == nil || strings.Contains(err.Error(), "sk-live") {
		t.Errorf("env value echoed: %v", err)
	}
}

func TestOpenAICompatNeverFollowsRedirects(t *testing.T) {
	var otherHits atomic.Int32
	var otherBody atomic.Value
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherHits.Add(1)
		b, _ := io.ReadAll(r.Body) //nolint:errcheck // test
		otherBody.Store(string(b))
		fmt.Fprint(w, `{"choices":[{"message":{"content":"x"}}]}`)
	}))
	defer other.Close()
	// a second loopback name is a different host for the redirect policy
	target := strings.Replace(other.URL, "127.0.0.1", "localhost", 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target+"/other", http.StatusTemporaryRedirect)
	}))
	defer srv.Close()
	env := map[string]string{"K": "sk-live-very-secret-123456"}
	// no HTTPClient injected: the default client must refuse the hop
	m, err := New(allowed(Config{Model: "m", BaseURL: srv.URL + "/v1", APIKeyEnv: "K", Cache: ptr(false), MaxRetries: -1}), Options{Getenv: func(k string) string { return env[k] }})
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.Chat(context.Background(), chatReq("private prompt"))
	if err == nil || !errors.Is(err, ErrProvider) {
		t.Fatalf("a redirect must surface as a provider error, got %v", err)
	}
	if otherHits.Load() != 0 {
		t.Fatalf("prompt was re-POSTed to another host: %v", otherBody.Load())
	}
	// an injected client is wrapped too
	m, err = New(allowed(Config{Model: "m", BaseURL: srv.URL + "/v1", APIKeyEnv: "K", Cache: ptr(false), MaxRetries: -1}), Options{Getenv: func(k string) string { return env[k] }, HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.Chat(context.Background(), chatReq("private prompt")); err == nil || otherHits.Load() != 0 {
		t.Fatalf("injected client followed a redirect: %v hits=%d", err, otherHits.Load())
	}
}

func TestPlainHTTPWithKeyNeedsLoopback(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		bad  bool
	}{
		{"https remote with key", Config{BaseURL: "https://gw.example/v1", APIKeyEnv: "K"}, false},
		{"http remote with key", Config{BaseURL: "http://gw.example/v1", APIKeyEnv: "K"}, true},
		{"http remote no key", Config{BaseURL: "http://ollama.lan:11434/v1"}, false},
		{"http localhost with key", Config{BaseURL: "http://localhost:8080/v1", APIKeyEnv: "K"}, false},
		{"http 127.0.0.1 with key", Config{BaseURL: "http://127.0.0.1:8080/v1", APIKeyEnv: "K"}, false},
		{"http ::1 with key", Config{BaseURL: "http://[::1]:8080/v1", APIKeyEnv: "K"}, false},
		{"http lookalike with key", Config{BaseURL: "http://localhost.evil.example/v1", APIKeyEnv: "K"}, true},
	}
	for _, tc := range cases {
		got := strings.Contains(strings.Join(tc.cfg.Validate(), "|"), "https")
		if got != tc.bad {
			t.Errorf("%s: problems=%v want bad=%v", tc.name, tc.cfg.Validate(), tc.bad)
		}
	}
}

func TestEstimateTokensIsConservativeForNonASCII(t *testing.T) {
	cjk := strings.Repeat("日本語のテキスト", 100) // ~1 token per rune in practice
	runes := len([]rune(cjk))
	if got := EstimateTokens(cjk); got < runes {
		t.Errorf("CJK: estimate %d is below the rune count %d", got, runes)
	}
	ascii := strings.Repeat("abcd", 100) // typical English/code is at most 4 bytes per token
	if got := EstimateTokens(ascii); got < len(ascii)/4 {
		t.Errorf("ASCII: estimate %d below len/4", got)
	}
	if EstimateTokens("") != 0 || EstimateTokens("a") != 1 {
		t.Error("edge cases")
	}
}

// slowBackend answers after a pause, so concurrent callers overlap in flight.
type slowBackend struct {
	Fake
	calls atomic.Int32
	model string
	usage Usage
	err   error
}

func (s *slowBackend) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	s.calls.Add(1)
	time.Sleep(20 * time.Millisecond)
	if s.err != nil {
		return ChatResponse{}, s.err
	}
	return ChatResponse{Text: "ok", Model: s.model, Usage: s.usage}, nil
}

func TestBudgetConcurrentCallsCannotOvershoot(t *testing.T) {
	const workers = 40
	req := chatReq("hello")
	req.MaxTokens = 100
	worstTokens := EstimatePromptTokens(req) + 100
	cases := []struct {
		name  string
		cfg   Config
		admit int // the most calls the limit can admit
	}{
		{"max_calls", Config{Model: "gpt-4o-mini", MaxCalls: 5}, 5},
		{"max_tokens", Config{Model: "gpt-4o-mini", MaxTokens: worstTokens*3 + 1}, 3},
		{"max_cost_usd", Config{Model: "gpt-4o-mini", PriceInputPerMTok: 1_000_000, PriceOutputPerMTok: 1_000_000, MaxCostUSD: float64(worstTokens)*2 + 1}, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := &slowBackend{model: "gpt-4o-mini", usage: Usage{PromptTokens: worstTokens - 100, CompletionTokens: 100}}
			cfg := allowed(tc.cfg)
			cfg.Cache = ptr(false)
			m := Wrap(b, cfg, Options{Retry: &RetryPolicy{}})
			var wg sync.WaitGroup
			var ok atomic.Int32
			for range workers {
				wg.Add(1)
				go func() {
					defer wg.Done()
					if _, err := m.Chat(context.Background(), req); err == nil {
						ok.Add(1)
					} else if !errors.Is(err, ErrBudget) {
						t.Errorf("unexpected error: %v", err)
					}
				}()
			}
			wg.Wait()
			if int(ok.Load()) > tc.admit || int(b.calls.Load()) > tc.admit {
				t.Fatalf("admitted %d calls (backend saw %d), limit allows %d", ok.Load(), b.calls.Load(), tc.admit)
			}
			if ok.Load() == 0 {
				t.Fatal("at least one call must fit")
			}
			sp := m.Spent()
			if (cfg.MaxTokens > 0 && sp.Tokens > cfg.MaxTokens) || (cfg.MaxCostUSD > 0 && sp.CostUSD > cfg.MaxCostUSD) || (cfg.MaxCalls > 0 && sp.Calls > cfg.MaxCalls) {
				t.Fatalf("overshoot: %+v", sp)
			}
		})
	}
}

func TestBudgetChargesRequestedModelNotTheReportedOne(t *testing.T) {
	// The provider reports a model name the table does not know. It must not charge $0.
	b := &slowBackend{model: "totally-unpriced-model", usage: Usage{PromptTokens: 1_000_000, CompletionTokens: 1_000_000}}
	cfg := allowed(Config{Model: "gpt-4o", MaxCostUSD: 100, Cache: ptr(false)})
	m := Wrap(b, cfg, Options{Retry: &RetryPolicy{}})
	resp, err := m.Chat(context.Background(), chatReq("x"))
	if err != nil {
		t.Fatal(err)
	}
	// gpt-4o: 1M in at $2.50 + 1M out at $10
	if sp := m.Spent(); sp.CostUSD < 12.49 || sp.CostUSD > 12.51 || !resp.CostKnown || resp.CostUSD < 12.49 {
		t.Fatalf("charged %+v, response cost %v known=%v", sp, resp.CostUSD, resp.CostKnown)
	}
	// a cheaper reported name must not lower the price either
	b.model = "gpt-4o-mini"
	before := m.Spent().CostUSD
	if _, err := m.Chat(context.Background(), chatReq("y")); err != nil {
		t.Fatal(err)
	}
	if got := m.Spent().CostUSD - before; got < 12.49 {
		t.Fatalf("second call charged %v at the reported model's price", got)
	}
	// with a cost cap, a requested model without a price is refused before any call
	b2 := &slowBackend{model: "custom", usage: Usage{PromptTokens: 1, CompletionTokens: 1}}
	m2 := Wrap(b2, allowed(Config{Model: "custom", MaxCostUSD: 1, Cache: ptr(false)}), Options{Retry: &RetryPolicy{}})
	if _, err := m2.Chat(context.Background(), chatReq("x")); !errors.Is(err, ErrBudget) || b2.calls.Load() != 0 {
		t.Fatalf("unknown price under a cost cap must be refused: %v", err)
	}
}

func TestBudgetAmbiguousFailuresChargeTheWorstCase(t *testing.T) {
	req := chatReq("x")
	req.MaxTokens = 100
	worst := EstimatePromptTokens(req) + 100
	run := func(err error) Spent {
		b := &slowBackend{err: err}
		m := Wrap(b, allowed(Config{Model: "gpt-4o-mini", MaxCalls: 10, Cache: ptr(false)}), Options{Retry: &RetryPolicy{}})
		_, _ = m.Chat(context.Background(), req) //nolint:errcheck // the error is the input
		return m.Spent()
	}
	if sp := run(&Error{Kind: KindProvider, Status: 400, Message: "bad"}); sp.Tokens != 0 || sp.Calls != 1 {
		t.Errorf("a clean 4xx is not billed: %+v", sp)
	}
	if sp := run(&Error{Kind: KindAuth, Status: 401}); sp.Tokens != 0 {
		t.Errorf("auth failure is not billed: %+v", sp)
	}
	for name, err := range map[string]error{
		"timeout":   &Error{Kind: KindTimeout, Message: "slow"},
		"transport": &Error{Kind: KindProvider, Message: "request failed"},
		"cancelled": context.DeadlineExceeded,
	} {
		if sp := run(err); sp.Tokens != worst || sp.CostUSD <= 0 {
			t.Errorf("%s may have been billed, want worst case %d: %+v", name, worst, sp)
		}
	}
}
