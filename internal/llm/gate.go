package llm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/tokens"
)

// NetworkDisabledMessage is the text every refused call carries.
const NetworkDisabledMessage = "LLM access is disabled: set [llm] allow_network = true in config.toml (or AI_RULEZ_LLM_ALLOW_NETWORK=1) to let ai-rulez send prompts to the configured model"

// withGate refuses every call unless network use is allowed, and bounds each call
// with its timeout. It is the outermost layer.
func withGate(next Client, allow bool, timeout time.Duration) Client {
	return &gateClient{next: next, allow: allow, timeout: timeout}
}

type gateClient struct {
	next    Client
	allow   bool
	timeout time.Duration
}

func (g *gateClient) deadline(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if d <= 0 {
		d = g.timeout
	}
	if d <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, d)
}

func wrapTimeout(ctx context.Context, err error) error {
	if err != nil && ctx.Err() == context.DeadlineExceeded {
		var e *Error
		if !errors.As(err, &e) {
			return &Error{Kind: KindTimeout, Message: "call exceeded its timeout", Cause: err}
		}
	}
	return err
}

func (g *gateClient) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	if !g.allow {
		return ChatResponse{}, newError(KindNetworkDisabled, "%s", NetworkDisabledMessage)
	}
	if !Finite(req.Temperature) {
		return ChatResponse{}, newError(KindConfig, "temperature must be a finite number")
	}
	ctx, cancel := g.deadline(ctx, req.Timeout)
	defer cancel()
	resp, err := g.next.Chat(ctx, req)
	return resp, wrapTimeout(ctx, err)
}

func (g *gateClient) Embed(ctx context.Context, req EmbedRequest) (EmbedResponse, error) {
	if !g.allow {
		return EmbedResponse{}, newError(KindNetworkDisabled, "%s", NetworkDisabledMessage)
	}
	ctx, cancel := g.deadline(ctx, req.Timeout)
	defer cancel()
	resp, err := g.next.Embed(ctx, req)
	return resp, wrapTimeout(ctx, err)
}

func (g *gateClient) Close() error { return g.next.Close() }

// DryRun is a Client that calls nothing: it writes what would be sent (model,
// sizes, estimated tokens and cost) to Out and returns ErrDryRun. Prompt text is
// printed only when ShowContent is set.
type DryRun struct {
	Out         io.Writer
	Model       string
	EmbedModel  string
	Pricing     Pricing
	ShowContent bool
}

// Chat implements Client.
func (d *DryRun) Chat(_ context.Context, req ChatRequest) (ChatResponse, error) {
	if req.Model == "" {
		req.Model = d.Model
	}
	est := Usage{PromptTokens: EstimatePromptTokens(req), CompletionTokens: req.MaxTokens}
	cost, known := d.Pricing.Cost(req.Model, est)
	printf(d.Out, "dry-run chat: %s\n  estimated tokens: prompt=%d max_completion=%d, %s\n", req.Summary(), est.PromptTokens, est.CompletionTokens, costText(cost, known))
	if d.ShowContent {
		for _, m := range req.Messages {
			printf(d.Out, "  [%s] %s\n", m.Role, strings.ReplaceAll(m.Content, "\n", "\n    "))
		}
	}
	return ChatResponse{}, &Error{Kind: KindDryRun, Message: "dry run: nothing was sent"}
}

// Embed implements Client.
func (d *DryRun) Embed(_ context.Context, req EmbedRequest) (EmbedResponse, error) {
	if req.Model == "" {
		req.Model = d.EmbedModel
	}
	total := 0
	for _, s := range req.Input {
		total += tokens.Estimate(s)
	}
	cost, known := d.Pricing.Cost(req.Model, Usage{PromptTokens: total})
	printf(d.Out, "dry-run embed: %s\n  estimated tokens: %d, %s\n", req.Summary(), total, costText(cost, known))
	return EmbedResponse{}, &Error{Kind: KindDryRun, Message: "dry run: nothing was sent"}
}

// Close implements Client.
func (d *DryRun) Close() error { return nil }

func costText(cost float64, known bool) string {
	if !known {
		return "cost unknown (no price for this model)"
	}
	return fmt.Sprintf("estimated cost <= $%.6f", cost)
}
