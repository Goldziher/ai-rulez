package llm

import (
	"fmt"

	"github.com/Goldziher/ai-rulez/v5/internal/pricing"
)

// PriceTableVersion identifies the built-in price table (internal/pricing, shared
// with the eval estimate). It is part of the cache identity, so a cost recorded
// under old prices is never replayed under new ones.
const PriceTableVersion = pricing.Version

// Pricing resolves prices for a model.
type Pricing struct {
	input, output float64
	override      bool
}

// NewPricing returns a Pricing that uses the config override when set.
func NewPricing(cfg Config) Pricing {
	if cfg.PriceInputPerMTok > 0 || cfg.PriceOutputPerMTok > 0 {
		return Pricing{input: cfg.PriceInputPerMTok, output: cfg.PriceOutputPerMTok, override: true}
	}
	return Pricing{}
}

// identity names the price source for the cache identity.
func (p Pricing) identity() string {
	if p.override {
		return fmt.Sprintf("override:%g:%g", p.input, p.output)
	}
	return "builtin:" + PriceTableVersion
}

func (p Pricing) lookup(model string) (pricing.Price, bool) {
	if p.override {
		return pricing.Price{InPerMTok: p.input, OutPerMTok: p.output}, true
	}
	return pricing.Lookup(model)
}

// Cost estimates the cost of usage on model. known is false when no price exists.
func (p Pricing) Cost(model string, u Usage) (usd float64, known bool) {
	pr, ok := p.lookup(model)
	if !ok {
		return 0, false
	}
	return (float64(u.PromptTokens)*pr.InPerMTok + float64(u.CompletionTokens)*pr.OutPerMTok) / 1e6, true
}

// BytesPerTokenEstimate is the divisor behind EstimateTokens. English prose runs
// near four bytes per token and dense code or non-Latin text near one to three,
// so three bytes per token overestimates typical input and never undercounts a
// CJK rune (three bytes, about one token).
const BytesPerTokenEstimate = 3

// EstimateTokens approximates the token count of text from its UTF-8 length
// (one token per BytesPerTokenEstimate bytes, rounded up). It is deliberately
// conservative, for budget decisions only; it is not a tokenizer.
func EstimateTokens(text string) int {
	if text == "" {
		return 0
	}
	return (len(text) + BytesPerTokenEstimate - 1) / BytesPerTokenEstimate
}

// EstimatePromptTokens approximates the prompt size of a chat request,
// including a small per-message framing overhead.
func EstimatePromptTokens(req ChatRequest) int {
	total := 3
	for _, m := range req.Messages {
		total += EstimateTokens(m.Content) + 4
	}
	if req.ResponseFormat != nil {
		total += EstimateTokens(req.ResponseFormat.Name) + EstimateTokens(mustJSON(req.ResponseFormat.Schema)) + 16
	}
	return total
}
