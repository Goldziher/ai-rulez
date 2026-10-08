package llm

import (
	"fmt"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/pricing"
	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
)

// PriceTableVersion identifies the built-in price table (internal/pricing, shared
// with the eval estimate). It is part of the cache identity, so a cost recorded
// under old prices is never replayed under new ones.
const PriceTableVersion = pricing.Version

// Pricing resolves prices for a model.
type Pricing struct {
	input, output float64
	override      bool
	// overrideModel is the bare model name the override is for; empty means every model.
	overrideModel string
}

// NewPricing returns a Pricing that uses the config override for the model it
// was written for: the user-scope model after Resolve or WithEnv, else Model.
// Any other model falls back to the built-in table.
func NewPricing(cfg Config) Pricing {
	if cfg.PriceInputPerMTok > 0 || cfg.PriceOutputPerMTok > 0 {
		target := cfg.Model
		if cfg.priceBound {
			target = cfg.priceModel
		}
		p := Pricing{input: cfg.PriceInputPerMTok, output: cfg.PriceOutputPerMTok, override: true, overrideModel: bareModel(target)}
		if p.overrideModel == "" && cfg.priceBound {
			p.override = false // the user named no model, so the override has nothing to price
		}
		return p
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

// bareModel lowercases a model name and drops any provider prefix.
func bareModel(model string) string {
	name := strings.ToLower(strings.TrimSpace(model))
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	return name
}

func (p Pricing) lookup(model string) (pricing.Price, bool) {
	if p.override && (p.overrideModel == "" || p.overrideModel == bareModel(model)) {
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

// EstimatePromptTokens approximates the prompt size of a chat request,
// including a small per-message framing overhead.
func EstimatePromptTokens(req ChatRequest) int {
	total := 3
	for _, m := range req.Messages {
		total += tokens.Estimate(m.Content) + 4
	}
	if req.ResponseFormat != nil {
		total += tokens.Estimate(req.ResponseFormat.Name) + tokens.Estimate(mustJSON(req.ResponseFormat.Schema)) + 16
	}
	return total
}
