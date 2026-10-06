package llm

import (
	"fmt"
	"strings"
)

// price is USD per million tokens.
type price struct{ in, out float64 }

// builtinPrices is a small, approximate table for common models. It is a
// convenience for budget estimates, not a billing source: prices change, and
// unknown models have no entry. Override with price_input_per_mtok /
// price_output_per_mtok. Keys are matched against the model name with any
// provider prefix removed, longest prefix first.
var builtinPrices = map[string]price{
	"gpt-4o-mini":            {0.15, 0.60},
	"gpt-4o":                 {2.50, 10.00},
	"gpt-4.1-mini":           {0.40, 1.60},
	"gpt-4.1":                {2.00, 8.00},
	"text-embedding-3-small": {0.02, 0},
	"text-embedding-3-large": {0.13, 0},
	"claude-haiku":           {1.00, 5.00},
	"claude-sonnet":          {3.00, 15.00},
	"claude-opus":            {15.00, 75.00},
}

// PriceTableVersion identifies the built-in price table. Bump it whenever a price
// changes: it is part of the cache identity, so a cost recorded under old prices
// is never replayed under new ones.
const PriceTableVersion = "2026-07"

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

func (p Pricing) lookup(model string) (price, bool) {
	if p.override {
		return price{p.input, p.output}, true
	}
	name := strings.ToLower(model)
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	best, bestLen, found := price{}, 0, false
	for prefix, pr := range builtinPrices {
		if strings.HasPrefix(name, prefix) && len(prefix) > bestLen {
			best, bestLen, found = pr, len(prefix), true
		}
	}
	return best, found
}

// Cost estimates the cost of usage on model. known is false when no price exists.
func (p Pricing) Cost(model string, u Usage) (usd float64, known bool) {
	pr, ok := p.lookup(model)
	if !ok {
		return 0, false
	}
	return (float64(u.PromptTokens)*pr.in + float64(u.CompletionTokens)*pr.out) / 1e6, true
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
