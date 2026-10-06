package commands

import (
	"encoding/json"

	"github.com/Goldziher/ai-rulez/v5/internal/evals"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

var evalCalibrateFlags struct {
	results    string
	harness    string
	model      string
	minSamples int
	format     string
}

var evalCalibrateCmd = &cobra.Command{
	Use:   "calibrate-estimate",
	Short: "Propose estimate assumptions measured from recorded eval runs",
	Long: `Read the eval results file and propose the assumptions of the cost estimate
([lint.evals.estimate]: overhead_tokens, assumed_output_tokens, activation_output_tokens) that the
recorded runs measured: the harness overhead moves by the median gap between the input tokens the runner
reported and the ones the estimate expected, per agent run, and the output assumption likewise. Runs of
different harnesses and models, and case runs and activation runs, are never mixed. It prints the
proposal and the median token error before and after; it changes nothing, reads only records signed with
your key, and sends nothing anywhere. Put the printed [lint.evals.estimate] table in your configuration to
apply it. See docs/evals.md.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error { return runEvalCalibrate(cmd) },
}

func init() {
	f := evalCalibrateCmd.Flags()
	f.StringVar(&evalCalibrateFlags.results, "results", "", "Results file (default <config dir>/eval-results.json)")
	f.StringVar(&evalCalibrateFlags.harness, "harness", "", "Only runs of this harness")
	f.StringVar(&evalCalibrateFlags.model, "model", "", "Only runs of this model")
	f.IntVar(&evalCalibrateFlags.minSamples, "min-samples", evals.DefaultMinSamples, "Runs a group needs before its proposal is not marked low-confidence")
	addFormatFlag(f, &evalCalibrateFlags.format, formatText, formatText, formatText, formatJSON)
	addJSONFlagAlias(f)
	f.StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
	EvalCmd.AddCommand(evalCalibrateCmd)
}

func runEvalCalibrate(cmd *cobra.Command) error {
	flags := &evalCalibrateFlags
	if flags.minSamples < 0 {
		return oops.Errorf("--min-samples must be >= 0, got %d", flags.minSamples)
	}
	if flags.format != formatText && flags.format != formatJSON {
		return oops.Errorf("unknown --format %q (use text or json)", flags.format)
	}
	cfg, err := loadConfigForCommand(commandContext(cmd), nil)
	if err != nil {
		return err
	}
	path := flags.results
	if path == "" {
		path = evals.DefaultStorePath(cfg.ConfigDir)
	}
	store, err := evals.LoadStoreKeyed(path, evals.ExistingUserKey())
	if err != nil {
		return oops.Wrapf(err, "read eval results")
	}
	cal := evals.Calibrate(store, &evals.CalibrateOptions{Harness: flags.harness, Model: flags.model, MinSamples: flags.minSamples})
	if flags.format == formatJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return oops.Wrapf(enc.Encode(cal), "write calibration")
	}
	return oops.Wrapf(cal.WriteText(cmd.OutOrStdout()), "write calibration")
}
