package commands

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator"
	"github.com/Goldziher/ai-rulez/internal/gitutil"
	"github.com/Goldziher/ai-rulez/internal/includes"
	"github.com/Goldziher/ai-rulez/internal/lint"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/samber/oops"
)

// Exit codes of `validate --strict`. 1 keeps its existing meaning (the
// configuration itself is invalid or could not be loaded).
const exitStrictFindings = 2

var (
	validateStrict bool
	validateFormat string
	validateFailOn string
	validateExtern bool
	validateOutput string
	// strictSecurityOnly restricts strict validation to the security family (the scan command).
	strictSecurityOnly bool
	strictTreeCache    lint.Loader
)

// checkStrictFlags rejects strict-only flags used without --strict.
func checkStrictFlags() error {
	if !validateStrict && (validateFormat != "" || validateFailOn != "" || validateExtern || validateOutput != "" || baselineFlagsSet() || changedRev() != "") {
		return oops.Errorf("--format, --output, --fail-on, --external, --since/--changed and the baseline flags require --strict")
	}
	if !lint.IsFormat(validateFormat) {
		return oops.Errorf("unknown --format %q (use %s)", validateFormat, strings.Join(lint.Formats(), ", "))
	}
	switch validateFailOn {
	case "", "error", "warning", "info", "none":
	default:
		return oops.Errorf("unknown --fail-on %q (use error, warning, info or none)", validateFailOn)
	}
	return checkFlagCombinations()
}

// checkFlagCombinations rejects strict flags that contradict each other.
func checkFlagCombinations() error {
	if validateSince != "" && validateChanged {
		return oops.Errorf("--since and --changed cannot be combined (--changed is --since HEAD)")
	}
	if validateUpdateBaseline && validateStrictBaseline {
		return oops.Errorf("--update-baseline and --strict-baseline cannot be combined: updating rewrites the entries that --strict-baseline would reject")
	}
	if validateUpdateBaseline && changedRev() != "" {
		return oops.Errorf("--update-baseline needs every finding; it cannot be combined with --since or --changed")
	}
	if validateBaselineReason != "" && !validateUpdateBaseline {
		return oops.Errorf("--baseline-reason only applies with --update-baseline")
	}
	return nil
}

// strictLint lints one loaded root.
func strictLint(cfg *config.Config) (*lint.Report, error) {
	if problems := lint.ValidateSettings(cfg.Lint); len(problems) > 0 {
		return nil, oops.Errorf("invalid [lint] settings: %v", problems)
	}
	tree, err := strictTreeCache.Load(cfg.BaseDir)
	if err != nil {
		return nil, oops.Wrapf(err, "index repository files")
	}
	var opts []lint.Option
	if cfg.Plugin != nil || cfg.Marketplace != nil {
		drift, driftErr := generator.NewGenerator(cfg).PluginVersionDrift("")
		if driftErr != nil {
			logger.Warn("Skipped the plugin version drift check", "error", driftErr)
		}
		opts = append(opts, lint.WithPluginDrift(drift))
	}
	return lint.RunWith(cfg, tree, lint.Options{SecurityOnly: strictSecurityOnly, External: validateExtern}, opts...)
}

// failOnFor resolves one root's threshold: the flag, else its [lint] fail_on,
// else error.
func failOnFor(cfg *config.Config) string {
	if validateFailOn != "" {
		return validateFailOn
	}
	if cfg != nil && cfg.Lint != nil && cfg.Lint.FailOn != "" {
		return cfg.Lint.FailOn
	}
	return "error"
}

// reportStrict prints the combined report and returns the process exit code.
// Each root is judged against its own threshold, so one root's [lint] fail_on
// never silences or tightens another's.
func reportStrict(reports []*lint.Report, cfgs []*config.Config) int {
	if validateUpdateBaseline {
		if err := updateBaselines(reports, cfgs); err != nil {
			fmtError(err)
			return 1
		}
		return 0
	}
	if err := applyBaselines(reports, cfgs); err != nil {
		fmtError(err)
		return 1
	}
	excess := make([][]lint.BudgetExcess, len(reports))
	for i, report := range reports {
		excess[i] = budgetsFor(cfgAt(cfgs, i)).Excess(report.Findings)
	}
	if err := narrowToChanged(reports, cfgs); err != nil {
		fmtError(err)
		return 1
	}
	combined := lint.Combine(reports)
	for _, e := range excess {
		combined.Budgets = append(combined.Budgets, e...)
	}
	if err := writeReport(combined, failOnFor(cfgAt(cfgs, 0))); err != nil {
		fmtError(err)
		return 1
	}
	code := 0
	for i, report := range reports {
		cfg := cfgAt(cfgs, i)
		if lint.FailedWithExcess(report.Findings, failOnFor(cfg), budgetsFor(cfg), excess[i]) {
			code = exitStrictFindings
		}
	}
	if baselineBlocks(reports) {
		code = exitStrictFindings
	}
	return code
}

// structuredFormat reports whether the format must be the only thing on stdout.
func structuredFormat(format string) bool {
	return format != "" && format != lint.FormatText
}

// writeReport prints the combined report in the chosen format, to --output
// (written atomically) or stdout.
func writeReport(combined lint.Combined, failOn string) error {
	opts := lint.WriteOptions{Version: Version, FailOn: failOn}
	if validateOutput == "" {
		return lint.Write(os.Stdout, validateFormat, combined, opts)
	}
	var buf bytes.Buffer
	if err := lint.Write(&buf, validateFormat, combined, opts); err != nil {
		return err //nolint:wrapcheck // formatter error
	}
	if dir := filepath.Dir(validateOutput); dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return oops.With("path", validateOutput).Wrapf(err, "create output directory")
		}
	}
	if err := gitutil.WriteFileAtomic(validateOutput, buf.Bytes(), 0o644); err != nil {
		return oops.With("path", validateOutput).Wrapf(err, "write report")
	}
	return nil
}

func runStrictSingle(cfg *config.Config) int {
	report, err := strictLint(cfg)
	if err != nil {
		fmtError(err)
		return 1
	}
	return reportStrict([]*lint.Report{report}, []*config.Config{cfg})
}

// warnUnpinned logs the remote includes and installed skills that follow a
// moving ref without a pin in ai-rulez.lock. It is advice, not a failure; the
// strict rule AR010 and "generate --locked" are the gates.
func warnUnpinned(cfg *config.Config) {
	for _, w := range includes.Unpinned(cfg) {
		ref := w.Ref
		if ref == "" {
			ref = "the default branch (HEAD)"
		}
		logger.Warn("Remote source is not pinned; run `ai-rulez lock`", "kind", w.Kind, "name", w.Name, "follows", ref)
	}
}

// enforceScanImports runs the security rules over imported content when
// [lint.security] scan_imports is set, before anything is written. At level
// "error" a finding stops the run; at "warn" it is logged.
func enforceScanImports(cfg *config.Config) error {
	findings, err := lint.ScanImports(cfg)
	if err != nil {
		return oops.Wrapf(err, "scan imported content")
	}
	if len(findings) == 0 {
		return nil
	}
	var lines []string
	blocked := false
	for _, f := range findings {
		lines = append(lines, fmt.Sprintf("%s:%d: %s %s: %s", f.File, f.Line, f.Severity, f.Code, f.Message))
		if f.Severity == lint.SeverityError {
			blocked = true
		}
	}
	if !blocked {
		for _, l := range lines {
			logger.Warn("Imported content: " + l)
		}
		return nil
	}
	return oops.Hint("Review the imported source, or lower [lint.security] scan_imports to \"warn\"").
		Errorf("imported content failed the security scan; nothing was written:\n  %s", strings.Join(lines, "\n  "))
}
