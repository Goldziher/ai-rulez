package llm

import (
	"context"
	"fmt"
	"sync"
)

// Limits are the spend caps of one run. Zero means unlimited for that axis.
type Limits struct {
	MaxCostUSD float64
	MaxTokens  int
	MaxCalls   int
}

// Active reports whether any limit is set.
func (l Limits) Active() bool { return l.MaxCostUSD > 0 || l.MaxTokens > 0 || l.MaxCalls > 0 }

// Spent is the running total of a Budget.
type Spent struct {
	CostUSD float64
	Tokens  int
	Calls   int
}

// Budget tracks spend across calls and refuses, before calling, any request that
// could exceed a limit (fail closed): the check uses the estimated prompt plus
// the worst-case completion, and an unknown price counts as unaffordable when a
// cost limit is set.
type Budget struct {
	limits  Limits
	pricing Pricing

	mu    sync.Mutex
	spent Spent
}

// NewBudget returns a Budget for limits.
func NewBudget(limits Limits, pricing Pricing) *Budget {
	return &Budget{limits: limits, pricing: pricing}
}

// Spent returns the totals so far.
func (b *Budget) Spent() Spent {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.spent
}

// reserve checks the limits for a call with the given worst-case usage and, when
// allowed, counts the call. It returns the worst-case cost it assumed.
func (b *Budget) reserve(model string, worst Usage) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.limits.MaxCalls > 0 && b.spent.Calls+1 > b.limits.MaxCalls {
		return newError(KindBudget, "max_calls %d reached (%d calls made); raise [llm] max_calls to continue", b.limits.MaxCalls, b.spent.Calls)
	}
	if b.limits.MaxTokens > 0 && b.spent.Tokens+worst.Total() > b.limits.MaxTokens {
		return newError(KindBudget, "max_tokens %d would be exceeded (%d used, this call may use up to %d)", b.limits.MaxTokens, b.spent.Tokens, worst.Total())
	}
	if b.limits.MaxCostUSD > 0 {
		cost, known := b.pricing.Cost(model, worst)
		if !known {
			return newError(KindBudget, "max_cost_usd is set but no price is known for model %q; set price_input_per_mtok and price_output_per_mtok", model)
		}
		if b.spent.CostUSD+cost > b.limits.MaxCostUSD {
			return newError(KindBudget, "max_cost_usd %.4f would be exceeded (%.4f spent, this call may cost up to %.4f)", b.limits.MaxCostUSD, b.spent.CostUSD, cost)
		}
	}
	b.spent.Calls++
	return nil
}

func (b *Budget) charge(model string, u Usage) {
	cost, _ := b.pricing.Cost(model, u)
	b.mu.Lock()
	b.spent.Tokens += u.Total()
	b.spent.CostUSD += cost
	b.mu.Unlock()
}

// WithBudget wraps c with the limits; it returns c unchanged when no limit is set.
func WithBudget(c Client, b *Budget, defaultModel, defaultEmbedModel string) Client {
	if !b.limits.Active() {
		return c
	}
	return &budgetClient{next: c, b: b, model: defaultModel, embedModel: defaultEmbedModel}
}

type budgetClient struct {
	next              Client
	b                 *Budget
	model, embedModel string
}

func (c *budgetClient) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	if req.MaxTokens <= 0 {
		req.MaxTokens = DefaultCompletionCap
	}
	model := firstNonEmpty(req.Model, c.model)
	worst := Usage{PromptTokens: EstimatePromptTokens(req), CompletionTokens: req.MaxTokens}
	if err := c.b.reserve(model, worst); err != nil {
		return ChatResponse{}, err
	}
	resp, err := c.next.Chat(ctx, req)
	if err != nil {
		// The provider may have billed a failed call we cannot see; keep the reservation's call count only.
		return resp, err
	}
	usage := resp.Usage
	if usage.Total() == 0 {
		usage = worst // no usage reported: assume the worst, fail closed
	}
	c.b.charge(firstNonEmpty(resp.Model, model), usage)
	return resp, nil
}

func (c *budgetClient) Embed(ctx context.Context, req EmbedRequest) (EmbedResponse, error) {
	model := firstNonEmpty(req.Model, c.embedModel)
	tokens := 0
	for _, s := range req.Input {
		tokens += EstimateTokens(s)
	}
	worst := Usage{PromptTokens: tokens}
	if err := c.b.reserve(model, worst); err != nil {
		return EmbedResponse{}, err
	}
	resp, err := c.next.Embed(ctx, req)
	if err != nil {
		return resp, err
	}
	usage := resp.Usage
	if usage.Total() == 0 {
		usage = worst
	}
	c.b.charge(firstNonEmpty(resp.Model, model), usage)
	return resp, nil
}

func (c *budgetClient) Close() error { return c.next.Close() }

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// Describe renders the limits for doctor output.
func (l Limits) Describe() string {
	if !l.Active() {
		return "none"
	}
	return fmt.Sprintf("max_cost_usd=%g max_tokens=%d max_calls=%d (0 = unlimited)", l.MaxCostUSD, l.MaxTokens, l.MaxCalls)
}
