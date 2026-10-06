package commands

import (
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

var (
	scanWriteBaseline   bool
	scanReason          string
	scanShowSuppressed  bool
	scanScannerBaseline string
)

func init() {
	for _, c := range []*cobra.Command{ValidateCmd, ScanCmd} {
		f := c.Flags()
		f.BoolVar(&scanWriteBaseline, "write-baseline", false,
			"With --external, accept every current scanner finding in "+lint.ScannerBaselineFile+" (needs --reason) and exit as if they were clean")
		f.StringVar(&scanReason, "reason", "", "With --write-baseline, why the findings are accepted (stored on each new entry)")
		f.BoolVar(&scanShowSuppressed, "show-suppressed", false, "With --external, also show scanner results the tool marked suppressed, as info")
		f.StringVar(&scanScannerBaseline, "scanner-baseline", "", "With --external, the scanner baseline file (default: <config dir>/"+lint.ScannerBaselineFile+")")
	}
}

// scannerFlagsSet reports whether a flag that only means something with
// --strict --external was given.
func scannerFlagsSet() bool {
	return scanWriteBaseline || scanReason != "" || scanShowSuppressed || scanScannerBaseline != ""
}

// scannerOptions validates the scanner flags and turns them into lint options.
func scannerOptions() (lint.ScannerOptions, error) {
	if scannerFlagsSet() && !validateExtern {
		return lint.ScannerOptions{}, oops.Errorf("--write-baseline, --reason, --show-suppressed and --scanner-baseline require --external")
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
	return lint.ScannerOptions{
		BaselinePath: scanScannerBaseline, WriteBaseline: scanWriteBaseline, Reason: scanReason,
		Today: today, ShowSuppressed: scanShowSuppressed,
	}, nil
}
