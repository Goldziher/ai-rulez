package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Goldziher/ai-rulez/v5/internal/evals"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

// validateActivationFlags checks the --mode, --surface and --scope flags before
// anything runs. A runner that cannot do activation is refused here, never run
// as a full case run.
func validateActivationFlags(cmd *cobra.Command) error {
	switch evalFlags.mode {
	case evals.ModeCases, evals.ModeActivation:
	default:
		return oops.Errorf("unknown --mode %q (use %s or %s)", evalFlags.mode, evals.ModeCases, evals.ModeActivation)
	}
	if evalFlags.mode != evals.ModeActivation {
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
	case evals.SurfaceRetrieval:
		return nil
	case evals.SurfaceNative:
		return requireNativeActivation(cmd)
	case "":
		return oops.Hint("--surface retrieval ranks the prompts offline with the find_skill ranker, for free").
			Errorf("--mode activation needs --surface (retrieval or native)")
	}
	return oops.Errorf("unknown --surface %q (use %s or %s)", evalFlags.surface, evals.SurfaceRetrieval, evals.SurfaceNative)
}

// requireNativeActivation refuses the native surface: a runner must declare the
// activation capability, and no runner does in this release.
func requireNativeActivation(cmd *cobra.Command) error {
	runner, _, err := buildEvalRunner(cmd)
	if err != nil {
		return err
	}
	if err := evals.RequireCapability(runner, evals.CapabilityActivation); err != nil {
		return oops.Hint("Use --surface retrieval for the offline find_skill ranking").Wrap(err)
	}
	return oops.Errorf("--surface native is not available in this release")
}

// runEvalActivation runs `eval run --mode activation --surface retrieval`.
func runEvalActivation(ctx context.Context, cmd *cobra.Command, skills []string, cfgDir, baseDir string) (failed bool, err error) {
	absDir, err := filepath.Abs(cfgDir)
	if err != nil {
		return false, oops.Wrapf(err, "resolve config directory")
	}
	date := evalFlags.date
	if date == "" {
		date = os.Getenv(EvalDateEnv)
	}
	opts := &evals.ActivationOptions{ConfigDir: absDir, Skills: skills, Scope: evalFlags.scope, Date: date}
	if thresholdGiven(cmd) {
		threshold := evalFlags.threshold
		opts.PassThreshold = &threshold
	}
	if opts.Changed, err = changedEvalSkills(absDir, baseDir); err != nil {
		return false, err
	}
	path := resultsPath(cfgDir)
	save := !evalDryRun() && !evalFlags.noWrite
	var store *evals.Store
	if save {
		if store, err = evals.LoadStoreKeyed(path, evals.UserKey()); err != nil {
			return false, err
		}
		opts.Store = store
	}
	report, runErr := evals.RunActivationRetrieval(ctx, opts)
	if report == nil {
		return false, runErr
	}
	if save && runErr == nil {
		if err := store.Save(path); err != nil {
			return false, err
		}
	}
	return report.Failed, errors.Join(writeActivationReport(cmd, report), runErr)
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
