package commands

import (
	"os"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

var (
	scanWriteBaseline   bool
	scanReason          string
	scanShowSuppressed  bool
	scanScannerBaseline string
	scanNoCache         bool
)

func init() {
	for _, c := range []*cobra.Command{ValidateCmd, ScanCmd} {
		f := c.Flags()
		f.BoolVar(&scanWriteBaseline, "write-baseline", false,
			"With --external, accept every current scanner finding in "+lint.ScannerBaselineFile+" (needs --reason) and exit as if they were clean")
		f.StringVar(&scanReason, "reason", "", "With --write-baseline, why the findings are accepted (stored on each new entry)")
		f.BoolVar(&scanShowSuppressed, "show-suppressed", false, "With --external, also show scanner results the tool marked suppressed, as info")
		f.StringVar(&scanScannerBaseline, "scanner-baseline", "", "With --external, the scanner baseline file (default: [lint.scanner_policy] baseline, else <config dir>/"+lint.ScannerBaselineFile+")")
		f.BoolVar(&scanNoCache, "no-scan-cache", false, "With --external, ignore and do not update the scanner result cache")
	}
	ScanCmd.Flags().BoolVar(&validateDryRun, "dry-run", false, "With --external, print what each scanner would run (command, staged files, environment names, isolation, cache state) and start nothing")
}

// scannerFlagsSet reports whether a flag that only means something with
// --strict --external was given.
func scannerFlagsSet() bool {
	return scanWriteBaseline || scanReason != "" || scanShowSuppressed || scanScannerBaseline != "" || scanNoCache
}

// scannerDryRun reports whether this run only prints the scanner plan.
func scannerDryRun() bool { return validateExtern && validateDryRun && !fixRequested() }

// scannerOptions validates the scanner flags and turns them into lint options.
func scannerOptions() (lint.ScannerOptions, error) {
	if scannerFlagsSet() && !validateExtern {
		return lint.ScannerOptions{}, oops.Errorf("--write-baseline, --reason, --show-suppressed, --scanner-baseline and --no-scan-cache require --external")
	}
	if scanReason != "" && !scanWriteBaseline {
		return lint.ScannerOptions{}, oops.Errorf("--reason only applies with --write-baseline")
	}
	if scanWriteBaseline && scanReason == "" {
		return lint.ScannerOptions{}, oops.Hint("Pass --reason \"why these findings are accepted\"").Errorf("--write-baseline needs --reason")
	}
	if !validateExtern {
		return lint.ScannerOptions{}, nil
	}
	today, err := baselineToday()
	if err != nil {
		return lint.ScannerOptions{}, err
	}
	opts := lint.ScannerOptions{
		BaselinePath: scanScannerBaseline, WriteBaseline: scanWriteBaseline, Reason: scanReason,
		Today: today, ShowSuppressed: scanShowSuppressed, NoCache: scanNoCache,
	}
	if scannerDryRun() {
		opts.DryRun, opts.Out = true, os.Stdout
		if structuredFormat(validateFormat) {
			opts.Out = os.Stderr // the report format owns stdout
		}
	}
	return opts, nil
}

// withScannerCache attaches the per-user scanner result cache of cfg's project.
func withScannerCache(opts lint.ScannerOptions, cfg *config.Config) lint.ScannerOptions {
	if validateExtern && !opts.NoCache && cfg != nil {
		opts.Cache = lint.UserScanCache(cfg.ConfigDir)
	}
	return opts
}
