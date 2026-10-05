package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Goldziher/ai-rulez/internal/evals"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

// EvalDateEnv supplies the date recorded in eval results when --date is not given.
const EvalDateEnv = "AI_RULEZ_EVAL_DATE"

// exitEvalFailed is the exit status of a run with failing, erroring or invalid skills.
const exitEvalFailed = 2

var evalFlags struct {
	harness       string
	runner        string
	runnerCommand string
	claudeBin     string
	runnerArgs    []string
	runs          int
	judgeModel    string
	timeout       time.Duration
	model         string
	ablation      bool
	dryRun        bool
	format        string
	out           string
	maxCost       float64
	date          string
	changedOnly   bool
	base          string
	force         bool
	threshold     float64
	allowExec     bool
	noWrite       bool
	results       string
	priceIn       float64
	priceOut      float64
}

// EvalCmd groups the skill eval commands.
var EvalCmd = &cobra.Command{
	Use:   "eval",
	Short: "Run skill evals and score them",
}

var evalRunCmd = &cobra.Command{
	Use:   "run [skill...]",
	Short: "Run the eval cases of skills through a pluggable runner and score them",
	Long: `Run the eval cases under skills/<name>/evals/ (and .ai-rulez/evals/<name>/) and score
each skill: pass rate, trigger precision and recall, the delta against a run without the
skill (--ablation), and token cost.

Cases are files named *.eval.yaml, *.eval.yml or *.eval.json; see docs/evals.md and
schema/eval-case.schema.json. A runner executes them:

  claude-plugin-eval   translate the cases for "claude plugin eval" and run it (harness claude)
  command              pipe the request JSON to --runner-command, read the response JSON

Results are recorded in .ai-rulez/eval-results.json with the skill's sha256 digest, and a
skill whose digest, cases, runner, harness, model and ablation setting are unchanged is not
re-run (--force overrides). --dry-run lists what would run and an estimated cost without
calling any runner. The command exits 2 when a skill fails its pass threshold, errors, or has
invalid cases.`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		failed, err := runEval(cmd, args)
		if err != nil {
			return err
		}
		if failed {
			os.Exit(exitEvalFailed)
		}
		return nil
	},
}

func init() {
	f := evalRunCmd.Flags()
	f.StringVar(&evalFlags.harness, "harness", "claude", "Harness the cases run against (recorded in the results)")
	f.StringVar(&evalFlags.runner, "runner", "", "Runner: claude-plugin-eval or command (default claude-plugin-eval for the claude harness, command when --runner-command is set)")
	f.StringVar(&evalFlags.runnerCommand, "runner-command", "", "Shell command for the command runner: receives the request JSON on stdin, prints the response JSON")
	f.StringVar(&evalFlags.claudeBin, "claude-bin", "claude", "claude executable for the claude-plugin-eval runner")
	f.StringArrayVar(&evalFlags.runnerArgs, "runner-arg", nil, "Extra argument for the claude-plugin-eval runner (for example --trust-plugin); repeatable")
	f.IntVar(&evalFlags.runs, "runs", 0, "Runs per case for claude-plugin-eval (its default is 3)")
	f.StringVar(&evalFlags.judgeModel, "judge-model", "", "Grader model for claude-plugin-eval")
	f.DurationVar(&evalFlags.timeout, "timeout", 30*time.Minute, "Time limit for one skill with the command runner")
	f.StringVar(&evalFlags.model, "model", "", "Model to run the cases with (cases may override it)")
	f.BoolVar(&evalFlags.ablation, "ablation", false, "Also run every case without the skill and report the delta")
	f.BoolVar(&evalFlags.dryRun, "dry-run", false, "List what would run with an estimated cost; call no runner and write nothing")
	f.StringVar(&evalFlags.format, "format", evals.FormatMarkdown, "Report format: json, markdown or junit")
	f.StringVar(&evalFlags.out, "out", "", "Write the report to <dir>/eval-report.<ext> instead of standard output")
	f.Float64Var(&evalFlags.maxCost, "max-cost", 0, "Stop above this many USD: refuse to start when the estimate exceeds it, skip skills once spend reaches it")
	f.StringVar(&evalFlags.date, "date", "", "Date recorded in the results (default $"+EvalDateEnv+"; the clock is never read)")
	f.BoolVar(&evalFlags.changedOnly, "changed-only", false, "Only skills with files changed against --base (git diff, plus untracked files)")
	f.StringVar(&evalFlags.base, "base", "HEAD", "Git ref --changed-only compares the working tree against")
	f.BoolVar(&evalFlags.force, "force", false, "Ignore the result cache and re-run every selected skill")
	f.Float64Var(&evalFlags.threshold, "threshold", -1, "Pass rate a skill needs (default [lint.evals] min_pass_rate, else 1)")
	f.BoolVar(&evalFlags.allowExec, "allow-exec", false, "Run command_exit assertions (they execute commands from the case files)")
	f.BoolVar(&evalFlags.noWrite, "no-write", false, "Do not update the results file")
	f.StringVar(&evalFlags.results, "results", "", "Results file (default <config dir>/eval-results.json)")
	f.Float64Var(&evalFlags.priceIn, "price-in", 0, "USD per million input tokens for the estimate (default by model tier)")
	f.Float64Var(&evalFlags.priceOut, "price-out", 0, "USD per million output tokens for the estimate (default by model tier)")
	f.StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
	EvalCmd.AddCommand(evalRunCmd)
}

func runEval(cmd *cobra.Command, skills []string) (failed bool, err error) {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	cfg, err := loadConfigForCommand(ctx, nil)
	if err != nil {
		return false, err
	}
	opts, err := buildEvalOptions(cmd, skills, cfg.ConfigDir, cfg.BaseDir)
	if err != nil {
		return false, err
	}
	if cfg.Lint != nil && cfg.Lint.Evals != nil && cfg.Lint.Evals.MinPassRate > 0 && evalFlags.threshold < 0 {
		opts.PassThreshold = cfg.Lint.Evals.MinPassRate
	}
	store, err := evals.LoadStore(resultsPath(cfg.ConfigDir))
	if err != nil {
		return false, err
	}
	opts.Store = store

	report, err := evals.Run(ctx, opts)
	if err != nil {
		return false, err
	}
	if !opts.DryRun && !evalFlags.noWrite && anyRan(report) {
		if err := store.Save(resultsPath(cfg.ConfigDir)); err != nil {
			return false, err
		}
	}
	if err := writeEvalReport(cmd, report); err != nil {
		return false, err
	}
	return report.Failed, nil
}

func resultsPath(configDirAbs string) string {
	if evalFlags.results != "" {
		return evalFlags.results
	}
	return evals.DefaultStorePath(configDirAbs)
}

func anyRan(report *evals.RunReport) bool {
	for i := range report.Skills {
		if report.Skills[i].Status == evals.RunRan {
			return true
		}
	}
	return false
}

func buildEvalOptions(cmd *cobra.Command, skills []string, cfgDir, baseDir string) (*evals.RunOptions, error) {
	absDir, err := filepath.Abs(cfgDir)
	if err != nil {
		return nil, oops.Wrapf(err, "resolve config directory")
	}
	date := evalFlags.date
	if date == "" {
		date = os.Getenv(EvalDateEnv)
	}
	opts := &evals.RunOptions{
		ConfigDir: absDir, Skills: skills, Harness: evalFlags.harness, Model: evalFlags.model,
		Ablation: evalFlags.ablation, DryRun: evalFlags.dryRun, Force: evalFlags.force,
		MaxCostUSD: evalFlags.maxCost, Date: date, PassThreshold: max(evalFlags.threshold, 0),
		Grade: evals.GradeOptions{AllowExec: evalFlags.allowExec},
		Price: evals.Price{InPerMTok: evalFlags.priceIn, OutPerMTok: evalFlags.priceOut},
	}
	if evalFlags.changedOnly {
		all, err := evals.FindSkills(absDir)
		if err != nil {
			return nil, oops.Wrapf(err, "list skills")
		}
		repo, err := filepath.Abs(baseDir)
		if err != nil {
			return nil, oops.Wrapf(err, "resolve project directory")
		}
		changed, err := evals.ChangedSkills(evals.ExecGit, repo, evalFlags.base, all)
		if err != nil {
			return nil, oops.Wrapf(err, "find changed skills")
		}
		opts.Changed = changed
	}
	runner, runs, err := buildEvalRunner(cmd)
	if err != nil {
		if !evalFlags.dryRun {
			return nil, err
		}
		runs = 1 // a dry run needs no runner, only the run count for its estimate
		if evalFlags.runs > 0 {
			runs = evalFlags.runs
		}
	}
	opts.Runner, opts.EstimateRuns = runner, runs
	return opts, nil
}

// buildEvalRunner selects the runner and the runs per case its estimate assumes.
func buildEvalRunner(cmd *cobra.Command) (evals.Runner, int, error) {
	name := evalFlags.runner
	if name == "" {
		switch {
		case evalFlags.runnerCommand != "":
			name = evals.RunnerCommand
		case evalFlags.harness == "claude":
			name = evals.RunnerClaudePluginEval
		default:
			return nil, 1, oops.Hint("Use --runner-command to plug in a runner for the "+evalFlags.harness+" harness").
				Errorf("no built-in runner for the %q harness", evalFlags.harness)
		}
	}
	switch name {
	case evals.RunnerClaudePluginEval:
		runs := evalFlags.runs
		if runs <= 0 {
			runs = 3
		}
		return &evals.ClaudePluginEval{Bin: evalFlags.claudeBin, Runs: evalFlags.runs, JudgeModel: evalFlags.judgeModel,
			ExtraArgs: evalFlags.runnerArgs, Stderr: cmd.ErrOrStderr()}, runs, nil
	case evals.RunnerCommand:
		if evalFlags.runnerCommand == "" {
			return nil, 1, oops.Errorf("--runner command needs --runner-command")
		}
		return &evals.CommandRunner{Command: evalFlags.runnerCommand, Timeout: evalFlags.timeout, Stderr: cmd.ErrOrStderr()}, 1, nil
	}
	return nil, 1, oops.Errorf("unknown runner %q (use %s or %s)", name, evals.RunnerClaudePluginEval, evals.RunnerCommand)
}

func writeEvalReport(cmd *cobra.Command, report *evals.RunReport) error {
	switch evalFlags.format {
	case evals.FormatJSON, evals.FormatMarkdown, evals.FormatJUnit:
	default:
		return oops.Errorf("unknown --format %q (use json, markdown or junit)", evalFlags.format)
	}
	if evalFlags.out == "" {
		return oops.Wrapf(report.Write(cmd.OutOrStdout(), evalFlags.format), "write report")
	}
	if err := os.MkdirAll(evalFlags.out, 0o750); err != nil {
		return oops.Wrapf(err, "create output directory")
	}
	path := filepath.Join(evalFlags.out, "eval-report."+evals.Extension(evalFlags.format))
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
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Wrote %s (%d skills, %s)\n", path, len(report.Skills), summaryWord(report))
	return oops.Wrapf(err, "write summary")
}

func summaryWord(report *evals.RunReport) string {
	switch {
	case report.DryRun:
		return fmt.Sprintf("dry run, estimated $%.2f", report.Estimate.CostUSD)
	case report.Failed:
		return "failing"
	}
	return "passing"
}
