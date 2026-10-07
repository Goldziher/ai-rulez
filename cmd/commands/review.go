package commands

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/jsondoc"

	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
	rv "github.com/Goldziher/ai-rulez/v5/internal/review"
)

// Exit statuses of the review commands: 1 when a review could not run (or --estimate shows a
// real run would be refused), 2 when a gate failed, a calibration missed its thresholds or a
// drift check found a regression.
const (
	exitReviewRefused = 1
	exitReviewGate    = 2
)

// exitRubricFindings is the exit status of `rubric lint` with findings, matching `validate`.
const exitRubricFindings = 2

// maxConcurrency bounds --concurrency: more parallel calls mostly trip provider rate limits.
const maxConcurrency = 16

var reviewFlags struct {
	rubric         string
	content        string
	estimate       bool
	showPrompt     bool
	model          string
	models         string
	maxCost        float64
	maxCalls       int
	includeImports bool
	format         string
	out            string
	semantic       bool
	k              int
	since          string
	role           string
	profile        string
	gate           bool
	gateLevel      string
	baseline       string
	writeBaseline  string
	noCache        bool
	cacheDir       string
	concurrency    int
}

// ReviewCmd scores skills, agents, commands and rules against a rubric.
var ReviewCmd = &cobra.Command{
	Use:   "review [id|name|path...]",
	Short: "Score skills, agents, commands and rules against a rubric, offline or with an LLM judge",
	Long: `Score content against a user-owned, versioned rubric (default builtin:skill-quality).

Without --semantic the review is offline: the score is computed from the lint findings each
rubric dimension names as its twins, and no model is ever called. Dimensions with no twin are
listed as not scored. The weights and the scoring formula are printed in every JSON report.

  --semantic     add an LLM judge for what lint cannot see: a vague or overlapping trigger, a
                 body that contradicts its description, injection intent, scope creep. It needs
                 [llm] allow_network = true in user scope and a model; findings are advisory.
                 Every quote a verdict cites must appear in the item. Flagged dimensions are
                 voted on (--k). Answers are cached; unchanged content costs nothing on re-run.
  --gate         with --semantic: exit 2 when a stable fail verdict exists on a dimension the
                 judge was calibrated for. Refused (exit 1) unless "review calibrate" measured
                 this exact rubric, prompt, golden set and model.
  --estimate     print the egress manifest of a judged run: which items would send what (sizes
                 and a hash, never content), to which host, estimated tokens and cost, and
                 whether a real run would be refused by the spend caps. Nothing is sent.
                 --show-prompt adds the exact planned messages.
  --baseline F   hide the findings in F (a baseline or an earlier --format json report); only
                 new findings are reported and gated. --write-baseline writes one.
  --since REV    only items changed since a git revision; --role and --profile review one slice.
  --models A,B   judge with each model and report where they disagree.

An item with a secret or hidden-character finding (AR001, AR002) is withheld: it is never part
of the manifest or sent. With [review] on_secret = "redact" an item with a credential is sent
with the credential masked. Content from includes and installed skills is skipped unless
--include-imports. Rubrics live in .ai-rulez/rubrics/<id>/ (see "ai-rulez rubric --help").

Exit status: 0 on success; 1 when the review could not run or was refused; 2 when --gate fails.
See docs/review.md.`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		code, err := runReview(cmd, args, cmd.OutOrStdout())
		if err != nil {
			return err
		}
		if code != 0 {
			os.Exit(code)
		}
		return nil
	},
}

// RubricCmd groups the rubric tools.
var RubricCmd = &cobra.Command{
	Use:   "rubric",
	Short: "List, show and lint review rubrics",
	Long: `Inspect the rubrics "ai-rulez review" scores against: the embedded builtin:skill-quality
and the ones under .ai-rulez/rubrics/<id>/ (rubric.toml, optional system.md, golden/*.golden.yaml
and calibration.json). See docs/review.md.`,
}

var rubricFormat string

var rubricListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the built-in and project rubrics",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runRubricList(cmd, cmd.OutOrStdout())
	},
}

var rubricShowCmd = &cobra.Command{
	Use:   "show [id]",
	Short: "Print a rubric's dimensions, weights and scoring formula",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRubricShow(cmd, args, cmd.OutOrStdout())
	},
}

var rubricLintCmd = &cobra.Command{
	Use:   "lint [id...]",
	Short: "Check rubrics, golden files and calibration records (AR9G8)",
	Long: `Check the project rubrics (or the named ones, or builtin:<id>): schema version, id, weights summing
to 1, unique dimension ids and codes, twins that are registered lint rules, thresholds in range,
golden cases with two labelers and known dimensions, and calibration.json. A rubric directory or
file that is a symlink is refused. Findings are AR9G8 errors.

Exit status: 0 when clean, 2 when there are findings, 1 when a rubric could not be read.`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		found, err := runRubricLint(cmd, args, cmd.OutOrStdout())
		if err != nil {
			return err
		}
		if found {
			os.Exit(exitRubricFindings)
		}
		return nil
	},
}

func init() {
	f := ReviewCmd.Flags()
	f.StringVar(&reviewFlags.rubric, "rubric", "", "Rubric id: builtin:<id> or a directory under .ai-rulez/rubrics (default [review] rubric, else builtin:skill-quality)")
	f.StringVar(&reviewFlags.content, "content", "", "What a judge receives: descriptions (name, description, frontmatter keys) or full (adds the frontmatter and body); default [review] content")
	f.BoolVar(&reviewFlags.semantic, "semantic", false, "Add the LLM judge (needs [llm] allow_network = true in user scope; findings are advisory)")
	f.BoolVar(&reviewFlags.estimate, "estimate", false, "Print the egress manifest and cost range of a model-judged run; send nothing")
	f.BoolVar(&reviewFlags.showPrompt, "show-prompt", false, "With --estimate, print the exact planned messages")
	f.StringVar(&reviewFlags.model, "model", "", "Model to judge with or price (default [llm] model)")
	f.StringVar(&reviewFlags.models, "models", "", "Comma-separated models to judge with and compare; the first is the primary")
	f.Float64Var(&reviewFlags.maxCost, "max-cost", 0, "Spend cap in USD (default [review] max_cost_usd, else 0.50; 0 = unlimited)")
	f.IntVar(&reviewFlags.maxCalls, "max-calls", 0, "Call cap (default [review] max_calls, else 300)")
	f.IntVar(&reviewFlags.k, "k", 0, "Most votes for a flagged dimension (default the rubric's votes.max)")
	f.BoolVar(&reviewFlags.gate, "gate", false, "Exit 2 on a stable fail verdict of a calibrated dimension; refused without a matching calibration record")
	f.StringVar(&reviewFlags.gateLevel, "gate-level", "", "Lowest severity ceiling that gates: info, warning or error (default [review.gate] level, else warning)")
	f.StringVar(&reviewFlags.baseline, "baseline", "", "Hide the findings in this baseline (or earlier --format json report); report and gate only new ones")
	f.StringVar(&reviewFlags.writeBaseline, "write-baseline", "", "Write a baseline of the findings of this run to this file")
	f.StringVar(&reviewFlags.since, "since", "", "Only items changed since this git revision (committed, staged, unstaged and untracked)")
	f.StringVar(&reviewFlags.role, "role", "", "Review the content slice of this role (see 'ai-rulez roles list')")
	f.StringVar(&reviewFlags.profile, "profile", "", "Review the content of this profile (or a comma-separated list)")
	f.IntVar(&reviewFlags.concurrency, "concurrency", 0, "Items judged at once (default 4, at most 16)")
	f.BoolVar(&reviewFlags.noCache, "no-cache", false, "Do not read or write the model response cache")
	f.StringVar(&reviewFlags.cacheDir, "cache-dir", "", "Response cache directory (default the user cache directory of this project)")
	f.BoolVar(&reviewFlags.includeImports, "include-imports", false, "Also review content from includes, installed skills and builtins")
	addFormatFlag(f, &reviewFlags.format, formatText, formatText, rv.Formats()...)
	f.StringVar(&reviewFlags.out, "out", "", "Write the report to this file instead of standard output")
	f.BoolVar(&noLocal, "no-local", false, "Ignore the machine-local config.local.* overlay and local/ content")
	f.StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")

	addFormatFlag(RubricCmd.PersistentFlags(), &rubricFormat, formatText, formatText, rv.Formats()...)
	RubricCmd.PersistentFlags().StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
	RubricCmd.AddCommand(rubricListCmd, rubricShowCmd, rubricLintCmd)
	ReviewCmd.AddCommand(reviewCalibrateCmd, reviewFixCmd, reviewExplainCmd)
}

// validateReviewFlags rejects what can be rejected before any work starts.
func validateReviewFlags(cmd *cobra.Command) error {
	if reviewFlags.showPrompt && !reviewFlags.estimate {
		return oops.Errorf("--show-prompt needs --estimate")
	}
	switch reviewFlags.content {
	case "", config.ReviewContentDescriptions, config.ReviewContentFull:
	default:
		return oops.Errorf("unknown --content %q (use %s or %s)", reviewFlags.content, config.ReviewContentDescriptions, config.ReviewContentFull)
	}
	if reviewFlags.concurrency < 0 || reviewFlags.concurrency > maxConcurrency {
		return oops.Errorf("--concurrency must be between 1 and %d", maxConcurrency)
	}
	if err := checkCaps(reviewFlags.maxCost, reviewFlags.maxCalls, reviewFlags.k); err != nil {
		return err
	}
	if err := validateReviewFlagScope(cmd); err != nil {
		return err
	}
	switch reviewFlags.gateLevel {
	case "", config.ReviewGateInfo, config.ReviewGateWarning, config.ReviewGateError:
	default:
		return oops.Errorf("unknown --gate-level %q (use info, warning or error)", reviewFlags.gateLevel)
	}
	if reviewFlags.role != "" && reviewFlags.profile != "" {
		return oops.Errorf("--role and --profile are mutually exclusive")
	}
	if reviewFlags.estimate && reviewFlags.gate {
		return oops.Errorf("--gate needs a judged run; --estimate sends nothing")
	}
	if reviewFlags.models != "" && cmd.Flags().Changed("model") {
		return oops.Errorf("use --model or --models, not both")
	}
	return nil
}

// validateReviewFlagScope rejects flags given for a mode that does not use them.
func validateReviewFlagScope(cmd *cobra.Command) error {
	fl := cmd.Flags()
	judged := reviewFlags.semantic || reviewFlags.estimate
	for _, name := range []string{"model", "models", "max-cost", "max-calls"} {
		if fl.Changed(name) && !judged {
			return oops.Errorf("--%s applies to --semantic or --estimate only", name)
		}
	}
	for _, name := range []string{"k", "gate", "gate-level", "no-cache", "cache-dir", "concurrency"} {
		if fl.Changed(name) && !reviewFlags.semantic {
			return oops.Errorf("--%s applies to --semantic only", name)
		}
	}
	if fl.Changed("gate-level") && !reviewFlags.gate {
		return oops.Errorf("--gate-level needs --gate")
	}
	return nil
}

// reviewContext is everything a review command loads: the configuration, the rubric, the
// lint evidence and the items.
type reviewContext struct {
	cfg *config.Config
	rb  *rv.Rubric
	in  rv.Input
}

// loadReview loads the config, the rubric and the lint evidence of a review, and narrows the items
// by --role, --profile and --since.
func loadReview(cmd *cobra.Command, args []string) (*reviewContext, error) {
	ctx := commandContext(cmd)
	cfg, err := loadConfigForCommand(ctx, nil)
	if err != nil {
		return nil, err
	}
	if problems := cfg.Review.Validate(); len(problems) > 0 {
		return nil, oops.Errorf("invalid [review] settings: %s", strings.Join(problems, "; "))
	}
	ref := reviewFlags.rubric
	if ref == "" && cfg.Review != nil {
		ref = cfg.Review.Rubric
	}
	rb, err := rv.Load(cfg.ConfigDir, ref)
	if err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	tree, err := lint.LoadTree(cfg.BaseDir)
	if err != nil {
		return nil, oops.Wrapf(err, "index repository files")
	}
	lrep, err := lint.RunWith(cfg, tree, lint.Options{Cwd: workingDir()})
	if err != nil {
		return nil, oops.Wrapf(err, "collect lint evidence")
	}
	view, err := reviewConfigView(cfg)
	if err != nil {
		return nil, err
	}
	items := rv.Collect(view, tree.Rel)
	if missing := rv.Unmatched(items, args); len(missing) > 0 {
		return nil, oops.Errorf("no item matches %s (use an id such as skill:name, a name or a path)", strings.Join(missing, ", "))
	}
	only, err := reviewSince(cfg, items)
	if err != nil {
		return nil, err
	}
	in := rv.Input{
		Rubric: rb, Items: items, Selector: args, Findings: lrep.Findings, Config: cfg.Review, Only: only,
		IncludeImports: reviewFlags.includeImports, Content: reviewFlags.content,
	}
	return &reviewContext{cfg: cfg, rb: rb, in: in}, nil
}

// reviewContent is the effective content mode.
func (rc *reviewContext) content() string { return rc.in.ContentMode() }

// runReview runs the review and writes the report. The exit status is 0, exitReviewRefused (the
// review or the estimate is refused) or exitReviewGate.
func runReview(cmd *cobra.Command, args []string, out io.Writer) (exit int, err error) {
	if err := validateReviewFlags(cmd); err != nil {
		return 0, err
	}
	rc, err := loadReview(cmd, args)
	if err != nil {
		return 0, err
	}
	res := rv.Run(rc.in)
	if reviewFlags.semantic && !reviewFlags.estimate {
		return runSemantic(cmd, rc, res, out)
	}
	var est *rv.Estimate
	if reviewFlags.estimate {
		est, err = planReviewEstimate(cmd, rc, res)
		if err != nil {
			return 0, err
		}
		if len(est.Refused) > 0 {
			exit = exitReviewRefused
		}
	}
	if err := applyBaseline(res, rc.rb); err != nil {
		return 0, err
	}
	report := rv.NewReport(rc.rb, res, est)
	if err := finishBaseline(report, res, rc.rb); err != nil {
		return 0, err
	}
	return exit, writeReview(out, report, rc.rb, res)
}

// reviewCaps resolves the spend ceilings: a flag, else user scope, else the lower of the
// repository value and the default.
func reviewCaps(cmd *cobra.Command, cfg *config.Config, defCost float64, defCalls int) (cost float64, calls int, res config.ReviewResolution, err error) {
	res, err = cfg.ResolveReview(nil)
	if err != nil {
		return 0, 0, res, err //nolint:wrapcheck // already contextual
	}
	cost, calls = res.Caps(cfg.Review, defCost, defCalls)
	if cmd.Flags().Changed("max-cost") {
		cost = reviewFlags.maxCost
	}
	if cmd.Flags().Changed("max-calls") {
		calls = reviewFlags.maxCalls
	}
	return cost, calls, res, nil
}

// withModel returns lc with the model replaced. A bare model keeps the configured provider; a
// provider-prefixed one replaces it, so the user names the route explicitly.
func withModel(lc llm.Config, model string) llm.Config {
	if model == "" {
		return lc
	}
	lc.Model = model
	if strings.Contains(model, "/") {
		lc.Provider = ""
	}
	return lc
}

// hostOf names where a call would go.
func hostOf(lc llm.Config, configDirPath string) string {
	host := llm.Diagnose(lc, llm.Options{ConfigDir: configDirPath}).BaseURLHost
	switch {
	case host != "":
	case lc.FullModel() == "" && lc.BaseURL == "":
		host = "(none: no [llm] configured)"
	default:
		host = "(provider default)"
	}
	return host
}

func planReviewEstimate(cmd *cobra.Command, rc *reviewContext, res *rv.Results) (*rv.Estimate, error) {
	resolved, err := rc.cfg.ResolveLLM(nil)
	if err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	lc := withModel(resolved.Config, reviewModelFlag())
	maxCost, maxCalls, _, err := reviewCaps(cmd, rc.cfg, rv.DefaultMaxCostUSD, rv.DefaultMaxCalls)
	if err != nil {
		return nil, err
	}
	return planWith(rc, res, lc, resolved, maxCost, maxCalls, reviewFlags.showPrompt), nil
}

// reviewModelFlag is the model --model or the first of --models names ("" when neither is set).
func reviewModelFlag() string {
	if reviewFlags.models != "" {
		return reviewModelList()[0]
	}
	return reviewFlags.model
}

// reviewModelList splits --models.
func reviewModelList() []string {
	var out []string
	for _, m := range strings.Split(reviewFlags.models, ",") {
		if m = strings.TrimSpace(m); m != "" {
			out = append(out, m)
		}
	}
	return out
}

// planWith builds the estimate of a judged run on lc.
func planWith(rc *reviewContext, res *rv.Results, lc llm.Config, resolved config.LLMResolution, maxCost float64, maxCalls int, showPrompt bool) *rv.Estimate {
	model := lc.FullModel()
	var prices rv.Prices
	if model != "" {
		pricing := llm.NewPricing(lc)
		prices = func(prompt, completion int) (float64, bool) {
			return pricing.Cost(model, llm.Usage{PromptTokens: prompt, CompletionTokens: completion})
		}
	}
	return rv.Plan(rv.EstimateInput{
		Rubric: rc.rb, Results: res, Content: rc.content(), Model: model, Host: hostOf(lc, rc.cfg.ConfigDir),
		NetworkAllowed: lc.AllowNetwork, IgnoredLLMKeys: resolved.Ignored, PolicyForbidsLLM: config.PolicyLocksIn(activePolicy, "llm", rc.cfg.BaseDir),
		Prices: prices, MaxCostUSD: maxCost, MaxCalls: maxCalls, ShowPrompt: showPrompt,
	})
}

// writeReview renders the report in the chosen format to --out or out.
func writeReview(out io.Writer, report *rv.Report, rb *rv.Rubric, res *rv.Results) error {
	w, closeFn, err := reportDestination(out, reviewFlags.out)
	if err != nil {
		return err
	}
	var werr error
	switch reviewFlags.format {
	case rv.FormatJSON:
		werr = report.WriteJSON(w)
	case rv.FormatSARIF:
		werr = rv.WriteSARIF(w, rb, res, Version)
	default:
		werr = report.WriteText(w)
	}
	if cerr := closeFn(); werr == nil {
		werr = cerr
	}
	return werr
}

// reportDestination opens --out, or returns out.
func reportDestination(out io.Writer, path string) (io.Writer, func() error, error) {
	if path == "" {
		return out, func() error { return nil }, nil
	}
	f, err := os.Create(path) //nolint:gosec // the user names the report file
	if err != nil {
		return nil, nil, oops.Wrapf(err, "create %s", path)
	}
	return f, func() error { return oops.Wrapf(f.Close(), "close %s", path) }, nil
}
func runRubricList(cmd *cobra.Command, out io.Writer) error {
	cfg, err := loadConfigForCommand(commandContext(cmd), nil)
	if err != nil {
		return err
	}
	type row struct {
		Ref     string `json:"ref"`
		Version int    `json:"version,omitempty"`
		Digest  string `json:"digest,omitempty"`
		Valid   bool   `json:"valid"`
	}
	var rows []row
	for _, id := range rv.BuiltinIDs() {
		rb, lerr := rv.LoadBuiltin(id)
		if lerr != nil {
			return lerr //nolint:wrapcheck // already contextual
		}
		rows = append(rows, row{Ref: rb.Ref, Version: rb.Version, Digest: rb.Digest, Valid: true})
	}
	ids, err := rv.ListIDs(cfg.ConfigDir)
	if err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	for _, id := range ids {
		rb, lerr := rv.Load(cfg.ConfigDir, id)
		if lerr != nil {
			rows = append(rows, row{Ref: id})
			continue
		}
		rows = append(rows, row{Ref: id, Version: rb.Version, Digest: rb.Digest, Valid: true})
	}
	if rubricFormat == formatJSON {
		return writeIndentedJSON(out, rows)
	}
	w := reportWriter{out}
	for _, r := range rows {
		if r.Valid {
			w.printf("%-28s version %d  %s\n", r.Ref, r.Version, r.Digest)
		} else {
			w.printf("%-28s invalid (run: ai-rulez rubric lint %s)\n", r.Ref, r.Ref)
		}
	}
	return nil
}

func runRubricShow(cmd *cobra.Command, args []string, out io.Writer) error {
	cfg, err := loadConfigForCommand(commandContext(cmd), nil)
	if err != nil {
		return err
	}
	ref := ""
	if len(args) > 0 {
		ref = args[0]
	} else if cfg.Review != nil {
		ref = cfg.Review.Rubric
	}
	rb, err := rv.Load(cfg.ConfigDir, ref)
	if err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	if rubricFormat == formatJSON {
		return writeIndentedJSON(out, rv.NewReport(rb, &rv.Results{}, nil).Rubric)
	}
	w := reportWriter{out}
	w.printf("%s  version %d  %s\napplies to: %s\n%s\n\n", rb.Ref, rb.Version, rb.Digest, strings.Join(rb.AppliesTo, ", "), rv.Formula)
	for i := range rb.Dimensions {
		d := &rb.Dimensions[i]
		w.printf("%-22s %-6s weight %.2f  %-12s ceiling %-7s twins: %s\n  %s\n", d.ID, d.Code, d.Weight, d.Group, d.Severity, orDash(strings.Join(d.Twins, ", ")), d.Question)
	}
	return nil
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func writeIndentedJSON(out io.Writer, v any) error {
	return oops.Wrapf(jsondoc.Write(out, v), "write json")
}

// runRubricLint lints the named rubrics, or every project rubric. found is true when any finding exists.
func runRubricLint(cmd *cobra.Command, args []string, out io.Writer) (found bool, err error) {
	if !contains(rv.Formats(), rubricFormat) {
		return false, oops.Errorf("unknown --format %q", rubricFormat)
	}
	cfg, err := loadConfigForCommand(commandContext(cmd), nil)
	if err != nil {
		return false, err
	}
	refs := args
	if len(refs) == 0 {
		if refs, err = rv.ListIDs(cfg.ConfigDir); err != nil {
			return false, err //nolint:wrapcheck // already contextual
		}
	}
	var problems []rv.Problem
	for _, ref := range refs {
		var ps []rv.Problem
		if id, ok := strings.CutPrefix(ref, config.BuiltinRubricPrefix); ok {
			ps, err = rv.LintBuiltin(id)
		} else {
			ps, err = rv.LintDir(filepath.Join(cfg.ConfigDir, rv.RubricsDir, ref))
		}
		if err != nil {
			return false, oops.Wrapf(err, "lint rubric %s", ref)
		}
		problems = append(problems, ps...)
	}
	sort.SliceStable(problems, func(i, j int) bool { return problems[i].File < problems[j].File })
	rep := &lint.Report{Root: cfg.BaseDir, Findings: rv.Findings(problems, workingDir())}
	if len(refs) == 0 && rubricFormat == formatText {
		reportWriter{out}.printf("no rubrics under %s\n", filepath.Join(cfg.ConfigDir, rv.RubricsDir))
		return false, nil
	}
	if err := lint.Write(out, rubricFormat, lint.Combine([]*lint.Report{rep}), lint.WriteOptions{Version: Version, FailOn: failOnError}); err != nil {
		return false, oops.Wrapf(err, "write report")
	}
	return len(problems) > 0, nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
