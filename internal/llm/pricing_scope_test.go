package llm

import (
	"context"
	"errors"
	"testing"
)

func TestPriceOverrideAppliesOnlyToTheUsersModel(t *testing.T) {
	// The user sets a price but no model; the repository chose the model. The
	// override must not price it.
	user := &Config{PriceInputPerMTok: 0.01, PriceOutputPerMTok: 0.01, MaxCostUSD: 1}
	cfg, _ := Resolve(&Config{Model: "claude-opus-4"}, user)
	cost, known := NewPricing(cfg).Cost(cfg.Model, Usage{PromptTokens: 1_000_000})
	if !known || cost < 14 {
		t.Fatalf("repo-chosen model must use the builtin price, got %v known=%v", cost, known)
	}
	// an unknown repo-chosen model under a cost cap is refused, not priced by the override
	cfg, _ = Resolve(&Config{Model: "mystery"}, user)
	b := &slowBackend{model: "mystery", usage: Usage{PromptTokens: 1, CompletionTokens: 1}}
	m := Wrap(b, allowed(cfg), Options{})
	if _, err := m.Chat(context.Background(), chatReq("x")); !errors.Is(err, ErrBudget) || b.calls.Load() != 0 {
		t.Fatalf("unknown repo-chosen model under a cost cap must be refused: %v", err)
	}
	// the user's own model keeps the override, and a per-request model does not inherit it
	cfg, _ = Resolve(&Config{Model: "mystery"}, &Config{Model: "house-model", PriceInputPerMTok: 2, PriceOutputPerMTok: 4})
	if cost, known = NewPricing(cfg).Cost("openai/house-model", Usage{PromptTokens: 1_000_000}); !known || cost != 2 {
		t.Fatalf("user's own model lost its override: %v %v", cost, known)
	}
	if _, known = NewPricing(cfg).Cost("other-model", Usage{PromptTokens: 1}); known {
		t.Fatal("override leaked to another model")
	}
	// a model set by the environment is user scope too
	cfg, _ = Resolve(&Config{Model: "mystery"}, &Config{PriceInputPerMTok: 2, PriceOutputPerMTok: 4})
	cfg, err := cfg.WithEnv(func(k string) string {
		if k == "AI_RULEZ_LLM_MODEL" {
			return "env-model"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	if cost, known = NewPricing(cfg).Cost("env-model", Usage{PromptTokens: 1_000_000}); !known || cost != 2 {
		t.Fatalf("env-chosen model lost the override: %v %v", cost, known)
	}
}
