package llm

import (
	"context"
	"os"
	"testing"
)

// Live checks of the non-Gemini providers, gated like the others (AI_RULEZ_LIVE_LLM=1)
// and on the provider's key. Each spends a few tokens.

// liter-llm renames max_tokens to max_completion_tokens for the native OpenAI provider and
// drops the temperature a reasoning model rejects (upstream #264), so gpt-5 accepts a
// max-token cap instead of a 400. Reproduces #298 item 3. Model overridable with
// AI_RULEZ_LIVE_OPENAI_MODEL.
func TestLiveOpenAIReasoningModelAcceptsAMaxTokenCap(t *testing.T) {
	if os.Getenv("AI_RULEZ_LIVE_LLM") != "1" {
		t.Skip("set AI_RULEZ_LIVE_LLM=1 to run live provider tests")
	}
	if os.Getenv("OPENAI_API_KEY") == "" {
		t.Skip("OPENAI_API_KEY is not set")
	}
	model := os.Getenv("AI_RULEZ_LIVE_OPENAI_MODEL")
	if model == "" {
		model = "gpt-5"
	}
	cfg := Config{
		Provider: "openai", Model: model, APIKeyEnv: "OPENAI_API_KEY",
		AllowNetwork: true, Cache: ptr(false), MaxRetries: -1, TimeoutSeconds: 60,
	}
	m, err := New(cfg, Options{Getenv: os.Getenv})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close() //nolint:errcheck // test

	resp, err := m.Chat(context.Background(), ChatRequest{
		Messages:  []Message{{Role: RoleUser, Content: "Reply with the single word: pong"}},
		MaxTokens: 16,
	})
	if err != nil {
		t.Fatalf("%s rejected the max-token cap: %v", model, err)
	}
	if resp.Usage.Total() == 0 || !resp.CostKnown {
		t.Fatalf("%s: no usage or price: %+v", model, resp)
	}
	t.Logf("%s: finish=%q usage=%+v cost=%v known=%v", model, resp.FinishReason, resp.Usage, resp.CostUSD, resp.CostKnown)
}

// The Anthropic route returns prompt/completion usage that ai-rulez settles on (#298 item 3).
// A cache-read specifically needs a cache_control breakpoint, which ai-rulez does not send, so
// only the read/write absence is asserted here. Model overridable with AI_RULEZ_LIVE_ANTHROPIC_MODEL.
func TestLiveAnthropicUsageAccounting(t *testing.T) {
	if os.Getenv("AI_RULEZ_LIVE_LLM") != "1" {
		t.Skip("set AI_RULEZ_LIVE_LLM=1 to run live provider tests")
	}
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY is not set")
	}
	model := os.Getenv("AI_RULEZ_LIVE_ANTHROPIC_MODEL")
	if model == "" {
		model = "claude-haiku-4-5"
	}
	cfg := Config{
		Provider: "anthropic", Model: model, APIKeyEnv: "ANTHROPIC_API_KEY",
		AllowNetwork: true, Cache: ptr(false), MaxRetries: -1, TimeoutSeconds: 60,
	}
	m, err := New(cfg, Options{Getenv: os.Getenv})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close() //nolint:errcheck // test

	resp, err := m.Chat(context.Background(), ChatRequest{
		Messages:  []Message{{Role: RoleUser, Content: "Reply with the single word: pong"}},
		MaxTokens: 16,
	})
	if err != nil {
		t.Fatalf("%s: %v", model, err)
	}
	if resp.Usage.PromptTokens == 0 || resp.Usage.CompletionTokens == 0 || !resp.CostKnown {
		t.Fatalf("%s: usage or price missing: %+v", model, resp)
	}
	t.Logf("%s: usage=%+v cost=%v known=%v", model, resp.Usage, resp.CostUSD, resp.CostKnown)
}
