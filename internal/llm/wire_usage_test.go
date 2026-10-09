package llm

import (
	"context"
	"fmt"
	"testing"
)

// Gemini reports completion_tokens without the thinking tokens it bills at the output rate; they
// show only in total_tokens (or in completion_tokens_details.reasoning_tokens). The call must be
// charged for them, or every spend cap is exceeded several times over (RV-LLM-1).
func TestDecodeChatShouldChargeThinkingTokens(t *testing.T) {
	tests := []struct {
		name           string
		usage          string
		wantPrompt     int
		wantCompletion int
	}{
		{"gemini total includes thinking", `{"prompt_tokens":35,"completion_tokens":4,"total_tokens":442}`, 35, 407},
		{"openai total equals the sum", `{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}`, 10, 20},
		{"no total", `{"prompt_tokens":10,"completion_tokens":20}`, 10, 20},
		{"total below the sum is ignored", `{"prompt_tokens":10,"completion_tokens":20,"total_tokens":5}`, 10, 20},
		{"reasoning inside completion is not added twice", `{"prompt_tokens":10,"completion_tokens":50,"completion_tokens_details":{"reasoning_tokens":40}}`, 10, 50},
		{"reasoning reported beside a smaller completion", `{"prompt_tokens":10,"completion_tokens":4,"completion_tokens_details":{"reasoning_tokens":400}}`, 10, 404},
		{"negative values are not credits", `{"prompt_tokens":-5,"completion_tokens":-9,"total_tokens":-1}`, 0, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			body := []byte(`{"model":"gemini-2.5-flash","choices":[{"message":{"content":"0.05"}}],"usage":` + tc.usage + `}`)

			// Act
			resp, err := decodeChat(body, NewPricing(Config{PriceInputPerMTok: 1, PriceOutputPerMTok: 1}), "gemini-2.5-flash")

			// Assert
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if resp.Usage.PromptTokens != tc.wantPrompt || resp.Usage.CompletionTokens != tc.wantCompletion {
				t.Errorf("usage = %+v, want prompt %d completion %d", resp.Usage, tc.wantPrompt, tc.wantCompletion)
			}
		})
	}
}

// The budget settles on the corrected usage, so a run of thinking calls stops at the cap.
func TestBudgetShouldStopThinkingCallsAtTheCap(t *testing.T) {
	// Arrange: each call reports 4 completion tokens but bills 1000 through total_tokens.
	f := NewFake()
	calls := 0
	backend := backendFunc(func(context.Context, ChatRequest) (ChatResponse, error) {
		calls++
		return decodeChat([]byte(`{"choices":[{"message":{"content":"x"}}],"usage":{"prompt_tokens":10,"completion_tokens":4,"total_tokens":1010}}`),
			NewPricing(Config{PriceInputPerMTok: 1, PriceOutputPerMTok: 1}), "m")
	}, f)
	cfg := Config{AllowNetwork: true, Model: "m", MaxTokens: 3000, PriceInputPerMTok: 1, PriceOutputPerMTok: 1}
	m := Wrap(backend, cfg, Options{NoCache: true, Retry: &RetryPolicy{}})

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

// liter-llm 2.2.0 maps Gemini's usageMetadata with the thinking tokens inside completion_tokens
// and total_tokens (upstream #253), so a call on any of its three Gemini routes is charged the
// reported usage, not its completion cap. The body is what that mapping returns for
// promptTokenCount 35, candidatesTokenCount 4, thoughtsTokenCount 403, totalTokenCount 442.
func TestLiterLLMShouldChargeTheReportedUsageOnGeminiRoutes(t *testing.T) {
	const reply = `{"choices":[{"message":{"content":"0.05"}}],"usage":{"prompt_tokens":35,"completion_tokens":407,"total_tokens":442,"completion_tokens_details":{"reasoning_tokens":403}}}`
	for _, model := range []string{"gemini/gemini-2.5-flash", "google_ai/gemini-2.5-flash", "vertex_ai/gemini-2.5-flash"} {
		for _, maxTokens := range []int{0, 800, 5000} {
			t.Run(fmt.Sprintf("%s cap %d", model, maxTokens), func(t *testing.T) {
				// Arrange
				stub := &stubNative{chat: func([]byte) ([]byte, error) { return []byte(reply), nil }}
				l := &literLLM{native: stub, provider: modelPrefix(model), model: model, pricing: NewPricing(Config{PriceInputPerMTok: 1, PriceOutputPerMTok: 1})}

				// Act
				resp, err := l.Chat(context.Background(), ChatRequest{MaxTokens: maxTokens, Messages: []Message{{Role: RoleUser, Content: "q"}}})

				// Assert
				if err != nil {
					t.Fatal(err)
				}
				if resp.Usage.PromptTokens != 35 || resp.Usage.CompletionTokens != 407 {
					t.Errorf("usage = %+v, want the reported prompt 35 and completion 407", resp.Usage)
				}
				if want := 442.0 / 1e6; resp.CostUSD != want || !resp.CostKnown {
					t.Errorf("cost = %v (known %v), want %v", resp.CostUSD, resp.CostKnown, want)
				}
			})
		}
	}
}

// A successful reply with no usage at all still fails closed to the worst case.
func TestBudgetShouldChargeTheCapWhenAGeminiReplyReportsNoUsage(t *testing.T) {
	// Arrange
	stub := &stubNative{chat: func([]byte) ([]byte, error) {
		return []byte(`{"choices":[{"message":{"content":"0.05"}}]}`), nil
	}}
	model := "gemini/gemini-2.5-flash"
	l := &literLLM{native: stub, provider: "gemini", model: model, pricing: NewPricing(Config{PriceInputPerMTok: 1, PriceOutputPerMTok: 1})}
	cfg := Config{AllowNetwork: true, Model: model, MaxTokens: 3000, PriceInputPerMTok: 1, PriceOutputPerMTok: 1}
	m := Wrap(l, cfg, Options{NoCache: true, Retry: &RetryPolicy{}})

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
