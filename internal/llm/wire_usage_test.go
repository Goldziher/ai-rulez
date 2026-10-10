package llm

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	lit "github.com/xberg-io/liter-llm/packages/go/v2"
)

// usageServer answers every chat request with a reply that reports usage verbatim. The provider
// is openai so a base_url endpoint stays the generic OpenAI-compatible route: the reply shape the
// tests assert is OpenAI's, not Gemini's.
func usageServer(t *testing.T, usage string) *literLLM {
	t.Helper()
	body := `{"id":"x","object":"chat.completion","created":1,"model":"gemini-2.5-flash","choices":[{"index":0,"message":{"role":"assistant","content":"0.05"},"finish_reason":"stop"}]` + usage + `}`
	srv := serve(t, func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, body) })
	return newLocal(t, localConfig(srv, Config{Provider: "openai", Model: "gemini-2.5-flash", MaxRetries: -1, PriceInputPerMTok: 1, PriceOutputPerMTok: 1}), nil)
}

// Some providers report completion_tokens without the thinking tokens they bill at the output
// rate; they show only in total_tokens (or in completion_tokens_details.reasoning_tokens). The call
// must be charged for them, or every spend cap is exceeded several times over (RV-LLM-1).
func TestChatShouldChargeThinkingTokens(t *testing.T) {
	tests := []struct {
		name           string
		usage          string
		wantPrompt     int
		wantCompletion int
		wantCached     int
	}{
		{"total includes thinking", `{"prompt_tokens":35,"completion_tokens":4,"total_tokens":442}`, 35, 407, 0},
		{"openai total equals the sum", `{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}`, 10, 20, 0},
		{"no total", `{"prompt_tokens":10,"completion_tokens":20}`, 10, 20, 0},
		{"total below the sum is ignored", `{"prompt_tokens":10,"completion_tokens":20,"total_tokens":5}`, 10, 20, 0},
		{"reasoning inside completion is not added twice", `{"prompt_tokens":10,"completion_tokens":50,"completion_tokens_details":{"reasoning_tokens":40}}`, 10, 50, 0},
		{"reasoning reported beside a smaller completion", `{"prompt_tokens":10,"completion_tokens":4,"completion_tokens_details":{"reasoning_tokens":400}}`, 10, 404, 0},
		{"cached prompt tokens are carried", `{"prompt_tokens":100,"completion_tokens":4,"prompt_tokens_details":{"cached_tokens":80}}`, 100, 4, 80},
		{"cached tokens never exceed the prompt", `{"prompt_tokens":10,"completion_tokens":4,"prompt_tokens_details":{"cached_tokens":80}}`, 10, 4, 10},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			l := usageServer(t, `,"usage":`+tc.usage)

			resp, err := l.Chat(context.Background(), chatReq("q"))

			if err != nil {
				t.Fatalf("chat: %v", err)
			}
			if resp.Usage.PromptTokens != tc.wantPrompt || resp.Usage.CompletionTokens != tc.wantCompletion || resp.Usage.CachedTokens != tc.wantCached {
				t.Errorf("usage = %+v, want prompt %d completion %d cached %d", resp.Usage, tc.wantPrompt, tc.wantCompletion, tc.wantCached)
			}
		})
	}
}

// A hostile or broken reply with a negative count is refused: it must never credit the budget.
func TestChatRefusesNegativeUsage(t *testing.T) {
	l := usageServer(t, `,"usage":{"prompt_tokens":-5,"completion_tokens":-9,"total_tokens":-1}`)

	_, err := l.Chat(context.Background(), chatReq("q"))

	if err == nil || IsTransient(err) {
		t.Fatalf("a negative usage must be a permanent error, got %v", err)
	}
}

// The budget settles on the corrected usage, so a run of thinking calls stops at the cap.
func TestBudgetShouldStopThinkingCallsAtTheCap(t *testing.T) {
	// Arrange: each call reports 4 completion tokens but bills 1000 through total_tokens.
	f := NewFake()
	calls := 0
	backend := backendFunc(func(context.Context, ChatRequest) (ChatResponse, error) {
		calls++
		return ChatResponse{Text: "x", Model: "m", Usage: usageOf(&lit.Usage{PromptTokens: 10, CompletionTokens: 4, TotalTokens: 1010})}, nil
	}, f)
	cfg := Config{AllowNetwork: true, Model: "m", MaxTokens: 3000, PriceInputPerMTok: 1, PriceOutputPerMTok: 1}
	m := Wrap(backend, cfg, Options{NoCache: true})

	// Act
	var err error
	for i := 0; i < 10 && err == nil; i++ {
		_, err = m.Chat(context.Background(), ChatRequest{MaxTokens: 1000, Messages: []Message{{Role: RoleUser, Content: fmt.Sprint(i)}}})
	}

	// Assert
	if err == nil {
		t.Fatal("ten 1010-token calls fit a 3000-token cap")
	}
	if calls != 2 || m.Spent().Tokens != 2020 {
		t.Errorf("calls = %d spent = %+v, want 2 calls charged 2020 tokens before the cap refuses", calls, m.Spent())
	}
}

// backendFunc is a Client whose Chat is fn; Embed and Close go to rest.
type backendFuncClient struct {
	fn func(context.Context, ChatRequest) (ChatResponse, error)
	Client
}

func backendFunc(fn func(context.Context, ChatRequest) (ChatResponse, error), rest Client) Client {
	return &backendFuncClient{fn: fn, Client: rest}
}

func (b *backendFuncClient) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	return b.fn(ctx, req)
}

// A reply the provider billed is charged as reported, not at the completion cap.
func TestChatShouldChargeTheReportedUsage(t *testing.T) {
	// 35 prompt, 4 visible and 403 thinking tokens, total 442 (what liter-llm's Gemini mapping returns).
	l := usageServer(t, `,"usage":{"prompt_tokens":35,"completion_tokens":407,"total_tokens":442,"completion_tokens_details":{"reasoning_tokens":403}}`)

	resp, err := l.Chat(context.Background(), ChatRequest{MaxTokens: 800, Messages: []Message{{Role: RoleUser, Content: "q"}}})

	if err != nil {
		t.Fatal(err)
	}
	if resp.Usage.PromptTokens != 35 || resp.Usage.CompletionTokens != 407 {
		t.Errorf("usage = %+v, want the reported prompt 35 and completion 407", resp.Usage)
	}
	if want := 442.0 / 1e6; resp.CostUSD != want || !resp.CostKnown {
		t.Errorf("cost = %v (known %v), want %v", resp.CostUSD, resp.CostKnown, want)
	}
}

// A successful reply with no usage at all still fails closed to the worst case.
func TestBudgetShouldChargeTheCapWhenAReplyReportsNoUsage(t *testing.T) {
	// Arrange
	l := usageServer(t, ``)
	cfg := Config{AllowNetwork: true, Model: "gemini-2.5-flash", MaxTokens: 3000, PriceInputPerMTok: 1, PriceOutputPerMTok: 1}
	m := Wrap(l, cfg, Options{NoCache: true})

	// Act
	_, err := m.Chat(context.Background(), ChatRequest{MaxTokens: 800, Messages: []Message{{Role: RoleUser, Content: "q"}}})

	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Spent().Tokens; got < 800 {
		t.Errorf("spent %d tokens, want at least the 800-token cap when no usage is reported", got)
	}
}
