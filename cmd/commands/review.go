package commands

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
	rv "github.com/Goldziher/ai-rulez/v5/internal/review"
)

// exitReviewRefused is the exit status of a review whose estimate a real run
// would refuse (over a cap, or no price for the model).
const exitReviewRefused = 1

// exitRubricFindings is the exit status of `rubric lint` with findings, matching `validate --strict`.
const exitRubricFindings = 2

var reviewFlags struct {
	rubric         string
	content        string
	estimate       bool
	showPrompt     bool
	model          string
	maxCost        float64
	maxCalls       int
	includeImports bool
	format         string
	out            string
}

// ReviewCmd scores skills, agents, commands and rules against a rubric.
var ReviewCmd = &cobra.Command{
	Use:   "review [id|name|path...]",
	Short: "Score skills, agents, commands and rules against a rubric (offline)",
	Long: `Score content against a user-owned, versioned rubric (default builtin:skill-quality).

This build is offline: the score is computed from the lint findings each rubric dimension
names as its twins, and no model is ever called. Dimensions with no twin are listed as
not scored. The weights and the scoring formula are printed in every JSON report.

  --estimate     print the egress manifest of a later model-judged run: which items would
                 send what (sizes and a hash, never content), to which host, estimated
                 tokens and cost from the price table, and whether a real run would be
                 refused by the spend caps. Nothing is sent. --show-prompt adds the
                 exact planned messages.

An item with a secret or hidden-character finding (AR001, AR002) is withheld: it is never
part of the manifest. Content from includes and installed skills is skipped unless
--include-imports. Rubrics live in .ai-rulez/rubrics/<id>/ (see "ai-rulez rubric --help").

Exit status: 0 on success; 1 when the review could not run or --estimate shows that a real
run would be refused. See docs/review.md.`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		refused, err := runReview(cmd, args, cmd.OutOrStdout())
		if err != nil {
			return err
		}
		if refused {
			os.Exit(exitReviewRefused)
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
	f.StringVar(&reviewFlags.content, "content", "", "What a judge would receive: descriptions (name, description, frontmatter keys) or full (adds the body); default [review] content")
	f.BoolVar(&reviewFlags.estimate, "estimate", false, "Print the egress manifest and cost range of a model-judged run; send nothing")
	f.BoolVar(&reviewFlags.showPrompt, "show-prompt", false, "With --estimate, print the exact planned messages")
	f.StringVar(&reviewFlags.model, "model", "", "Model the estimate prices (default [llm] model)")
	f.Float64Var(&reviewFlags.maxCost, "max-cost", 0, "Spend cap in USD the estimate is checked against (default [review] max_cost_usd, else 0.50; 0 = unlimited)")
	f.IntVar(&reviewFlags.maxCalls, "max-calls", 0, "Call cap the estimate is checked against (default [review] max_calls, else 300)")
	f.BoolVar(&reviewFlags.includeImports, "include-imports", false, "Also review content from includes, installed skills and builtins")
	addFormatFlag(f, &reviewFlags.format, formatText, formatText, rv.Formats()...)
	addJSONFlagAlias(f)
	f.StringVar(&reviewFlags.out, "out", "", "Write the report to this file instead of standard output")
	f.BoolVar(&noLocal, "no-local", false, "Ignore the machine-local config.local.* overlay and local/ content")
	f.StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")

	addFormatFlag(RubricCmd.PersistentFlags(), &rubricFormat, formatText, formatText, rv.Formats()...)
	addJSONFlagAlias(RubricCmd.PersistentFlags())
	RubricCmd.PersistentFlags().StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
	RubricCmd.AddCommand(rubricListCmd, rubricShowCmd, rubricLintCmd)
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
	if reviewFlags.maxCost < 0 || reviewFlags.maxCalls < 0 {
		return oops.Errorf("--max-cost and --max-calls must not be negative")
	}
	if !reviewFlags.estimate {
		for _, name := range []string{"model", "max-cost", "max-calls"} {
			if cmd.Flags().Changed(name) {
				return oops.Errorf("--%s applies to --estimate only", name)
			}
		}
	}
	return nil
}

// reviewInputs loads the config, the rubric and the lint evidence of a review.
func reviewInputs(cmd *cobra.Command, args []string) (cfg *config.Config, rb *rv.Rubric, in rv.Input, tree *lint.Tree, err error) {
	ctx := commandContext(cmd)
	cfg, err = loadConfigForCommand(ctx, nil)
	if err != nil {
		return nil, nil, in, nil, err
	}
	if problems := cfg.Review.Validate(); len(problems) > 0 {
		return nil, nil, in, nil, oops.Errorf("invalid [review] settings: %s", strings.Join(problems, "; "))
	}
	ref := reviewFlags.rubric
	if ref == "" && cfg.Review != nil {
		ref = cfg.Review.Rubric
	}
	rb, err = rv.Load(cfg.ConfigDir, ref)
	if err != nil {
		return nil, nil, in, nil, err //nolint:wrapcheck // already contextual
	}
	tree, err = lint.LoadTree(cfg.BaseDir)
	if err != nil {
		return nil, nil, in, nil, oops.Wrapf(err, "index repository files")
	}
	lrep, err := lint.RunWith(cfg, tree, lint.Options{Cwd: workingDir()})
	if err != nil {
		return nil, nil, in, nil, oops.Wrapf(err, "collect lint evidence")
	}
	items := rv.Collect(cfg, tree.Rel)
	if missing := rv.Unmatched(items, args); len(missing) > 0 {
		return nil, nil, in, nil, oops.Errorf("no item matches %s (use an id such as skill:name, a name or a path)", strings.Join(missing, ", "))
	}
	in = rv.Input{
		Rubric: rb, Items: items, Selector: args, Findings: lrep.Findings, Config: cfg.Review,
		IncludeImports: reviewFlags.includeImports, Content: reviewFlags.content,
	}
	return cfg, rb, in, tree, nil
}

// runReview runs the review and writes the report. refused is true when
// --estimate shows a real run would be refused.
func runReview(cmd *cobra.Command, args []string, out io.Writer) (refused bool, err error) {
	if err := validateReviewFlags(cmd); err != nil {
		return false, err
	}
	cfg, rb, in, _, err := reviewInputs(cmd, args)
	if err != nil {
		return false, err
	}
	res := rv.Run(in)
	var est *rv.Estimate
	if reviewFlags.estimate {
		est, err = planReviewEstimate(cmd, cfg, rb, res, in.ContentMode())
		if err != nil {
			return false, err
		}
		refused = len(est.Refused) > 0
	}
	report := rv.NewReport(rb, res, est)
	return refused, writeReview(out, report, rb, res)
}

func planReviewEstimate(cmd *cobra.Command, cfg *config.Config, rb *rv.Rubric, res *rv.Results, content string) (*rv.Estimate, error) {
	resolved, err := cfg.ResolveLLM(nil)
	if err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	lc := resolved.Config
	if reviewFlags.model != "" {
		lc.Model, lc.Provider = reviewFlags.model, ""
	}
	model := lc.FullModel()
	host := llm.Diagnose(lc, llm.Options{ConfigDir: cfg.ConfigDir}).BaseURLHost
	switch {
	case host != "":
	case model == "" && lc.BaseURL == "":
		host = "(none: no [llm] configured)"
	default:
		host = "(provider default)"
	}
	var prices rv.Prices
	if model != "" {
		pricing := llm.NewPricing(lc)
		prices = func(prompt, completion int) (float64, bool) {
			return pricing.Cost(model, llm.Usage{PromptTokens: prompt, CompletionTokens: completion})
		}
	}
	maxCost, maxCalls := rv.DefaultMaxCostUSD, rv.DefaultMaxCalls
	if cfg.Review != nil {
		if cfg.Review.MaxCostUSD > 0 {
			maxCost = cfg.Review.MaxCostUSD
		}
		if cfg.Review.MaxCalls > 0 {
			maxCalls = cfg.Review.MaxCalls
		}
	}
	if cmd.Flags().Changed("max-cost") {
		maxCost = reviewFlags.maxCost
	}
	if cmd.Flags().Changed("max-calls") {
		maxCalls = reviewFlags.maxCalls
	}
	return rv.Plan(rv.EstimateInput{
		Rubric: rb, Results: res, Content: content, Model: model, Host: host, NetworkAllowed: lc.AllowNetwork, IgnoredLLMKeys: resolved.Ignored,
		PolicyForbidsLLM: config.PolicyLocks("llm"),
		Prices:           prices, MaxCostUSD: maxCost, MaxCalls: maxCalls, ShowPrompt: reviewFlags.showPrompt,
	}), nil
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
	for _, d := range rb.Dimensions {
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
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return oops.Wrapf(enc.Encode(v), "write json")
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
	if err := lint.Write(out, rubricFormat, lint.Combine([]*lint.Report{rep}), lint.WriteOptions{Version: Version, FailOn: "error"}); err != nil {
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
