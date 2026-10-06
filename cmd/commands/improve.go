package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/evals"
	"github.com/Goldziher/ai-rulez/v5/internal/improve"
	"github.com/Goldziher/ai-rulez/v5/internal/improve/adapter"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
	"github.com/Goldziher/ai-rulez/v5/internal/sandbox"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

// exitImproveNoCandidate is the exit status of a run that finished without an
// acceptable candidate (the report is written).
const exitImproveNoCandidate = 2

// improveExperimental is printed once per invocation.
const improveExperimental = "warning: `ai-rulez improve` is experimental: flags, the optimizer protocol and report.json may change before it is stable (docs/improve.md)"

var improveFlags struct {
	with              string
	holdoutTag        string
	holdoutFraction   float64
	minGain           float64
	maxRegressions    int
	maxRounds         int
	maxHoldoutEvals   int
	maxCost           float64
	runs              int
	timeout           time.Duration
	evalTimeout       time.Duration
	harness           string
	model             string
	runnerCommand     string
	claudeBin         string
	judgeModel        string
	runnerArgs        []string
	allowFrontmatter  bool
	allowScripts      bool
	envPass           []string
	egress            []string
	yes               bool
	dryRun            bool
	stopAtFirstAccept bool
	allowExec         bool
	format            string
	date              string
	priceIn           float64
	priceOut          float64

	adapter            string
	trustRepoOptimizer bool
	requireCIAboveZero bool
	isolation          string
	adapterModel       string
	adapterJudgeModel  string
	allowSameModel     bool
}

// ImproveCmd groups the experimental skill improvement commands.
var ImproveCmd = &cobra.Command{
	Use:   "improve",
	Short: "(experimental) Improve skills with an external optimizer behind a held-out eval gate",
}

var improveRunCmd = &cobra.Command{
	Use:   "run <skill>",
	Short: "(experimental) Run an external optimizer on a copy of a skill and gate its candidate",
	Long: `EXPERIMENTAL. Run an external optimizer (--with) on a throwaway copy of an authored skill and
accept its candidate only if a held-out eval set improves without regressions.

The optimizer speaks a small JSON protocol on standard input and output (docs/improve.md). It gets
the train cases only; cases tagged "holdout" (or a deterministic --holdout-fraction split when none
is tagged) never leave ai-rulez. A candidate is accepted when held-out pass rate gains at least
--min-gain with at most --max-regressions held-out cases flipping to fail, trigger precision and
recall do not drop, near-miss false positives do not rise, and the diff policy and security scan
hold (no new tools, scripts, executable bits or larger skill). The run lives under
.ai-rulez/local/improve/<run-id>/ with report.json (improve-report/1) and diff.patch; the authored
skill is untouched until "ai-rulez improve apply <run-id>", which never commits.

--max-cost is required. The optimizer runs without a shell, in a scrubbed environment (plus the
names in --env-pass; credential-like names need --egress), under --timeout. ai-rulez cannot sandbox
its file system or network access: run it in a container or CI job for anything beyond local
experiments. Exit status: 0 candidate accepted, 2 no acceptable candidate, 1 refused or failed.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		noCandidate, err := runImprove(cmd, args[0])
		if err != nil {
			return err
		}
		if noCandidate {
			os.Exit(exitImproveNoCandidate)
		}
		return nil
	},
}

var improveApplyCmd = &cobra.Command{
	Use:   "apply <run-id>",
	Short: "(experimental) Write an accepted improve candidate into the skill (no commit)",
	Long: `Show the diff of an accepted run and write it into the authored skill after confirmation (--yes
skips the prompt). Refuses when the skill changed since the run (AR9J1), when the saved candidate
no longer matches its digest, when the run was not recorded by this machine's user (the report
is signed with the per-user key outside the repository), or when it now breaks the default diff
policy: edits to scripts/ and assets/ need --allow-scripts, frontmatter changes need
--allow-frontmatter. Nothing is committed.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, stop := signal.NotifyContext(commandContext(cmd), os.Interrupt)
		defer stop()
		fmt.Fprintln(cmd.ErrOrStderr(), improveExperimental)
		cfg, err := loadConfigForCommand(ctx, nil)
		if err != nil {
			return err
		}
		configDirAbs, err := filepath.Abs(cfg.ConfigDir)
		if err != nil {
			return oops.Wrapf(err, "resolve config directory")
		}
		if err := checkFormatFlag(improveFlags.format); err != nil {
			return err
		}
		asJSON := improveFlags.format == formatJSON
		out := cmd.OutOrStdout()
		if asJSON {
			out = cmd.ErrOrStderr() // stdout carries the result document only
		}
		res, err := improve.Apply(ctx, &improve.ApplyOptions{
			ConfigDir: configDirAbs, RunID: args[0], Yes: improveFlags.yes, Out: out, Confirm: confirmProceed,
			AllowScripts: improveFlags.allowScripts, AllowFrontmatter: improveFlags.allowFrontmatter,
		})
		if err != nil {
			return oops.Wrap(err)
		}
		if asJSON {
			return writeImproveJSON(cmd.OutOrStdout(), res)
		}
		return nil
	},
}

func init() {
	f := improveRunCmd.Flags()
	f.StringVar(&improveFlags.with, "with", "", "Optimizer command, run without a shell: words separated by spaces, or a JSON array of strings")
	f.StringVar(&improveFlags.holdoutTag, "holdout-tag", improve.DefaultHoldoutTag, "Eval cases carrying this tag are held out")
	f.Float64Var(&improveFlags.holdoutFraction, "holdout-fraction", improve.DefaultHoldoutFraction, "Deterministic held-out fraction when no case carries the tag (0 to 1)")
	f.Float64Var(&improveFlags.minGain, "min-gain", improve.DefaultMinGain, "Held-out pass-rate gain (0 to 1) a candidate needs")
	f.IntVar(&improveFlags.maxRegressions, "max-regressions", 0, "Held-out cases allowed to flip from pass to fail")
	f.IntVar(&improveFlags.maxRounds, "max-rounds", improve.DefaultMaxRounds, "Optimizer invocations")
	f.IntVar(&improveFlags.maxHoldoutEvals, "max-holdout-evals", improve.DefaultMaxHoldoutEvals, "Times the held-out set may be evaluated")
	f.Float64Var(&improveFlags.maxCost, "max-cost", 0, "Total spend ceiling in USD, required: measured eval cost plus the cost the optimizer reports")
	f.IntVar(&improveFlags.runs, "runs", improve.DefaultRuns, "Eval runs per case and arm; the majority outcome counts")
	f.DurationVar(&improveFlags.timeout, "timeout", improve.DefaultTimeout, "Time limit for one optimizer invocation; its whole process tree is killed when it ends")
	f.DurationVar(&improveFlags.evalTimeout, "eval-timeout", 30*time.Minute, "Time limit for one eval runner call")
	f.StringVar(&improveFlags.harness, "harness", "claude", "Harness the evals run against (recorded in the report)")
	f.StringVar(&improveFlags.model, "model", "", "Model the evals run with")
	f.StringVar(&improveFlags.runnerCommand, "runner-command", "", "Shell command for the command eval runner (default: claude-plugin-eval for the claude harness)")
	f.StringVar(&improveFlags.claudeBin, "claude-bin", "claude", "claude executable for the claude-plugin-eval runner")
	f.StringVar(&improveFlags.judgeModel, "judge-model", "", "Grader model for claude-plugin-eval")
	f.StringArrayVar(&improveFlags.runnerArgs, "runner-arg", nil, "Extra argument for the claude-plugin-eval runner; repeatable")
	f.BoolVar(&improveFlags.allowFrontmatter, "allow-frontmatter", false, "Let the optimizer change allowed-tools, model and disable-model-invocation (the name is always fixed)")
	f.BoolVar(&improveFlags.allowScripts, "allow-scripts", false, "Let the optimizer change scripts/ and assets/ and reference scripts")
	f.StringSliceVar(&improveFlags.envPass, "env-pass", nil, "Environment variable names forwarded to the optimizer (credential-like names need --egress)")
	f.StringSliceVar(&improveFlags.egress, "egress", nil, "Hosts the optimizer sends data to: printed in the consent summary and recorded; not enforced")
	f.BoolVarP(&improveFlags.yes, "yes", "y", false, "Skip the consent prompt (CI); on apply, skip the confirmation")
	f.BoolVar(&improveFlags.dryRun, "dry-run", false, "Print the plan, split, estimate and egress; run nothing and write nothing")
	f.BoolVar(&improveFlags.stopAtFirstAccept, "stop-at-first-accept", false, "Stop after the first accepted round instead of using every round")
	f.BoolVar(&improveFlags.allowExec, "allow-exec", false, "Run command_exit assertions of the cases (they execute commands from the case files)")
	addFormatFlag(f, &improveFlags.format, formatText, formatText, formatText, formatJSON)
	addJSONFlagAlias(f)
	f.StringVar(&improveFlags.date, "date", "", "Date recorded in the report (default $"+EvalDateEnv+"; the clock is never read)")
	f.Float64Var(&improveFlags.priceIn, "price-in", 0, "USD per million input tokens for the estimate (default by model tier)")
	f.Float64Var(&improveFlags.priceOut, "price-out", 0, "USD per million output tokens for the estimate (default by model tier)")
	f.StringVar(&improveFlags.adapter, "adapter", "", "Bundled optimizer adapter, the same as --with builtin:NAME (see improve adapters)")
	f.BoolVar(&improveFlags.trustRepoOptimizer, "trust-repo-optimizer", false, "Use [improve] optimizer and env_pass from a repository config (they choose a command that runs on your machine)")
	f.BoolVar(&improveFlags.requireCIAboveZero, "require-ci-above-zero", false, "Also require the 95% bootstrap interval of the held-out gain to exclude zero")
	f.StringVar(&improveFlags.isolation, "isolation", "", "Confine the optimizer: none (default), auto (when a sandbox backend works) or require (refuse without one)")
	f.StringVar(&improveFlags.adapterModel, "adapter-model", "", "builtin:review-fix: model that writes the fix (default [review.fix] model); must differ from the judge")
	f.StringVar(&improveFlags.adapterJudgeModel, "adapter-judge-model", "", "builtin:review-fix: model that judges and verifies (default [llm] model)")
	f.BoolVar(&improveFlags.allowSameModel, "allow-same-model", false, "builtin:review-fix: let the fixer and the judge be the same model (self-preference risk)")
	f.StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
	improveApplyCmd.Flags().BoolVarP(&improveFlags.yes, "yes", "y", false, "Write without the confirmation prompt")
	improveApplyCmd.Flags().BoolVar(&improveFlags.allowScripts, "allow-scripts", false, "Allow the candidate to change scripts/ and assets/ and reference scripts")
	improveApplyCmd.Flags().BoolVar(&improveFlags.allowFrontmatter, "allow-frontmatter", false, "Allow the candidate to change allowed-tools, model and disable-model-invocation")
	addFormatFlag(improveApplyCmd.Flags(), &improveFlags.format, formatText, formatText, formatText, formatJSON)
	addJSONFlagAlias(improveApplyCmd.Flags())
	ImproveCmd.AddCommand(improveRunCmd, improveApplyCmd)
}

func runImprove(cmd *cobra.Command, skill string) (noCandidate bool, err error) {
	ctx, stop := signal.NotifyContext(commandContext(cmd), os.Interrupt)
	defer stop()
	errOut := cmd.ErrOrStderr()
	fmt.Fprintln(errOut, improveExperimental)
	if err := checkFormatFlag(improveFlags.format); err != nil {
		return false, err
	}
	cfg, err := loadConfigForCommand(ctx, nil)
	if err != nil {
		return false, err
	}
	st, err := resolveImproveSettings(cmd, cfg, errOut)
	if err != nil {
		return false, err
	}
	choice, err := resolveOptimizer(cmd, cfg, st.optimizer)
	if err != nil {
		if len(st.ignored) > 0 && st.optimizer == "" {
			return false, oops.Hint("The repository config sets [improve] optimizer; pass --trust-repo-optimizer to use it, or --with").Wrap(err)
		}
		return false, err
	}
	st.adapter, st.envPass = choice.adapter, append(st.envPass, choice.envPass...)
	if len(st.egress) == 0 {
		st.egress = choice.egress // a bundled adapter's own host, shown in the consent summary
	}
	configDirAbs, err := filepath.Abs(cfg.ConfigDir)
	if err != nil {
		return false, oops.Wrapf(err, "resolve config directory")
	}
	repo, err := filepath.Abs(cfg.BaseDir)
	if err != nil {
		return false, oops.Wrapf(err, "resolve project directory")
	}
	opts, err := buildImproveOptions(errOut, skill, choice.argv, configDirAbs, repo, st)
	if err != nil {
		return false, err
	}
	plan, err := improve.Prepare(ctx, opts)
	if err != nil {
		return false, oops.Wrap(err)
	}
	if improveFlags.dryRun {
		return false, printImprovePlan(cmd.OutOrStdout(), plan)
	}
	fmt.Fprint(errOut, plan.Summary())
	if !improveFlags.yes && !confirmProceed("Run the optimizer with this plan?") {
		return false, oops.Hint("Re-run with --yes to skip the prompt (CI)").Errorf("not confirmed: nothing was run")
	}
	report, err := plan.Execute(ctx)
	if err != nil {
		if report != nil { // stopped mid-run: the spend and the rounds so far are in the saved report
			if perr := printImproveReport(cmd.OutOrStdout(), report); perr != nil {
				return false, perr
			}
		}
		return false, oops.Wrap(err)
	}
	if err := printImproveReport(cmd.OutOrStdout(), report); err != nil {
		return false, err
	}
	return !report.Accepted(), nil
}

// improveSettings are the effective settings of one run: a flag wins over the user config, which wins
// over the repository config, which wins over the default.
type improveSettings struct {
	optimizer       string
	holdoutTag      string
	holdoutFraction float64
	minGain         float64
	maxRegressions  int
	maxRounds       int
	maxHoldoutEvals int
	minHoldoutCases int
	runs            int
	maxSkillGrowth  float64
	requireCI       bool
	isolation       sandbox.Mode
	envPass         []string
	egress          []string
	adapter         string
	// ignored are the repository keys that were not used (AR9J6).
	ignored []string
}

// resolveImproveSettings merges the flags with [improve] and validates the table.
func resolveImproveSettings(cmd *cobra.Command, cfg *config.Config, errOut io.Writer) (*improveSettings, error) {
	if problems := cfg.Improve.Validate(); len(problems) > 0 {
		return nil, oops.Hint("Fix the [improve] table in config.toml").Errorf("%s", strings.Join(problems, "; "))
	}
	res, err := cfg.ResolveImprove(improveFlags.trustRepoOptimizer, nil)
	if err != nil {
		return nil, oops.Wrap(err)
	}
	if len(res.IgnoredRepoKeys) > 0 {
		fmt.Fprintf(errOut, "warning: %s the repository config sets [improve] %s, which choose what runs on your machine and with which environment: ignored without --trust-repo-optimizer\n",
			improve.CodeRepoOptimizerIgnored, strings.Join(res.IgnoredRepoKeys, " and "))
	}
	if len(res.LoosenedRepoKeys) > 0 {
		fmt.Fprintf(errOut, "warning: %s the repository config sets [improve] %s looser than the defaults: ignored without --trust-repo-optimizer (a repository may tighten the gate, not weaken it)\n",
			improve.CodeRepoOptimizerIgnored, strings.Join(res.LoosenedRepoKeys, ", "))
	}
	e := &res.Effective
	changed := cmd.Flags().Changed
	st := &improveSettings{
		optimizer: strings.TrimSpace(e.Optimizer), holdoutTag: improveFlags.holdoutTag,
		holdoutFraction: improveFlags.holdoutFraction, minGain: improveFlags.minGain, maxRegressions: improveFlags.maxRegressions,
		maxRounds: improveFlags.maxRounds, maxHoldoutEvals: improveFlags.maxHoldoutEvals, runs: improveFlags.runs,
		minHoldoutCases: e.MinHoldoutCases, maxSkillGrowth: e.MaxSkillGrowth, requireCI: improveFlags.requireCIAboveZero || e.RequireCIAboveZero,
		envPass: improveFlags.envPass, egress: improveFlags.egress, ignored: res.IgnoredRepoKeys,
	}
	if !changed("holdout-tag") && e.HoldoutTag != "" {
		st.holdoutTag = e.HoldoutTag
	}
	if !changed("holdout-fraction") && e.HoldoutFraction != nil {
		st.holdoutFraction = *e.HoldoutFraction
	}
	if !changed("min-gain") && e.MinGain != nil {
		st.minGain = *e.MinGain
	}
	if !changed("max-regressions") && e.MaxRegressions != nil {
		st.maxRegressions = *e.MaxRegressions
	}
	for _, n := range []struct {
		flag   string
		cfgVal int
		dst    *int
	}{{"max-rounds", e.MaxRounds, &st.maxRounds}, {"max-holdout-evals", e.MaxHoldoutEvals, &st.maxHoldoutEvals}, {"runs", e.Runs, &st.runs}} {
		if !changed(n.flag) && n.cfgVal != 0 {
			*n.dst = n.cfgVal
		}
	}
	if !changed("env-pass") && len(e.EnvPass) > 0 {
		st.envPass = e.EnvPass
	}
	mode := improveFlags.isolation
	if !changed("isolation") && e.Isolation != "" {
		mode = e.Isolation
	}
	st.isolation = sandbox.ModeNone
	if mode != "" {
		if st.isolation, err = sandbox.ParseMode(mode); err != nil {
			return nil, oops.Wrap(err)
		}
	}
	switch {
	case improveFlags.with != "" && improveFlags.adapter != "":
		return nil, oops.Errorf("pass --with or --adapter, not both")
	case improveFlags.with != "":
		st.optimizer = improveFlags.with
	case improveFlags.adapter != "":
		st.optimizer = adapter.Prefix + improveFlags.adapter
	}
	return st, nil
}

func buildImproveOptions(errOut io.Writer, skill string, argv []string, configDirAbs, repo string, st *improveSettings) (*improve.Options, error) {
	for name, value := range map[string]float64{"--max-cost": improveFlags.maxCost, "--price-in": improveFlags.priceIn, "--price-out": improveFlags.priceOut} {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return nil, oops.Errorf("%s must be a finite number >= 0, got %v", name, value)
		}
	}
	price := evals.Price{InPerMTok: improveFlags.priceIn, OutPerMTok: improveFlags.priceOut}
	if price == (evals.Price{}) {
		var known bool
		if price, known = evals.PriceFor(improveFlags.model); !known {
			return nil, oops.Errorf("model %q has no built-in price, so --max-cost cannot be checked: pass --price-in and --price-out", improveFlags.model)
		}
	}
	evalRunner, err := buildImproveEvalRunner(errOut)
	if err != nil {
		return nil, err
	}
	date := improveFlags.date
	if date == "" {
		date = os.Getenv(EvalDateEnv)
	}
	return &improve.Options{
		ConfigDir: configDirAbs, RepoDir: repo, SkillID: skill, OptimizerArgv: argv, Exec: runner.Exec{}, HostEnv: runner.HostEnv(),
		EnvPass: st.envPass, Egress: st.egress, Timeout: improveFlags.timeout, Stderr: errOut,
		Eval: evalRunner, Harness: improveFlags.harness, Model: improveFlags.model, Runs: st.runs,
		Grade: evals.GradeOptions{AllowExec: improveFlags.allowExec}, Price: price,
		HoldoutTag: st.holdoutTag, HoldoutFraction: st.holdoutFraction, MinGain: st.minGain,
		MaxRegressions: st.maxRegressions, MaxRounds: st.maxRounds, MaxHoldoutEvals: st.maxHoldoutEvals,
		MinHoldoutCases: st.minHoldoutCases, MaxCostUSD: improveFlags.maxCost, StopAtFirstAccept: improveFlags.stopAtFirstAccept, RequireCIAboveZero: st.requireCI,
		MaxSkillGrowth: st.maxSkillGrowth, Isolation: st.isolation, Adapter: st.adapter,
		AllowFrontmatter: improveFlags.allowFrontmatter, AllowScripts: improveFlags.allowScripts,
		Git: evals.ExecGit, Date: date, ToolVersion: Version,
	}, nil
}

// buildImproveEvalRunner picks the eval runner. improve repeats a run itself
// (--runs, majority vote), so claude-plugin-eval is asked for one run per call.
func buildImproveEvalRunner(errOut io.Writer) (evals.Runner, error) {
	switch {
	case improveFlags.runnerCommand != "":
		return &evals.CommandRunner{Command: improveFlags.runnerCommand, Timeout: improveFlags.evalTimeout, Stderr: errOut}, nil
	case improveFlags.harness == "claude":
		return &evals.ClaudePluginEval{Bin: improveFlags.claudeBin, Runs: 1, JudgeModel: improveFlags.judgeModel,
			ExtraArgs: improveFlags.runnerArgs, Timeout: improveFlags.evalTimeout, Stderr: errOut}, nil
	}
	return nil, oops.Hint("Use --runner-command to plug in a runner for the "+improveFlags.harness+" harness").
		Errorf("no built-in eval runner for the %q harness", improveFlags.harness)
}

func printImprovePlan(w io.Writer, plan *improve.Plan) error {
	if improveFlags.format == formatJSON {
		doc := map[string]any{
			"schema": "improve-plan/1", "dry_run": true, "run_id": plan.RunID, "skill": plan.Skill.ID,
			"train": improve.IDs(plan.Split.Train), "held_out": improve.IDs(plan.Split.Held), "split_method": plan.Split.Method,
			"estimate": plan.Estimate, "max_cost_usd": plan.Opts.MaxCostUSD, "egress": plan.Opts.Egress, "warnings": plan.Warnings,
		}
		return writeImproveJSON(w, doc)
	}
	_, err := fmt.Fprintf(w, "%s\nDry run: nothing was run and nothing was written.\n", plan.Summary())
	return oops.Wrap(err)
}

func writeImproveJSON(w io.Writer, doc any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return oops.Wrapf(enc.Encode(doc), "write report")
}

func printImproveReport(w io.Writer, r *improve.Report) error {
	if improveFlags.format == formatJSON {
		return writeImproveJSON(w, r)
	}
	_, err := io.WriteString(w, improve.FormatReport(r))
	return oops.Wrap(err)
}
