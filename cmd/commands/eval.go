package commands

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/evals"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

// EvalDateEnv supplies the date recorded in eval results when --date is not given.
const EvalDateEnv = "AI_RULEZ_EVAL_DATE"

// exitEvalFailed is the exit status of a run with failing, erroring or invalid skills.
const exitEvalFailed = 2

// evalHarnessClaude is the default harness and runner executable name.
const evalHarnessClaude = "claude"

var evalFlags struct {
	harness         string
	runner          string
	runnerCommand   string
	claudeBin       string
	codexBin        string
	runnerArgs      []string
	runs            int
	judgeModel      string
	timeout         time.Duration
	model           string
	ablation        bool
	dryRun          bool
	estimate        bool
	mode            string
	surface         string
	scope           string
	descriptionFrom string
	format          string
	out             string
	maxCost         float64
	maxCostMode     string
	grader          string
	allowLLM        bool
	graderMaxCost   float64
	date            string
	changedOnly     bool
	base            string
	force           bool
	threshold       float64
	allowExec       bool
	noWrite         bool
	results         string
	priceIn         float64
	priceOut        float64
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
  claude-native        native activation surface through "claude -p" (harness claude)
  codex-native         native activation surface through "codex exec" (harness codex)

Results are recorded in .ai-rulez/eval-results.json with the skill's sha256 digest, and a
skill whose digest, cases, runner settings, harness, model, ablation setting and --allow-exec are
unchanged is not re-run (--force overrides); runs with errored cases are never cached.
Each record in the committed results file is signed with a per-user key, and a stored result
is replayed only when its signature verifies and its recorded digests match the skill on disk.
A record that cannot be verified (written on another machine, or edited by hand) is treated as
unverified and re-run. CI without the user key therefore re-runs committed records instead of
trusting them; --force also forces a re-run. Flags are
checked before anything is run, and each skill's result is saved as soon as it finishes. --dry-run lists what would run and an estimated cost without
calling any runner; --estimate is an alias. The estimate is a range (low, expected, high), and
each recorded run keeps the estimate next to what the runner reported.

--mode activation measures only whether the right skill is chosen for a prompt. --surface retrieval
ranks the prompts of the cases with the offline find_skill ranker (no model, no cost) and reports
activation rates with Wilson intervals, recall@k and a confusion matrix between sibling skills;
--scope picks which skills compete. --surface native installs every competing skill in a harness,
repeats each prompt --runs times (default 5) and records which skills the model loaded; it needs a runner
that declares the activation capability and the native surface (claude-native and codex-native do; a command
runner answers the capabilities probe) and is refused otherwise. The command exits 2 when a skill fails its pass
threshold, errors, or has invalid cases.

--grader builtin grades every rubric (and rubric_items checklist) with the configured [llm] model from the
runner's output instead of trusting the runner's own score. It needs a runner that returns the answer
(the command runner's "output"; claude-plugin-eval returns what its own grader read), --allow-llm, and
allow_network plus a model in the user config; --grader-max-cost caps its spend, which also counts towards
--max-cost.`,
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
	f.StringVar(&evalFlags.harness, "harness", evalHarnessClaude, "Harness the cases run against (recorded in the results)")
	f.StringVar(&evalFlags.runner, "runner", "", "Runner: claude-plugin-eval, command, claude-native or codex-native (default claude-plugin-eval for the claude harness, command when --runner-command is set; claude-native or codex-native for --surface native)")
	f.StringVar(&evalFlags.runnerCommand, "runner-command", "", "Shell command for the command runner: receives the request JSON on stdin, prints the response JSON")
	f.StringVar(&evalFlags.claudeBin, "claude-bin", evalHarnessClaude, "claude executable for the claude-plugin-eval and claude-native runners")
	f.StringVar(&evalFlags.codexBin, "codex-bin", "codex", "codex executable for the codex-native runner")
	f.StringArrayVar(&evalFlags.runnerArgs, "runner-arg", nil, "Extra argument for the claude-plugin-eval runner (for example --trust-plugin); repeatable")
	f.IntVar(&evalFlags.runs, "runs", 0, "Runs per case: for claude-plugin-eval (default 3, always passed to claude explicitly) and per prompt for --surface native (default 5)")
	f.StringVar(&evalFlags.judgeModel, "judge-model", "", "Grader model for claude-plugin-eval")
	f.DurationVar(&evalFlags.timeout, "timeout", 30*time.Minute, "Time limit for one skill with either runner; the runner's whole process tree is killed when it ends")
	f.StringVar(&evalFlags.model, "model", "", "Model to run the cases with (cases may override it)")
	f.BoolVar(&evalFlags.ablation, "ablation", false, "Also run every case without the skill and report the delta")
	f.BoolVar(&evalFlags.dryRun, "dry-run", false, "List what would run with an estimated cost range; call no runner and write nothing")
	f.BoolVar(&evalFlags.estimate, "estimate", false, "Alias of --dry-run")
	f.StringVar(&evalFlags.mode, "mode", evals.ModeCases, "What to measure: cases (full eval cases through a runner) or activation (only whether the right skill is chosen)")
	f.StringVar(&evalFlags.surface, "surface", "", "With --mode activation: retrieval (offline find_skill ranking, free) or native (a runner that declares the activation capability and the native surface)")
	f.StringVar(&evalFlags.scope, "scope", evals.ScopeDomain, "With --mode activation: the skills that compete for a prompt: domain (the skill's domain plus root skills) or all")
	f.StringVar(&evalFlags.descriptionFrom, "description-from", "", "With --mode activation and one skill: measure the description in this file instead of the skill's own, for this run only (nothing is recorded, the source is not edited)")
	addFormatFlag(f, &evalFlags.format, evals.FormatMarkdown, evals.FormatMarkdown, evals.FormatJSON, evals.FormatMarkdown, evals.FormatJUnit)
	addJSONFlagAlias(f)
	f.StringVar(&evalFlags.out, "out", "", "Write the report to <dir>/eval-report.<ext> instead of standard output")
	f.Float64Var(&evalFlags.maxCost, "max-cost", 0, "Advisory run-wide spend cap in USD (finite, >= 0; 0 means no limit): refuse to start when the estimate exceeds it, skip skills once spend reaches it, warn when a runner overshoots the budget it was given; a runner that reports no cost is assumed to have spent the whole budget")
	f.StringVar(&evalFlags.maxCostMode, "max-cost-mode", "", "Estimate figure that must fit under --max-cost before a run starts: expected (default for case runs) or high (default for --mode activation)")
	f.StringVar(&evalFlags.grader, "grader", evals.GraderRunner, "Who grades a case's rubric: runner (whatever the runner provides) or builtin (the configured [llm] model judges the runner's transcript; needs --allow-llm)")
	f.BoolVar(&evalFlags.allowLLM, "allow-llm", false, "Agree that --grader builtin sends the runner's transcripts to the configured [llm] model (it also needs allow_network and a model in the user config)")
	f.Float64Var(&evalFlags.graderMaxCost, "grader-max-cost", defaultGraderMaxCost, "Spend cap in USD for --grader builtin (0 keeps only the [llm] limits)")
	f.StringVar(&evalFlags.date, "date", "", "Date recorded in the results (default $"+EvalDateEnv+"; the clock is never read)")
	f.BoolVar(&evalFlags.changedOnly, "changed-only", false, "Only skills with files changed against --base (git diff, plus untracked files)")
	f.StringVar(&evalFlags.base, "base", "HEAD", "Git ref --changed-only compares the working tree against")
	f.BoolVar(&evalFlags.force, "force", false, "Ignore the result cache and re-run every selected skill")
	f.Float64Var(&evalFlags.threshold, "threshold", 1, "Pass rate (0 to 1) a skill needs; 0 records scores without gating. Falls back to [lint.evals] min_pass_rate when not given")
	f.BoolVar(&evalFlags.allowExec, "allow-exec", false, "Run command_exit assertions (they execute commands from the case files)")
	f.BoolVar(&evalFlags.noWrite, "no-write", false, "Do not update the results file")
	f.StringVar(&evalFlags.results, "results", "", "Results file (default <config dir>/eval-results.json)")
	f.Float64Var(&evalFlags.priceIn, "price-in", 0, "USD per million input tokens for the estimate (default by model tier)")
	f.Float64Var(&evalFlags.priceOut, "price-out", 0, "USD per million output tokens for the estimate (default by model tier)")
	f.StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
	EvalCmd.AddCommand(evalRunCmd)
}

func runEval(cmd *cobra.Command, skills []string) (failed bool, err error) {
	ctx, stop := signal.NotifyContext(commandContext(cmd), os.Interrupt)
	defer stop()
	// Everything that can be rejected is rejected before any paid work starts.
	if err := validateEvalFlags(cmd); err != nil {
		return false, err
	}
	cfg, err := loadConfigForCommand(ctx, nil)
	if err != nil {
		return false, err
	}
	if evalFlags.mode == evals.ModeActivation {
		return runEvalActivation(ctx, cmd, skills, cfg)
	}
	opts, err := buildEvalOptions(cmd, skills, cfg.ConfigDir, cfg.BaseDir)
	if err != nil {
		return false, err
	}
	opts.Params, opts.Price = estimateParams(cfg), evalPrice(cfg, evalFlags.model)
	grader, release, err := buildGrader(cmd, cfg)
	if err != nil {
		return false, err
	}
	defer release()
	opts.Grader = grader
	applyConfigPassFloor(cmd, cfg, opts)
	store, err := attachStore(opts, resultsPath(cfg.ConfigDir))
	if err != nil {
		return false, err
	}
	save := !opts.DryRun && !evalFlags.noWrite
	if save {
		opts.OnSkill = saveEachSkill(store, resultsPath(cfg.ConfigDir))
	}

	report, runErr := evals.Run(ctx, opts)
	if report == nil {
		return false, runErr
	}
	if save && anyRan(report) {
		if err := store.Save(resultsPath(cfg.ConfigDir)); err != nil {
			return false, err
		}
	}
	return report.Failed, errors.Join(writeEvalReport(cmd, report), runErr)
}

// applyConfigPassFloor takes the pass-rate floor from [lint.evals] unless --threshold was given.
func applyConfigPassFloor(cmd *cobra.Command, cfg *config.Config, opts *evals.RunOptions) {
	if cfg.Lint != nil && cfg.Lint.Evals != nil && cfg.Lint.Evals.MinPassRate > 0 && !thresholdGiven(cmd) {
		floor := cfg.Lint.Evals.MinPassRate
		opts.PassThreshold = &floor
	}
}

// saveEachSkill writes the store (atomically) as soon as a skill has run, so a
// crash or Ctrl-C keeps the skills that already paid for themselves.
func saveEachSkill(store *evals.Store, path string) func(*evals.SkillRun) error {
	return func(run *evals.SkillRun) error {
		if run.Status != evals.RunRan {
			return nil
		}
		return store.Save(path)
	}
}

func commandContext(cmd *cobra.Command) context.Context {
	if ctx := cmd.Context(); ctx != nil {
		return ctx
	}
	return cmdContext()
}

// attachStore loads the results file into opts.
func attachStore(opts *evals.RunOptions, path string) (*evals.Store, error) {
	key := evals.UserKey()
	store, err := evals.LoadStoreKeyed(path, key)
	if err != nil {
		return nil, err
	}
	opts.Store = store
	return store, nil
}

// thresholdGiven reports whether --threshold was passed explicitly.
func thresholdGiven(cmd *cobra.Command) bool {
	flag := cmd.Flags().Lookup("threshold")
	return flag != nil && flag.Changed
}

// validateEvalFlags checks every flag that does not need the project, so a typo
// costs nothing.
func validateEvalFlags(cmd *cobra.Command) error {
	switch evalFlags.format {
	case evals.FormatJSON, evals.FormatMarkdown, evals.FormatJUnit:
	default:
		return oops.Errorf("unknown --format %q (use json, markdown or junit)", evalFlags.format)
	}
	for name, value := range map[string]float64{"--max-cost": evalFlags.maxCost, "--price-in": evalFlags.priceIn, "--price-out": evalFlags.priceOut} {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return oops.Errorf("%s must be a finite number >= 0, got %v", name, value)
		}
	}
	if thresholdGiven(cmd) && (math.IsNaN(evalFlags.threshold) || evalFlags.threshold < 0 || evalFlags.threshold > 1) {
		return oops.Errorf("--threshold must be between 0 and 1, got %v", evalFlags.threshold)
	}
	if evalFlags.runs < 0 {
		return oops.Errorf("--runs must be >= 0, got %d", evalFlags.runs)
	}
	if err := validateActivationFlags(); err != nil {
		return err
	}
	if err := validateGraderFlags(); err != nil {
		return err
	}
	if evalFlags.timeout < 0 {
		return oops.Errorf("--timeout must be >= 0, got %s", evalFlags.timeout)
	}
	return nil
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
		Ablation: evalFlags.ablation, DryRun: evalDryRun(), Force: evalFlags.force,
		MaxCostUSD: evalFlags.maxCost, MaxCostMode: evalFlags.maxCostMode, Date: date,
		Grade: evals.GradeOptions{AllowExec: evalFlags.allowExec}, ToolVersion: Version,
		Price: evals.Price{InPerMTok: evalFlags.priceIn, OutPerMTok: evalFlags.priceOut},
	}
	if thresholdGiven(cmd) {
		threshold := evalFlags.threshold
		opts.PassThreshold = &threshold
	}
	if opts.Changed, err = changedEvalSkills(absDir, baseDir); err != nil {
		return nil, err
	}
	runner, runs, err := buildEvalRunner(cmd)
	if err != nil {
		if !evalDryRun() {
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

// evalDryRun says whether the run only estimates: --dry-run or its alias --estimate.
func evalDryRun() bool { return evalFlags.dryRun || evalFlags.estimate }

// changedEvalSkills is the set of skills --changed-only selects; nil without the flag.
func changedEvalSkills(absDir, baseDir string) (map[string]bool, error) {
	if !evalFlags.changedOnly {
		return nil, nil
	}
	all, err := evals.FindSkills(absDir)
	if err != nil {
		return nil, oops.Wrapf(err, "list skills")
	}
	repo, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, oops.Wrapf(err, "resolve project directory")
	}
	changed, err := evals.ChangedSkills(evals.ExecGit, repo, evalFlags.base, all)
	return changed, oops.Wrapf(err, "find changed skills")
}

// buildEvalRunner selects the runner and the runs per case its estimate assumes.
func buildEvalRunner(cmd *cobra.Command) (evals.Runner, int, error) {
	name := evalFlags.runner
	if name == "" {
		switch {
		case evalFlags.runnerCommand != "":
			name = evals.RunnerCommand
		case evalFlags.harness == evalHarnessClaude:
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
		// Pass the effective run count: the estimate assumes it, so claude must be told the
		// same number instead of falling back to a default of its own.
		return &evals.ClaudePluginEval{Bin: evalFlags.claudeBin, Runs: runs, JudgeModel: evalFlags.judgeModel,
			ExtraArgs: evalFlags.runnerArgs, Timeout: evalFlags.timeout, Stderr: cmd.ErrOrStderr(),
			IgnoreRubricVerdict: evalFlags.grader == evals.GraderBuiltin}, runs, nil
	case evals.RunnerCommand:
		if evalFlags.runnerCommand == "" {
			return nil, 1, oops.Errorf("--runner command needs --runner-command")
		}
		return &evals.CommandRunner{Command: evalFlags.runnerCommand, Timeout: evalFlags.timeout, Stderr: cmd.ErrOrStderr()}, 1, nil
	}
	return nil, 1, oops.Errorf("unknown runner %q (use %s or %s)", name, evals.RunnerClaudePluginEval, evals.RunnerCommand)
}

func writeEvalReport(cmd *cobra.Command, report *evals.RunReport) error {
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
