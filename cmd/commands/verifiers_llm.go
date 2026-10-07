package commands

import (
	"context"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
	"github.com/Goldziher/ai-rulez/v5/internal/verifiers"
)

// defaultVerifiersMaxCost is the default --max-cost of an llm verifier run, in USD.
const defaultVerifiersMaxCost = 0.50

var (
	verifiersAllowLLM bool
	verifiersMaxCost  float64
	verifiersEstimate bool
	verifiersGateLLM  bool
)

// verifierLLMOptions builds the model access of a run. Model use needs
// --allow-llm and, from the user config or environment (never the repository),
// allow_network = true and a model; without all of that the llm verifiers are
// skipped with the reason, and nothing is sent. --estimate needs neither: it
// prints the egress manifest and the cost bound and calls nothing. The returned
// func releases the client.
func verifierLLMOptions(ctx context.Context, cfg *config.Config) (*verifiers.LLMOptions, func(), error) {
	noop := func() {}
	if verifiersMaxCost < 0 {
		return nil, noop, oops.Errorf("--max-cost must not be negative")
	}
	resolved, err := cfg.ResolveLLM(nil)
	if err != nil {
		return nil, noop, err //nolint:wrapcheck // already contextual
	}
	lc := resolved.Config
	pricing := llm.NewPricing(lc)
	opts := &verifiers.LLMOptions{
		Model: lc.FullModel(), MaxCostUSD: verifiersMaxCost, Estimate: verifiersEstimate, Gate: verifiersGateLLM,
		Prices: pricing.Cost,
	}
	switch {
	case verifiersEstimate:
		return opts, noop, nil
	case !verifiersAllowLLM:
		opts.Disabled = "pass --allow-llm to evaluate llm verifiers"
		return opts, noop, nil
	case !lc.AllowNetwork:
		opts.Disabled = "[llm] allow_network is not enabled in the user config (a repository config cannot enable it)"
		return opts, noop, nil
	case lc.FullModel() == "":
		opts.Disabled = "no [llm] model is configured"
		return opts, noop, nil
	}
	if maxCost := verifiersMaxCost; maxCost > 0 && (lc.MaxCostUSD == 0 || maxCost < lc.MaxCostUSD) {
		lc.MaxCostUSD = maxCost // the budget guard enforces it too, fail closed
	}
	managed, err := llm.New(lc, llm.Options{ConfigDir: cfg.ConfigDir})
	if err != nil {
		return nil, noop, err //nolint:wrapcheck // already contextual
	}
	opts.Client = managed
	return opts, func() {
		_ = managed.Close() //nolint:errcheck // releasing the client at exit; nothing can act on a close error
	}, nil
}
