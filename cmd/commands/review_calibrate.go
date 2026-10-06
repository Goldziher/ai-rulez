package commands

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/llm"
	rv "github.com/Goldziher/ai-rulez/v5/internal/review"
)

// Defaults of a calibration run: it asks every vote of every golden case, so it costs more than
// a review. They bound it like any run and can be raised with --max-cost and --max-calls.
const (
	defaultCalibrateMaxCostUSD = 2.00
	defaultCalibrateMaxCalls   = 1000
)

var calibrateFlags struct {
	rubric   string
	golden   string
	model    string
	models   string
	k        int
	content  string
	maxCost  float64
	maxCalls int
	noCache  bool
	noProbes bool
	workers  int
	noWrite  bool
	compare  string
	out      string
	format   string
}

var reviewCalibrateCmd = &cobra.Command{
	Use:   "calibrate",
	Short: "Measure the judge against a rubric's golden set (needed before gating)",
	Long: `Judge every case of the rubric's golden set the way "review --semantic" does (all k votes are
asked, so the judge's consistency can be measured), compare the verdicts with the adjudicated
human labels, and run the metamorphic probes on the cases that list them. Per dimension it reports
quadratic-weighted kappa against the labels, precision and recall of "flagged" with Wilson 95%
intervals, the labelers' own agreement (below the rubric's threshold the dimension is "ill-defined":
fix the rubric, not the model), the consistency of the k votes (identical share and Fleiss' kappa),
the pad, reorder, rename and canary probes, and the calibration curve (vote agreement against
measured precision).

The record (calibration.json in the rubric directory; <config dir>/calibration/<id>.builtin.json
for a built-in rubric) binds the result to the rubric digest, the prompt digest, the golden set, the
resolved model id and the content mode. "review --semantic --gate" is refused unless a passing record
matches the run exactly and is younger than the rubric's max_age_days.

  --golden DIR     use the golden cases under DIR (DIR/golden/*.golden.yaml or DIR/*.golden.yaml;
                   paths in a case are relative to DIR); the default is the rubric directory
  --models A,B     calibrate each model and compare them: pairwise kappa, and where they disagree
                   (nothing is written)
  --compare FILE   drift check: calibrate, compare with the record in FILE, print every golden
                   verdict that changed, and exit 2 when a kappa fell by more than 0.05 or the
                   judge no longer meets its thresholds (nothing is written)

It needs the same opt-in as --semantic ([llm] allow_network = true in user scope, a model) and
is bounded by --max-cost (default 2.00 USD) and --max-calls (default 1000). Exit status: 0 when the
judge passes, 2 when it does not (or drifted), 1 when it could not run.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		code, err := runCalibrate(cmd, cmd.OutOrStdout())
		if err != nil {
			return err
		}
		if code != 0 {
			os.Exit(code)
		}
		return nil
	},
}

func init() {
	f := reviewCalibrateCmd.Flags()
	f.StringVar(&calibrateFlags.rubric, "rubric", "", "Rubric id (default [review] rubric, else builtin:skill-quality)")
	f.StringVar(&calibrateFlags.golden, "golden", "", "Directory with the golden cases (default the rubric directory)")
	f.StringVar(&calibrateFlags.model, "model", "", "Model to calibrate (default [llm] model)")
	f.StringVar(&calibrateFlags.models, "models", "", "Comma-separated models to calibrate and compare; writes no record")
	f.IntVar(&calibrateFlags.k, "k", 0, "Votes per dimension (default the rubric's votes.max)")
	f.StringVar(&calibrateFlags.content, "content", config.ReviewContentFull, "Content the judge is calibrated with: full or descriptions (the record binds it)")
	f.Float64Var(&calibrateFlags.maxCost, "max-cost", 0, "Spend cap in USD (default 2.00; 0 = unlimited)")
	f.IntVar(&calibrateFlags.maxCalls, "max-calls", 0, "Call cap (default 1000)")
	f.BoolVar(&calibrateFlags.noCache, "no-cache", false, "Do not read or write the response cache (variance runs)")
	f.IntVar(&calibrateFlags.workers, "concurrency", 0, "Cases judged at once (default 4, at most 16)")
	f.BoolVar(&calibrateFlags.noProbes, "no-probes", false, "Skip the metamorphic probes")
	f.BoolVar(&calibrateFlags.noWrite, "no-write", false, "Do not write the record")
	f.StringVar(&calibrateFlags.compare, "compare", "", "Compare with this calibration record and fail on drift (writes nothing)")
	f.StringVar(&calibrateFlags.out, "out", "", "Write the record here instead of the rubric's calibration.json")
	addFormatFlag(f, &calibrateFlags.format, formatText, formatText, formatText, formatJSON)
	addJSONFlagAlias(f)
	f.StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
	f.BoolVar(&noLocal, "no-local", false, "Ignore the machine-local config.local.* overlay and local/ content")
}

// calibrationOutput is the JSON of a calibration run.
type calibrationOutput struct {
	Record      *rv.CalibrationRecord `json:"record,omitempty"`
	Usage       rv.RunUsage           `json:"usage"`
	Differences []rv.CaseDiff         `json:"differences,omitempty"`
	Drift       *rv.Drift             `json:"drift,omitempty"`
	Comparison  *rv.ModelComparison   `json:"comparison,omitempty"`
	RecordFile  string                `json:"record_file,omitempty"`
	Errors      []calibrationError    `json:"errors,omitempty"`
	Incomplete  bool                  `json:"incomplete,omitempty"`
	Stopped     string                `json:"stopped_because,omitempty"`
}

// calibrationError is a golden case the judge could not answer for a dimension.
type calibrationError struct {
	Case      string `json:"case"`
	Dimension string `json:"dimension"`
	Note      string `json:"note"`
}

func runCalibrate(cmd *cobra.Command, out io.Writer) (int, error) {
	if calibrateFlags.workers < 0 || calibrateFlags.workers > maxConcurrency {
		return 0, oops.Errorf("--concurrency must be between 1 and %d", maxConcurrency)
	}
	if calibrateFlags.maxCost < 0 || calibrateFlags.maxCalls < 0 || calibrateFlags.k < 0 {
		return 0, oops.Errorf("--max-cost, --max-calls and --k must not be negative")
	}
	if calibrateFlags.models != "" && (calibrateFlags.compare != "" || cmd.Flags().Changed("model")) {
		return 0, oops.Errorf("--models cannot be combined with --model or --compare")
	}
	if calibrateFlags.content != config.ReviewContentFull && calibrateFlags.content != config.ReviewContentDescriptions {
		return 0, oops.Errorf("unknown --content %q (use full or descriptions)", calibrateFlags.content)
	}
	cfg, err := loadConfigForCommand(commandContext(cmd), nil)
	if err != nil {
		return 0, err
	}
	if problems := cfg.Review.Validate(); len(problems) > 0 {
		return 0, oops.Errorf("invalid [review] settings: %s", strings.Join(problems, "; "))
	}
	ref := calibrateFlags.rubric
	if ref == "" && cfg.Review != nil {
		ref = cfg.Review.Rubric
	}
	rb, err := rv.Load(cfg.ConfigDir, ref)
	if err != nil {
		return 0, err //nolint:wrapcheck // already contextual
	}
	base := calibrateFlags.golden
	if base == "" {
		if rb.Dir == "" {
			return 0, oops.Hint("pass --golden DIR with the golden cases").Errorf("the built-in rubric %s has no golden set of its own", rb.Ref)
		}
		base = rb.Dir
	}
	set, err := rv.LoadGolden(base, rb)
	if err != nil {
		return 0, err //nolint:wrapcheck // already contextual
	}
	if len(set.Cases) == 0 {
		return 0, oops.Errorf("no golden cases under %s", base)
	}

	models := []string{calibrateFlags.model}
	if calibrateFlags.models != "" {
		models = nil
		for _, m := range strings.Split(calibrateFlags.models, ",") {
			if m = strings.TrimSpace(m); m != "" {
				models = append(models, m)
			}
		}
	}
	// The spend caps come from the flags here, not from reviewFlags.
	defCost, defCalls := defaultCalibrateMaxCostUSD, defaultCalibrateMaxCalls
	var reports []*rv.CalibrationReport
	var names []string
	for _, m := range models {
		js, rerr := resolveCalibrateJudge(cmd, cfg, m, defCost, defCalls, len(models))
		if rerr != nil {
			return 0, rerr
		}
		if err := js.ready(cfg); err != nil {
			return 0, err
		}
		client, cerr := reviewClientFactory(js.lc, llm.Options{ConfigDir: cfg.ConfigDir, NoCache: calibrateFlags.noCache})
		if cerr != nil {
			return 0, cerr
		}
		rep, runErr := rv.Calibrate(commandContext(cmd), rv.CalibrateInput{
			Rubric: rb, Golden: set, Now: reviewNow(), NoProbes: calibrateFlags.noProbes,
			Options: rv.SemanticOptions{Client: client, K: calibrateFlags.k, Content: calibrateFlags.content, Workers: calibrateFlags.workers},
		})
		_ = client.Close() //nolint:errcheck // nothing to flush
		if runErr != nil {
			return 0, oops.Wrapf(runErr, "calibration of %s stopped", js.lc.FullModel())
		}
		if rep.Incomplete {
			return exitReviewRefused, oops.Errorf("calibration of %s is incomplete: %s; raise --max-cost or --max-calls", js.lc.FullModel(), rep.StoppedBecause)
		}
		reports = append(reports, rep)
		names = append(names, js.lc.FullModel())
	}

	result := calibrationOutput{Usage: reports[0].Usage}
	for _, r := range reports[1:] {
		result.Usage.Calls += r.Usage.Calls
		result.Usage.Cached += r.Usage.Cached
		result.Usage.Tokens += r.Usage.Tokens
		result.Usage.CostUSD += r.Usage.CostUSD
	}
	exit := 0
	if len(reports) == 1 {
		rep := reports[0]
		result.Record, result.Differences = rep.Record, rep.Diffs
		for _, c := range rep.Cases {
			for _, d := range sortedStrings(c.Errors) {
				result.Errors = append(result.Errors, calibrationError{Case: c.ID, Dimension: d, Note: c.Errors[d]})
			}
		}
		switch {
		case calibrateFlags.compare != "":
			old, lerr := rv.LoadCalibration(calibrateFlags.compare)
			if lerr != nil {
				return 0, lerr //nolint:wrapcheck // already contextual
			}
			if old == nil {
				return 0, oops.Errorf("no calibration record at %s", calibrateFlags.compare)
			}
			drift := rv.CompareCalibration(old, rep.Record)
			result.Drift = &drift
			if drift.Failed {
				exit = exitReviewGate
			}
		case !calibrateFlags.noWrite:
			path := calibrateFlags.out
			if path == "" {
				path = rv.CalibrationPath(cfg.ConfigDir, rb)
			}
			if err := rv.SaveCalibration(path, rep.Record); err != nil {
				return 0, err //nolint:wrapcheck // already contextual
			}
			result.RecordFile = path
		}
		if rep.Record.Status != "pass" && exit == 0 {
			exit = exitReviewGate
		}
	} else {
		tables := make([]map[string]map[string]string, len(reports))
		for i, r := range reports {
			tables[i] = map[string]map[string]string{}
			for _, c := range r.Cases {
				tables[i][c.ID] = c.Verdicts
			}
		}
		dims := make([]string, 0, len(rb.Dimensions))
		for _, d := range rb.Dimensions {
			dims = append(dims, d.ID)
		}
		result.Comparison = rv.CompareModels(names, tables, dims)
		result.Record = reports[0].Record
		for _, r := range reports {
			if r.Record.Status != "pass" {
				exit = exitReviewGate
			}
		}
	}
	if calibrateFlags.format == formatJSON {
		return exit, writeIndentedJSON(out, result)
	}
	writeCalibrationText(out, rb, set, names, reports, result)
	return exit, nil
}

// resolveCalibrateJudge is resolveJudge with the calibration caps: --max-cost and --max-calls,
// else the calibration defaults, split between the models being compared.
func resolveCalibrateJudge(cmd *cobra.Command, cfg *config.Config, model string, defCost float64, defCalls, models int) (*judgeSetup, error) {
	resolved, err := cfg.ResolveLLM(nil)
	if err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	rres, err := cfg.ResolveReview(nil)
	if err != nil {
		return nil, err //nolint:wrapcheck // already contextual
	}
	cost, calls := rres.Caps(nil, defCost, defCalls)
	if cmd.Flags().Changed("max-cost") {
		cost = calibrateFlags.maxCost
	}
	if cmd.Flags().Changed("max-calls") {
		calls = calibrateFlags.maxCalls
	}
	if models > 1 {
		cost /= float64(models)
		calls = max(calls/models, 1)
	}
	lc := withModel(resolved.Config, model)
	lc.MaxCostUSD = tighterFloat(lc.MaxCostUSD, cost)
	lc.MaxCalls = tighterInt(lc.MaxCalls, calls)
	return &judgeSetup{lc: lc, resolved: resolved, review: rres, maxCost: cost, maxCalls: calls}, nil
}

func writeCalibrationText(out io.Writer, rb *rv.Rubric, set *rv.GoldenSet, names []string, reports []*rv.CalibrationReport, res calibrationOutput) {
	w := reportWriter{out}
	for i, rep := range reports {
		rec := rep.Record
		w.printf("rubric %s@%d  %d golden cases  model %s (answered as %s)  k=%d  content %s\n", rb.ID, rb.Version, len(set.Cases), names[i], rec.Model, rec.K, rec.Content)
		w.printf("%-22s %4s %6s %6s %6s %6s %7s %7s %7s  %s\n", "dimension", "n", "kappa", "prec", "recall", "f1", "consist", "fleiss", "human", "status")
		for _, d := range rb.Dimensions {
			dc := rec.Dimensions[d.ID]
			w.printf("%-22s %4d %6.2f %6.2f %6.2f %6.2f %7.2f %7.2f %7.2f  %s\n", d.ID, dc.N, dc.Kappa, dc.Precision, dc.Recall, dc.F1, dc.Consistency, dc.FleissKappa, dc.HumanKappa, dc.Status)
			if len(dc.Metamorphic) > 0 {
				var parts []string
				for _, p := range []string{rv.ProbePad, rv.ProbeReorder, rv.ProbeRename, rv.ProbeCanary} {
					if v, ok := dc.Metamorphic[p]; ok {
						parts = append(parts, fmt.Sprintf("%s %.2f", p, v))
					}
				}
				w.printf("%-22s probes: %s\n", "", strings.Join(parts, "  "))
			}
			for _, m := range dc.Misses {
				w.printf("%-22s miss: %s\n", "", m)
			}
			if len(dc.Curve) > 0 {
				var parts []string
				for _, c := range dc.Curve {
					parts = append(parts, fmt.Sprintf("agree %.2f -> precision %.2f (n=%d)", c.Agreement, c.Precision, c.N))
				}
				w.printf("%-22s curve: %s\n", "", strings.Join(parts, "; "))
			}
		}
		w.printf("calibration: %s\n", rec.Status)
		if len(rep.Diffs) > 0 {
			w.printf("%d verdict(s) differ from the adjudicated labels:\n", len(rep.Diffs))
			for _, d := range rep.Diffs {
				w.printf("  %-28s %-20s label %-4s judge %-4s agree %.2f\n", d.Case, d.Dimension, d.Label, d.Got, d.Agreement)
			}
		}
	}
	if res.RecordFile != "" {
		w.printf("record written to %s (run `ai-rulez lock` to pin it)\n", res.RecordFile)
	}
	if d := res.Drift; d != nil {
		if d.Failed {
			w.printf("DRIFT:\n")
		} else {
			w.printf("no drift against %s\n", calibrateFlags.compare)
		}
		for _, r := range d.Regressions {
			w.printf("  %s\n", r)
		}
		for _, c := range d.Changed {
			w.printf("  changed: %-28s %-20s %s -> %s\n", c.Case, c.Dimension, c.Was, c.Now)
		}
	}
	if m := res.Comparison; m != nil {
		sb := &strings.Builder{}
		rv.WriteModelsText(sb, m)
		w.printf("%s", sb.String())
	}
	if n := len(res.Errors); n > 0 {
		w.printf("%d dimension(s) could not be answered, for example %s %s: %s\n", n, res.Errors[0].Case, res.Errors[0].Dimension, res.Errors[0].Note)
	}
	w.printf("calls %d, cached %d, tokens %d, cost $%.4f, %d quote(s) dropped\n", res.Usage.Calls, res.Usage.Cached, res.Usage.Tokens, res.Usage.CostUSD, res.Usage.Hallucinated)
}

func sortedStrings(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
