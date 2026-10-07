package llm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
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
// could exceed a limit (fail closed). Each call first reserves its worst case
// (estimated prompt plus the completion cap) under one lock, so concurrent calls
// cannot jointly overshoot a limit: a call is admitted only when what has been
// spent plus what is reserved by calls in flight plus its own worst case fits.
// The reservation is settled with the actual usage when the call ends. An
// unknown price counts as unaffordable when a cost limit is set.
type Budget struct {
	limits  Limits
	pricing Pricing

	mu       sync.Mutex
	spent    Spent
	reserved Spent // worst case held by calls in flight (Calls is unused)
}

// NewBudget returns a Budget for limits.
func NewBudget(limits Limits, pricing Pricing) *Budget {
	return &Budget{limits: limits, pricing: pricing}
}

// Spent returns the totals charged so far (settled calls only).
func (b *Budget) Spent() Spent {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.spent
}

// reservation is the worst case held for one in-flight call.
type reservation struct {
	b     *Budget
	model string
	worst Usage
	cost  float64
}

// reserve atomically checks the limits against spent + reserved + the worst case
// of this call and, when it fits, holds that worst case and counts the call.
func (b *Budget) reserve(model string, worst Usage) (*reservation, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.limits.MaxCalls > 0 && b.spent.Calls+1 > b.limits.MaxCalls {
		return nil, newError(KindBudget, "max_calls %d reached (%d calls made); raise [llm] max_calls to continue", b.limits.MaxCalls, b.spent.Calls)
	}
	if b.limits.MaxTokens > 0 && b.spent.Tokens+b.reserved.Tokens+worst.Total() > b.limits.MaxTokens {
		return nil, newError(KindBudget, "max_tokens %d would be exceeded (%d used, %d held by calls in flight, this call may use up to %d)",
			b.limits.MaxTokens, b.spent.Tokens, b.reserved.Tokens, worst.Total())
	}
	cost, known := b.pricing.Cost(model, worst)
	if b.limits.MaxCostUSD > 0 {
		if !known {
			return nil, newError(KindBudget, "max_cost_usd is set but no price is known for model %q; set price_input_per_mtok and price_output_per_mtok", model)
		}
		if b.spent.CostUSD+b.reserved.CostUSD+cost > b.limits.MaxCostUSD {
			return nil, newError(KindBudget, "max_cost_usd %.4f would be exceeded (%.4f spent, %.4f held by calls in flight, this call may cost up to %.4f)",
				b.limits.MaxCostUSD, b.spent.CostUSD, b.reserved.CostUSD, cost)
		}
	}
	b.spent.Calls++
	b.reserved.Tokens += worst.Total()
	b.reserved.CostUSD += cost
	return &reservation{b: b, model: model, worst: worst, cost: cost}, nil
}

// settle replaces the reservation with the actual charge. usage is what the
// call is charged; respModel is the model name the provider reported, which is
// never trusted to lower the price: the call is charged at the dearer of the
// requested model's price and the reported model's price.
func (r *reservation) settle(respModel string, usage Usage) (costUSD float64, known bool) {
	cost, known := r.b.pricing.Cost(r.model, usage)
	if respModel != "" && respModel != r.model {
		if alt, altKnown := r.b.pricing.Cost(respModel, usage); altKnown {
			known = true
			cost = max(cost, alt)
		}
	}
	r.release(usage.Total(), cost)
	return cost, known
}

// fail settles a call that returned an error. A clean rejection (an HTTP error
// status, or an error raised before anything was sent) is not billed, so only
// the call count stays. Anything else (a timeout, a transport error after the
// request may have been sent, an undecodable reply, a canceled context) may
// have been billed or may still be running, so the worst case is charged.
func (r *reservation) fail(err error) {
	if unbilled(err) {
		r.release(0, 0)
		return
	}
	r.release(r.worst.Total(), r.cost)
}

func (r *reservation) release(tokens int, cost float64) {
	r.b.mu.Lock()
	defer r.b.mu.Unlock()
	r.b.reserved.Tokens -= r.worst.Total()
	r.b.reserved.CostUSD -= r.cost
	r.b.spent.Tokens += tokens
	r.b.spent.CostUSD += cost
}

// unbilled reports whether err is a rejection the provider did not bill.
func unbilled(err error) bool {
	var e *Error
	if !errors.As(err, &e) {
		return false
	}
	// Only a documented client-error rejection is known to be free. A 5xx, 429
	// or gateway timeout may have run (and been billed) upstream, so those are
	// charged at the worst case like any other unknown outcome.
	switch e.Status {
	case 0:
	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusUnprocessableEntity:
		return true
	default:
		return false
	}
	switch e.Kind {
	case KindAuth, KindConfig, KindContextLength, KindBudget, KindNetworkDisabled, KindDryRun:
		return true
	default:
		return false
	}
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
	res, err := c.b.reserve(model, worst)
	if err != nil {
		return ChatResponse{}, err
	}
	resp, err := c.next.Chat(ctx, req)
	if err != nil {
		res.fail(err)
		return resp, err
	}
	usage := chargeable(resp.Usage, worst)
	resp.Usage = usage
	resp.CostUSD, resp.CostKnown = res.settle(resp.Model, usage)
	return resp, nil
}

func (c *budgetClient) Embed(ctx context.Context, req EmbedRequest) (EmbedResponse, error) {
	model := firstNonEmpty(req.Model, c.embedModel)
	tokens := 0
	for _, s := range req.Input {
		tokens += EstimateTokens(s)
	}
	worst := Usage{PromptTokens: tokens}
	res, err := c.b.reserve(model, worst)
	if err != nil {
		return EmbedResponse{}, err
	}
	resp, err := c.next.Embed(ctx, req)
	if err != nil {
		res.fail(err)
		return resp, err
	}
	usage := chargeable(resp.Usage, worst)
	resp.Usage = usage
	if resp.Requests > 1 {
		c.b.addCalls(resp.Requests - 1)
	}
	resp.CostUSD, resp.CostKnown = res.settle(resp.Model, usage)
	return resp, nil
}

// addCalls counts requests a call made beyond the one reserve counted.
func (b *Budget) addCalls(n int) {
	b.mu.Lock()
	b.spent.Calls += n
	b.mu.Unlock()
}

// chargeable is the usage a call is charged. The provider's figure is trusted
// only when it is plausible: a missing report (zero) or a negative component
// could lower the spent total, so either one fails closed to the worst case.
func chargeable(reported, worst Usage) Usage {
	if reported.PromptTokens < 0 || reported.CompletionTokens < 0 || reported.Total() <= 0 {
		return worst
	}
	return reported
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
