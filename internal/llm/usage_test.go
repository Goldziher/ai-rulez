package llm

import (
	"context"
	"testing"
)

func TestBudgetIgnoresProviderReportedNegativeUsage(t *testing.T) {
	req := chatReq("x")
	req.MaxTokens = 100
	worst := EstimatePromptTokens(req) + 100
	b := &slowBackend{model: "gpt-4o-mini", usage: Usage{PromptTokens: -1_000_000, CompletionTokens: 1}}
	m := Wrap(b, allowed(Config{Model: "gpt-4o-mini", MaxTokens: worst * 10, Cache: ptr(false)}), Options{Retry: &RetryPolicy{}})
	for range 3 {
		resp, err := m.Chat(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.Usage.PromptTokens < 0 || resp.CostUSD < 0 {
			t.Fatalf("negative usage leaked into the response: %+v", resp)
		}
	}
	if sp := m.Spent(); sp.Tokens < worst*3 {
		t.Fatalf("negative usage credited the budget: spent %+v, want at least %d", sp, worst*3)
	}
}

func TestBudgetIgnoresNegativeEmbedUsage(t *testing.T) {
	m := Wrap(&negEmbed{}, allowed(Config{EmbeddingModel: "text-embedding-3-small", MaxTokens: 1000, Cache: ptr(false)}), Options{Retry: &RetryPolicy{}})
	if _, err := m.Embed(context.Background(), EmbedRequest{Input: []string{"hello world"}}); err != nil {
		t.Fatal(err)
	}
	if sp := m.Spent(); sp.Tokens <= 0 {
		t.Fatalf("negative embed usage credited the budget: %+v", sp)
	}
}

type negEmbed struct{ Fake }

func (*negEmbed) Embed(_ context.Context, req EmbedRequest) (EmbedResponse, error) {
	return EmbedResponse{Vectors: make([][]float32, len(req.Input)), Usage: Usage{PromptTokens: -500}}, nil
}

func TestDecodeClampsNegativeUsage(t *testing.T) {
	resp, err := decodeChat([]byte(`{"choices":[{"message":{"content":"a"}}],"usage":{"prompt_tokens":-5,"completion_tokens":3}}`), Pricing{}, "m")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Usage.PromptTokens < 0 || resp.Usage.CompletionTokens != 3 {
		t.Fatalf("usage not clamped: %+v", resp.Usage)
	}
	er, err := decodeEmbed([]byte(`{"data":[{"index":0,"embedding":[1]}],"usage":{"prompt_tokens":-9}}`), Pricing{}, "m", 1)
	if err != nil {
		t.Fatal(err)
	}
	if er.Usage.PromptTokens < 0 {
		t.Fatalf("embed usage not clamped: %+v", er.Usage)
	}
}
