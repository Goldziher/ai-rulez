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
	m := Wrap(b, allowed(Config{Model: "gpt-4o-mini", MaxTokens: worst * 10, Cache: ptr(false)}), Options{})
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
	m := Wrap(&negEmbed{}, allowed(Config{EmbeddingModel: "text-embedding-3-small", MaxTokens: 1000, Cache: ptr(false)}), Options{})
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
