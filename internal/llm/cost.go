package llm

import (
	"strings"
	"unicode/utf8"
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

// EstimateTokens approximates the token count of text (about four bytes per
// token for English and code). It is deliberately conservative for budgets.
func EstimateTokens(text string) int {
	if text == "" {
		return 0
	}
	n := utf8.RuneCountInString(text)
	return (n + 3) / 4
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
