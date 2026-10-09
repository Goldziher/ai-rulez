package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/verifiers"
)

var (
	calibrateNoWrite bool
	calibrateJSON    bool
)

// VerifiersCalibrateCmd measures the llm verifiers against their labeled examples.
var VerifiersCalibrateCmd = &cobra.Command{
	Use:   "calibrate [name...]",
	Short: "Measure how reliable an llm verifier's failures are, so it may gate",
	Long: fmt.Sprintf(`Run the [[verifiers.examples]] of the llm verifiers (default: all of them) through
the model and record how often a "fail" verdict was right (precision) and how many real
failures it found (recall), with 95%% intervals. An example labeled expect = "fail" is a
true flag when the verifier fails on it. The record goes to
.ai-rulez/verifiers/calibration/<id>.json; commit it.

A verifier gates only with "verifiers run --gate-llm" and only while its record is current
(same checklist, examples, prompt and model) and passes the bar: precision of at least %.2f,
on at least %d examples with at least %d expected to fail and %d to pass, none unevaluated.
Without a current passing record every llm verdict stays advisory, capped at warning.

This sends the example files to the model, so it needs --allow-llm and allow_network = true
in the user config. --estimate prints what would be sent and the cost bound, and calls
nothing. Exit codes: 0 every record passes, 2 at least one does not (it is still written
unless --no-write), 1 it could not run.`,
		verifiers.CalibrationMinPrecision, verifiers.CalibrationMinExamples, verifiers.CalibrationMinPerClass, verifiers.CalibrationMinPerClass),
	RunE: func(cmd *cobra.Command, args []string) error {
		return calibrateVerifiers(watchParentContext(cmd), args, os.Stdout)
	},
}

func init() {
	VerifiersCmd.AddCommand(VerifiersCalibrateCmd)
	f := VerifiersCalibrateCmd.Flags()
	f.BoolVar(&verifiersAllowLLM, "allow-llm", false, "Send the example files to the configured model (needs allow_network in the user config)")
	f.Float64Var(&verifiersMaxCost, "max-cost", defaultVerifiersMaxCost, "Most the run may cost in USD (0 removes this cap; [llm] limits still apply)")
	f.BoolVar(&verifiersEstimate, "estimate", false, "Print what would be sent and the cost bound, and call nothing")
	f.BoolVar(&calibrateNoWrite, "no-write", false, "Print the measurement without writing the record")
	addJSONFormat(f, &calibrateJSON, "")
	f.BoolVar(&noLocal, "no-local", false, "Ignore the machine-local config.local.* overlay and local/ content")
	f.StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
}

// calibrateVerifiers runs `verifiers calibrate` and returns the exit code.
func calibrateVerifiers(ctx context.Context, names []string, out io.Writer) error {
	cfg, err := loadVerifierConfig(ctx, nil)
	if err != nil {
		return fail(err)
	}
	if calibrateNoWrite && verifiersEstimate {
		return fail(oops.Errorf("--no-write and --estimate cannot be combined: an estimate calls nothing, so there is nothing to write"))
	}
	opts, release, err := verifierLLMOptions(ctx, cfg)
	if err != nil {
		return fail(err)
	}
	defer release()
	rep, err := verifiers.Calibrate(ctx, cfg, verifiers.CalibrateOptions{Names: names, LLM: *opts, Now: cfg.Host.Now()})
	if err != nil {
		return fail(err)
	}
	failed := false
	for i := range rep.Records {
		rec := &rep.Records[i]
		failed = failed || rec.Status != verifiers.CalibrationPass
		if calibrateNoWrite {
			continue
		}
		if err := verifiers.SaveCalibration(cfg, rec); err != nil {
			return fail(err)
		}
	}
	if calibrateJSON {
		if err := writeJSON(out, rep); err != nil {
			return fail(err)
		}
	} else if _, err := io.WriteString(out, renderCalibration(rep, !calibrateNoWrite)); err != nil {
		return fail(oops.Wrapf(err, "write calibration report"))
	}
	if failed {
		return exitStatus(exitVerifiersFindings)
	}
	return nil
}

func renderCalibration(rep *verifiers.CalibrationReport, written bool) string {
	var b strings.Builder
	for _, line := range rep.Estimates {
		b.WriteString(line + "\n")
	}
	if len(rep.Estimates) == 0 && len(rep.Records) == 0 {
		b.WriteString("No llm verifier to calibrate.\n")
	}
	for i := range rep.Records {
		rec := &rep.Records[i]
		fmt.Fprintf(&b, "%s: %s on %d examples with %s (%s)\n", rec.Verifier, strings.ToUpper(rec.Status), rec.NItems, rec.Model, rec.Date)
		fmt.Fprintf(&b, "  precision %.2f (95%% interval %.2f-%.2f), recall %.2f (%.2f-%.2f), tp %d fp %d fn %d tn %d\n",
			rec.Precision, rec.PrecisionCI[0], rec.PrecisionCI[1], rec.Recall, rec.RecallCI[0], rec.RecallCI[1], rec.TP, rec.FP, rec.FN, rec.TN)
		for _, m := range rec.Misses {
			b.WriteString("  - " + m + "\n")
		}
	}
	if rep.LLM != nil {
		fmt.Fprintf(&b, "model calls: %d (%d cached), about $%.4f\n", rep.LLM.Calls, rep.LLM.Cached, rep.LLM.CostUSD)
	}
	if written && len(rep.Records) > 0 {
		b.WriteString("Records written to .ai-rulez/verifiers/calibration/ (commit them).\n")
	}
	return b.String()
}
