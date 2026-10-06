package commands

import (
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/evals"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

// defaultGraderMaxCost is the default --grader-max-cost, in USD.
const defaultGraderMaxCost = 0.25

// validateGraderFlags checks --grader and its companions before anything runs.
func validateGraderFlags() error {
	switch evalFlags.grader {
	case "", evals.GraderRunner:
		return nil
	case evals.GraderBuiltin:
	default:
		return oops.Errorf("unknown --grader %q (use %s or %s)", evalFlags.grader, evals.GraderRunner, evals.GraderBuiltin)
	}
	if evalFlags.mode == evals.ModeActivation {
		return oops.Errorf("--grader needs --mode cases: an activation run grades no rubric")
	}
	if evalFlags.graderMaxCost < 0 {
		return oops.Errorf("--grader-max-cost must be >= 0, got %v", evalFlags.graderMaxCost)
	}
	return nil
}

// buildGrader returns the built-in rubric grader for --grader builtin, or nil. It
// refuses, before any case runs, unless model use is switched on in every place
// that has to agree: --allow-llm, and from the user config or environment (never
// the repository) allow_network = true and a model. A dry run builds nothing and
// sends nothing. The returned func releases the client.
func buildGrader(cmd *cobra.Command, cfg *config.Config) (evals.RubricGrader, func(), error) {
	noop := func() {}
	if evalFlags.grader != evals.GraderBuiltin || evalDryRun() {
		return nil, noop, nil
	}
	if !evalFlags.allowLLM {
		return nil, noop, oops.Hint("--grader builtin sends the runner's transcripts to the configured [llm] model; pass --allow-llm to agree").
			Errorf("--grader builtin needs --allow-llm")
	}
	resolved, err := cfg.ResolveLLM(nil)
	if err != nil {
		return nil, noop, oops.Wrapf(err, "resolve [llm] configuration")
	}
	lc := resolved.Config
	switch {
	case !lc.AllowNetwork:
		return nil, noop, oops.Hint("Set allow_network = true in the user config ($XDG_CONFIG_HOME/ai-rulez/config.toml) or AI_RULEZ_LLM_ALLOW_NETWORK=1; a repository config cannot enable it, see docs/llm.md").
			Errorf("--grader builtin: [llm] allow_network is not enabled")
	case lc.FullModel() == "":
		return nil, noop, oops.Hint("Set [llm] model (and provider) in the user config, see docs/llm.md").Errorf("--grader builtin: no [llm] model is configured")
	}
	if cap := evalFlags.graderMaxCost; cap > 0 && (lc.MaxCostUSD == 0 || cap < lc.MaxCostUSD) {
		lc.MaxCostUSD = cap // the budget guard enforces it, failing closed
	}
	if lc.MaxCostUSD > 0 {
		// The budget guard refuses a model it cannot price, per call, after the runner has been
		// paid for: say so now instead.
		if _, known := llm.NewPricing(lc).Cost(lc.FullModel(), llm.Usage{PromptTokens: 1, CompletionTokens: 1}); !known {
			return nil, noop, oops.Hint("Set price_input_per_mtok and price_output_per_mtok under [llm] in the user config, or pass --grader-max-cost 0 to drop the cap").
				Errorf("--grader builtin: no price is known for model %q, so the spend cap cannot be enforced", lc.FullModel())
		}
	}
	managed, err := llm.New(lc, llm.Options{ConfigDir: cfg.ConfigDir})
	if err != nil {
		return nil, noop, oops.Wrapf(err, "start the grader's model client")
	}
	grader := &evals.JudgeGrader{Client: managed, Model: lc.FullModel(), Spent: func() float64 { return managed.Spent().CostUSD }}
	cmd.PrintErrf("grading rubrics with %s (the runner's transcripts are sent to it; spend capped at $%.2f)\n", lc.FullModel(), lc.MaxCostUSD)
	return grader, func() { _ = managed.Close() }, nil //nolint:errcheck // releasing a client
}
