package commands

import (
	"context"
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
	scanMinSeverity     string
	scanRules           []string
	scanReasonFor       []string
)

func init() {
	for _, c := range []*cobra.Command{ValidateCmd, ScanCmd} {
		f := c.Flags()
		f.BoolVar(&scanWriteBaseline, "write-baseline", false,
			"With --external, accept every current scanner finding in "+lint.ScannerBaselineFile+" (needs --reason) and exit as if they were clean")
		f.StringVar(&scanReason, "reason", "", "With --write-baseline, why the findings are accepted (stored on each new entry); {rule}, {severity} and {scanner} are replaced per entry")
		f.StringVar(&scanMinSeverity, "min-severity", "", "With --write-baseline, record only findings of at least this severity (info, warning, error)")
		f.StringArrayVar(&scanRules, "rule", nil, "With --write-baseline, record only findings of this scanner rule id, scanner:rule or AR code (a glob; repeatable)")
		f.StringArrayVar(&scanReasonFor, "reason-for", nil, "With --write-baseline, RULE=text: the reason of new entries of that rule (a glob; repeatable); --reason covers the rest")
		f.BoolVar(&scanShowSuppressed, "show-suppressed", false, "With --external, also show scanner results the tool marked suppressed, as info")
		f.StringVar(&scanScannerBaseline, "scanner-baseline", "", "With --external, the scanner baseline file (default: [lint.scanner_policy] baseline, else <config dir>/"+lint.ScannerBaselineFile+")")
		f.BoolVar(&scanNoCache, "no-scan-cache", false, "With --external, ignore and do not update the scanner result cache")
	}
	specDryRun.Bool(ScanCmd.Flags(), &validateDryRun, "With --external, print what each scanner would run (command, staged files, environment names, isolation, cache state) and start nothing")
}

// scannerFlagsSet reports whether a flag that only means something with
// --strict --external was given.
func scannerFlagsSet() bool {
	return scanWriteBaseline || scanMinSeverity != "" || len(scanRules) > 0 || len(scanReasonFor) > 0 || scanReason != "" || scanShowSuppressed || scanScannerBaseline != "" || scanNoCache
}

// scannerDryRun reports whether this run only prints the scanner plan.
func scannerDryRun() bool { return validateExtern && validateDryRun && !fixRequested() }

// scannerOptions validates the scanner flags and turns them into lint options.
func scannerOptions() (lint.ScannerOptions, error) {
	if scannerFlagsSet() && !validateExtern {
		return lint.ScannerOptions{}, oops.Errorf("--write-baseline, --reason, --show-suppressed, --scanner-baseline and --no-scan-cache require --external")
	}
	if (scanReason != "" || scanMinSeverity != "" || len(scanRules) > 0 || len(scanReasonFor) > 0) && !scanWriteBaseline {
		return lint.ScannerOptions{}, oops.Errorf("--reason, --reason-for, --min-severity and --rule only apply with --write-baseline")
	}
	if scanWriteBaseline && scanReason == "" {
		return lint.ScannerOptions{}, oops.Hint("Pass --reason \"why these findings are accepted\"").Errorf("--write-baseline needs --reason")
	}
	if !validateExtern {
		return lint.ScannerOptions{}, nil
	}
	filter, reasonFor, err := baselineSelection()
	if err != nil {
		return lint.ScannerOptions{}, err
	}
	today, err := baselineToday()
	if err != nil {
		return lint.ScannerOptions{}, err
	}
	opts := lint.ScannerOptions{
		BaselinePath: scanScannerBaseline, WriteBaseline: scanWriteBaseline, Reason: scanReason,
		Filter: filter, ReasonFor: reasonFor,
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

// baselineSelection parses --min-severity, --rule and --reason-for.
func baselineSelection() (lint.BaselineFilter, map[string]string, error) {
	filter, err := lint.ParseBaselineFilter(scanMinSeverity, scanRules)
	if err != nil {
		return filter, nil, oops.Wrap(err)
	}
	reasonFor, err := lint.ParseReasonFor(scanReasonFor)
	if err != nil {
		return filter, nil, oops.Wrap(err)
	}
	return filter, reasonFor, nil
}

// scannerOptionsFor is scannerOptions for one root: it attaches the project's
// result cache and, under --changed or --since, the files changed since the
// revision, so a scanner handed {skill_dirs} gets only the changed skills.
func scannerOptionsFor(ctx context.Context, cfg *config.Config) (lint.ScannerOptions, error) {
	opts, err := scannerOptions()
	if err != nil {
		return opts, err
	}
	opts = withScannerCache(opts, cfg)
	if validateExtern && changedRev() != "" {
		changed, cerr := changedFiles(ctx, cfg, changedRev(), map[string][]string{})
		if cerr != nil {
			return opts, cerr
		}
		opts.ChangedFiles = append([]string{}, changed...) // non-nil: an empty list means nothing changed
	}
	return opts, nil
}

// withScannerCache attaches the per-user scanner result cache of cfg's project.
func withScannerCache(opts lint.ScannerOptions, cfg *config.Config) lint.ScannerOptions {
	if validateExtern && !opts.NoCache && cfg != nil {
		opts.Cache = lint.UserScanCache(cfg.Log(), cfg.ConfigDir)
	}
	return opts
}
