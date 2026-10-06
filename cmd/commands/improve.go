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

	"github.com/Goldziher/ai-rulez/v5/internal/evals"
	"github.com/Goldziher/ai-rulez/v5/internal/improve"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
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
no longer matches its digest, or when it now breaks the diff policy. Nothing is committed.`,
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
		_, err = improve.Apply(ctx, &improve.ApplyOptions{
			ConfigDir: configDirAbs, RunID: args[0], Yes: improveFlags.yes, Out: cmd.OutOrStdout(), Confirm: confirmProceed,
		})
		return oops.Wrap(err)
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
	f.StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
	improveApplyCmd.Flags().BoolVarP(&improveFlags.yes, "yes", "y", false, "Write without the confirmation prompt")
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
	argv, err := improve.ParseArgv(improveFlags.with)
	if err != nil {
		return false, oops.Hint("Pass the optimizer with --with, for example --with 'python optimize.py'").Wrap(err)
	}
	cfg, err := loadConfigForCommand(ctx, nil)
	if err != nil {
		return false, err
	}
	configDirAbs, err := filepath.Abs(cfg.ConfigDir)
	if err != nil {
		return false, oops.Wrapf(err, "resolve config directory")
	}
	repo, err := filepath.Abs(cfg.BaseDir)
	if err != nil {
		return false, oops.Wrapf(err, "resolve project directory")
	}
	opts, err := buildImproveOptions(errOut, skill, argv, configDirAbs, repo)
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
		return false, oops.Wrap(err)
	}
	if err := printImproveReport(cmd.OutOrStdout(), report); err != nil {
		return false, err
	}
	return !report.Accepted(), nil
}

func buildImproveOptions(errOut io.Writer, skill string, argv []string, configDirAbs, repo string) (*improve.Options, error) {
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
		EnvPass: improveFlags.envPass, Egress: improveFlags.egress, Timeout: improveFlags.timeout, Stderr: errOut,
		Eval: evalRunner, Harness: improveFlags.harness, Model: improveFlags.model, Runs: improveFlags.runs,
		Grade: evals.GradeOptions{AllowExec: improveFlags.allowExec}, Price: price,
		HoldoutTag: improveFlags.holdoutTag, HoldoutFraction: improveFlags.holdoutFraction, MinGain: improveFlags.minGain,
		MaxRegressions: improveFlags.maxRegressions, MaxRounds: improveFlags.maxRounds, MaxHoldoutEvals: improveFlags.maxHoldoutEvals,
		MaxCostUSD: improveFlags.maxCost, StopAtFirstAccept: improveFlags.stopAtFirstAccept,
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
	var b strings.Builder
	fmt.Fprintf(&b, "Run %s for %s: %s\n", r.RunID, r.Skill, r.Status)
	if r.Baseline != nil {
		fmt.Fprintf(&b, "Baseline held-out pass rate %.0f%% (%d case(s))\n", r.Baseline.PassRate*100, r.Baseline.Scored)
	}
	for i := range r.Rounds {
		rd := &r.Rounds[i]
		fmt.Fprintf(&b, "Round %d: %s", rd.Round, rd.Decision)
		if rd.Held != nil {
			fmt.Fprintf(&b, " (held-out %.0f%% -> %.0f%%, %+.1f points, %d win(s), %d loss(es))", rd.Held.Base.PassRate*100, rd.Held.Cand.PassRate*100, rd.Held.Gain*100, len(rd.Held.Wins), len(rd.Held.Losses))
		}
		b.WriteString("\n")
		for _, v := range rd.Violations {
			fmt.Fprintf(&b, "  %s\n", v.String())
		}
		for _, reason := range rd.Reasons {
			fmt.Fprintf(&b, "  %s\n", reason)
		}
		for _, warn := range rd.Warnings {
			fmt.Fprintf(&b, "  warning: %s\n", warn)
		}
		if rd.Description != nil {
			fmt.Fprintf(&b, "  description: %q -> %q\n", rd.Description.Before, rd.Description.After)
		}
	}
	if r.Reason != "" {
		fmt.Fprintf(&b, "%s\n", r.Reason)
	}
	fmt.Fprintf(&b, "Spent $%.2f of $%.2f (evals $%.2f, optimizer-reported $%.2f)\n", r.Costs.TotalUSD, r.Costs.MaxUSD, r.Costs.EvalUSD, r.Costs.OptimizerUSD)
	if r.Costs.OptimizerReportedNoCost {
		b.WriteString("warning: the optimizer reported no cost; its own spend is bounded only by its credentials\n")
	}
	fmt.Fprintf(&b, "Report: .ai-rulez/local/improve/%s/report.json\n", r.RunID)
	if r.Accepted() {
		fmt.Fprintf(&b, "Diff:   .ai-rulez/local/improve/%s/diff.patch\nReview it, then: ai-rulez improve apply %s\n", r.RunID, r.RunID)
	}
	_, err := io.WriteString(w, b.String())
	return oops.Wrap(err)
}
