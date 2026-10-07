package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/evals"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

// validateActivationFlags checks the --mode, --surface and --scope flags before
// anything runs. A runner that cannot do activation is refused by the runner
// handshake before any prompt is sent, never run as a full case run.
func validateActivationFlags() error {
	switch evalFlags.mode {
	case evals.ModeCases, evals.ModeActivation:
	default:
		return oops.Errorf("unknown --mode %q (use %s or %s)", evalFlags.mode, evals.ModeCases, evals.ModeActivation)
	}
	switch evalFlags.maxCostMode {
	case "", evals.CostModeExpected, evals.CostModeHigh:
	default:
		return oops.Errorf("unknown --max-cost-mode %q (use %s or %s)", evalFlags.maxCostMode, evals.CostModeExpected, evals.CostModeHigh)
	}
	if evalFlags.mode != evals.ModeActivation {
		if evalFlags.descriptionFrom != "" {
			return oops.Errorf("--description-from needs --mode activation")
		}
		if evalFlags.surface != "" {
			return oops.Errorf("--surface needs --mode activation")
		}
		if evalFlags.scope != evals.ScopeDomain {
			return oops.Errorf("--scope needs --mode activation")
		}
		return nil
	}
	if evalFlags.scope != evals.ScopeDomain && evalFlags.scope != evals.ScopeAll {
		return oops.Errorf("unknown --scope %q (use %s or %s)", evalFlags.scope, evals.ScopeDomain, evals.ScopeAll)
	}
	if evalFlags.format == evals.FormatJUnit {
		return oops.Errorf("--mode activation writes json or markdown, not junit")
	}
	switch evalFlags.surface {
	case evals.SurfaceRetrieval, evals.SurfaceNative:
		return nil
	case "":
		return oops.Hint("--surface retrieval ranks the prompts offline with the find_skill ranker, for free; --surface native asks the harness's model").
			Errorf("--mode activation needs --surface (retrieval or native)")
	}
	return oops.Errorf("unknown --surface %q (use %s or %s)", evalFlags.surface, evals.SurfaceRetrieval, evals.SurfaceNative)
}

// buildActivationRunner selects the runner of the native surface: the one the
// flags name, else the built-in adapter of the harness. The runner is not asked
// what it supports here; the engine does that (the handshake) before any request.
func buildActivationRunner(cmd *cobra.Command) (evals.Runner, string, error) {
	harness := evalFlags.harness
	name := evalFlags.runner
	if name == "" {
		switch {
		case evalFlags.runnerCommand != "":
			name = evals.RunnerCommand
		case harness == evalHarnessClaude:
			name = evals.RunnerClaudeNative
		case harness == "codex":
			name = evals.RunnerCodexNative
		default:
			return nil, harness, oops.Hint("Use --runner-command to plug in a runner for the "+harness+" harness").
				Errorf("no built-in native activation runner for the %q harness", harness)
		}
	}
	if name == evals.RunnerCodexNative && !cmd.Flags().Changed("harness") {
		harness = "codex"
	}
	switch name {
	case evals.RunnerClaudeNative:
		return &evals.ClaudeNative{Bin: evalFlags.claudeBin, ExtraArgs: evalFlags.runnerArgs, Stderr: cmd.ErrOrStderr(), SkillGate: skillSecurityGate}, harness, nil
	case evals.RunnerCodexNative:
		return &evals.CodexNative{Bin: evalFlags.codexBin, ExtraArgs: evalFlags.runnerArgs, Stderr: cmd.ErrOrStderr()}, harness, nil
	case evals.RunnerClaudePluginEval:
		return &evals.ClaudePluginEval{Bin: evalFlags.claudeBin, ExtraArgs: evalFlags.runnerArgs, Stderr: cmd.ErrOrStderr()}, harness, nil
	case evals.RunnerCommand:
		if evalFlags.runnerCommand == "" {
			return nil, harness, oops.Errorf("--runner command needs --runner-command")
		}
		return &evals.CommandRunner{Command: evalFlags.runnerCommand, Timeout: evalFlags.timeout, Stderr: cmd.ErrOrStderr()}, harness, nil
	}
	return nil, harness, unknownEvalRunner(name)
}

// estimateParams reads the [lint.evals.estimate] assumptions.
func estimateParams(cfg *config.Config) evals.EstimateParams {
	if cfg == nil || cfg.Lint == nil || cfg.Lint.Evals == nil || cfg.Lint.Evals.Estimate == nil {
		return evals.EstimateParams{}
	}
	e := cfg.Lint.Evals.Estimate
	return evals.EstimateParams{OverheadTokens: e.OverheadTokens, AssumedOutputTokens: e.AssumedOutputTokens,
		ActivationOutputTokens: e.ActivationOutputTokens, ToolLoopFactor: e.ToolLoopFactor}
}

// evalPrice resolves the price the estimate uses: --price-in and --price-out, else
// [lint.evals.estimate] price_in_per_mtok and price_out_per_mtok, and for a side
// left unset the price table's price for the model. No price at all is the zero
// Price, which keeps the table.
func evalPrice(cfg *config.Config, model string) evals.Price {
	in, out := evalFlags.priceIn, evalFlags.priceOut
	if cfg != nil && cfg.Lint != nil && cfg.Lint.Evals != nil && cfg.Lint.Evals.Estimate != nil {
		e := cfg.Lint.Evals.Estimate
		if in == 0 {
			in = e.PriceInPerMTok
		}
		if out == 0 {
			out = e.PriceOutPerMTok
		}
	}
	if in == 0 && out == 0 {
		return evals.Price{}
	}
	table, _ := evals.PriceFor(model)
	if in == 0 {
		in = table.InPerMTok
	}
	if out == 0 {
		out = table.OutPerMTok
	}
	return evals.Price{InPerMTok: in, OutPerMTok: out}
}

// runEvalActivation runs `eval run --mode activation`, on the retrieval surface or
// the native one.
func runEvalActivation(ctx context.Context, cmd *cobra.Command, skills []string, cfg *config.Config) (failed bool, err error) {
	opts, err := buildActivationOptions(cmd, skills, cfg)
	if err != nil {
		return false, err
	}
	path := resultsPath(cfg.ConfigDir)
	save := !evalDryRun() && !evalFlags.noWrite
	store, err := attachActivationStore(opts, path, save)
	if err != nil {
		return false, err
	}
	report, runErr := evals.RunActivation(ctx, opts)
	if report == nil {
		return false, runErr
	}
	// Spend is recorded even when a later skill failed or the run was interrupted.
	if save && store != nil && (runErr == nil || evalFlags.surface == evals.SurfaceNative) {
		if err := store.Save(path); err != nil {
			return false, err
		}
	}
	return report.Failed, errors.Join(writeActivationReport(cmd, report), runErr)
}

// buildActivationOptions turns the flags into the options of an activation run.
func buildActivationOptions(cmd *cobra.Command, skills []string, cfg *config.Config) (*evals.ActivationOptions, error) {
	cfgDir, baseDir := cfg.ConfigDir, cfg.BaseDir
	absDir, err := filepath.Abs(cfgDir)
	if err != nil {
		return nil, oops.Wrapf(err, "resolve config directory")
	}
	date := evalFlags.date
	if date == "" {
		date = os.Getenv(EvalDateEnv)
	}
	opts := &evals.ActivationOptions{ConfigDir: absDir, Skills: skills, Scope: evalFlags.scope, Date: date, Surface: evalFlags.surface}
	if evalFlags.descriptionFrom != "" {
		if len(skills) != 1 {
			return nil, oops.Hint("Name the skill: ai-rulez eval run <skill> --mode activation --description-from FILE").
				Errorf("--description-from measures a candidate description of exactly one skill, got %d", len(skills))
		}
		if opts.Description, err = readDescriptionFile(evalFlags.descriptionFrom); err != nil {
			return nil, err
		}
		opts.DescriptionSkill = skills[0]
	}
	if thresholdGiven(cmd) {
		threshold := evalFlags.threshold
		opts.PassThreshold = &threshold
	}
	if opts.Changed, err = changedEvalSkills(absDir, baseDir); err != nil {
		return nil, err
	}
	if evalFlags.surface == evals.SurfaceNative {
		if err := configureNative(cmd, opts, cfg); err != nil {
			return nil, err
		}
	}
	return opts, nil
}

// attachActivationStore loads the results store: for writing when a run will save, read-only
// for a native dry run (the estimate leaves out what a real run would replay).
func attachActivationStore(opts *evals.ActivationOptions, path string, save bool) (store *evals.Store, err error) {
	switch {
	case save:
		if store, err = evals.LoadStoreKeyed(path, evals.UserKey()); err != nil {
			return nil, err
		}
		opts.Store = store
	case evalFlags.surface == evals.SurfaceNative:
		if opts.Store, err = evals.LoadStoreKeyed(path, evals.ExistingUserKey()); err != nil {
			return nil, err
		}
	}
	return store, nil
}

// readDescriptionFile reads a candidate skill description (--description-from):
// bounded, valid UTF-8, no hidden characters (a description is sent to a model).
func readDescriptionFile(path string) (string, error) {
	const limit = 16 << 10
	data, _, err := safefs.ReadRegularKeepMode(path) // the user's file: its mode is not ours to change
	if err != nil {
		return "", oops.With("path", path).Wrapf(err, "read the description file")
	}
	if len(data) > limit {
		return "", oops.With("path", path).Errorf("the description file is over the %d byte limit", limit)
	}
	if !utf8.Valid(data) {
		return "", oops.With("path", path).Errorf("the description file is not valid UTF-8")
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return "", oops.With("path", path).Errorf("the description file is empty")
	}
	if found, hidden := lint.DetectHidden(text); hidden {
		return "", oops.With("path", path).Errorf("the description file holds a hidden character (%s)", found)
	}
	return text, nil
}

// configureNative fills the settings of the native surface.
func configureNative(cmd *cobra.Command, opts *evals.ActivationOptions, cfg *config.Config) error {
	opts.Harness, opts.Model, opts.Force = evalFlags.harness, evalFlags.model, evalFlags.force
	opts.Runs, opts.DryRun, opts.MaxCostUSD = evalFlags.runs, evalDryRun(), evalFlags.maxCost
	opts.MaxCostMode = evalFlags.maxCostMode
	if opts.MaxCostMode == "" {
		opts.MaxCostMode = evals.CostModeHigh // activation is cheap, so the cap holds the worst case
	}
	opts.Price = evalPrice(cfg, evalFlags.model)
	opts.Params, opts.ToolVersion, opts.Timeout = estimateParams(cfg), Version, evalFlags.timeout
	runner, harness, err := buildActivationRunner(cmd)
	if err != nil {
		if !evalDryRun() {
			return err
		}
		return nil // a dry run needs no runner
	}
	opts.Runner, opts.Harness = runner, harness
	return nil
}

func writeActivationReport(cmd *cobra.Command, report *evals.ActivationReport) error {
	if evalFlags.out == "" {
		return oops.Wrapf(report.Write(cmd.OutOrStdout(), evalFlags.format), "write report")
	}
	if err := os.MkdirAll(evalFlags.out, 0o750); err != nil {
		return oops.Wrapf(err, "create output directory")
	}
	path := filepath.Join(evalFlags.out, "eval-activation."+evals.Extension(evalFlags.format))
	file, err := os.Create(path) //nolint:gosec // user-chosen output directory
	if err != nil {
		return oops.Wrapf(err, "create report")
	}
	if err := report.Write(file, evalFlags.format); err != nil {
		_ = file.Close() //nolint:errcheck // the write error is the one to report
		return oops.Wrapf(err, "write report")
	}
	if err := file.Close(); err != nil {
		return oops.Wrapf(err, "close report")
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Wrote %s (%d skills)\n", path, len(report.Skills))
	return oops.Wrapf(err, "write summary")
}

// skillSecurityGate refuses a skill whose SKILL.md has an error-level security
// finding before a native activation run copies it into a harness plugin and
// sends it to a model.
func skillSecurityGate(id, text string) error {
	for _, f := range lint.ScanText(id+"/SKILL.md", text) {
		if f.Severity == lint.SeverityError {
			return oops.Errorf("%s: %s (line %d)", f.Code, f.Message, f.Line)
		}
	}
	return nil
}
