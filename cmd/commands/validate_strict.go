package commands

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/okfbridge"
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
	// validateVerifiers is --verifiers: also report the verifiers as AR9H findings.
	validateVerifiers bool
	// validateApprovalsBase is --approvals-base: the git revision approvals are compared with (AR716).
	validateApprovalsBase string
	// validateRepoRoot is --repo-root: the directory repo-relative paths and
	// tracked-file globs resolve against (default: the git toplevel, else the
	// directory holding the configuration).
	validateRepoRoot string
	validateOutput   string
	// validateLintProfile overrides [lint] profile. It is not --profile: that
	// name selects a generation profile everywhere else.
	validateLintProfile string
	// validateAnalyzers runs only these analyzers (replacing [lint] analyzers).
	validateAnalyzers []string
	// validateAllowEgress names the egress = true scanners allowed to run (--allow-egress).
	validateAllowEgress []string
	// strictSecurityOnly restricts strict validation to the security family (the scan command).
	strictSecurityOnly bool
	strictTreeCache    lint.Loader
)

// strictOnlyFlagSet reports whether any flag that only means something with
// --strict was given.
func strictOnlyFlagSet() bool {
	return validateFormat != "" || validateFailOn != "" || validateExtern || validateOutput != "" ||
		fixRequested() || validateDryRun || validateLintProfile != "" || len(validateAnalyzers) > 0 ||
		baselineFlagsSet() || changedRev() != "" || scannerFlagsSet() || validateApprovalsBase != "" || validateVerifiers
}

// checkStrictFlags rejects strict-only flags used without --strict.
func checkStrictFlags() error {
	if !validateStrict && strictOnlyFlagSet() {
		return oops.Errorf("--format, --output, --fail-on, --external, --since/--changed, --fix, --verifiers and the baseline flags require --strict")
	}
	if len(validateAllowEgress) > 0 && !validateExtern {
		return oops.Errorf("--allow-egress requires --external")
	}
	if err := checkFlagValues(); err != nil {
		return err
	}
	return checkFlagCombinations()
}

// checkFlagValues rejects unknown names and values.
func checkFlagValues() error {
	if err := checkFormat(validateFormat, lint.Formats()); err != nil {
		return err
	}
	switch validateFailOn {
	case "", "error", "warning", "info", "none":
	default:
		return oops.Errorf("unknown --fail-on %q (use error, warning, info or none)", validateFailOn)
	}
	known := lint.AnalyzerNames()
	for _, a := range validateAnalyzers {
		if !slices.Contains(known, strings.ToLower(strings.TrimSpace(a))) {
			return oops.Errorf("unknown --analyzer %q (use %s)", a, strings.Join(known, ", "))
		}
	}
	if _, ok := lint.LookupProfile(validateLintProfile); !ok {
		return oops.Errorf("unknown --lint-profile %q (use %s)", validateLintProfile, strings.Join(lint.ProfileNames(), ", "))
	}
	return nil
}

// checkFlagCombinations rejects strict flags that contradict each other.
func checkFlagCombinations() error {
	if validateDryRun && !fixRequested() && !validateExtern {
		return oops.Errorf("--dry-run only applies with --fix, --fix-unsafe or --external")
	}
	if fixRequested() && validateUpdateBaseline {
		return oops.Errorf("--fix and --update-baseline cannot be combined: fix first, then record what is left")
	}
	if validateSince != "" && validateChanged {
		return oops.Errorf("--since and --changed cannot be combined (--changed is --since HEAD)")
	}
	if changedRev() == "" && (validateSinceMax != 0 || (validateSinceDepth != "" && validateSinceDepth != "1")) {
		return oops.Errorf("--since-depth and --since-max-files need --since or --changed")
	}
	if _, err := sinceDepth(); err != nil {
		return err
	}
	if validateSinceMax < 0 {
		return oops.Errorf("--since-max-files must not be negative")
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

// repoRootEnv names the environment variable that --repo-root defaults to.
const repoRootEnv = "AI_RULEZ_REPO_ROOT"

// applyRepoRoot resolves --repo-root (or AI_RULEZ_REPO_ROOT) and points the
// strict tree loader at it. An empty value keeps the default lookup.
func applyRepoRoot() error {
	root := validateRepoRoot
	if root == "" {
		root = os.Getenv(repoRootEnv)
	}
	if root == "" {
		return nil
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return oops.Wrapf(err, "resolve --repo-root %q", root)
	}
	if info, statErr := os.Stat(abs); statErr != nil || !info.IsDir() {
		return oops.Errorf("--repo-root %q is not a directory", root)
	}
	// Without git there is no index to bound the file list, and an explicit root
	// can be anything (a home directory, /), so the walk it would trigger is
	// refused rather than guessed at.
	if !gitutil.IsRepo(abs) {
		return oops.Hint("point --repo-root at a git work tree (git must be installed), or omit it to use the "+
			"configuration's directory").Errorf("--repo-root %q is not inside a git repository", root)
	}
	strictTreeCache = lint.Loader{Root: abs}
	return nil
}

// strictLint lints one loaded root.
func strictLint(cfg *config.Config) (*lint.Report, error) {
	if validateLintProfile != "" {
		lc := config.LintConfig{}
		if cfg.Lint != nil {
			lc = *cfg.Lint
		}
		lc.Profile = validateLintProfile
		cfg.Lint = &lc
	}
	if problems := lint.ValidateSettings(cfg.Lint); len(problems) > 0 {
		return nil, oops.Errorf("invalid [lint] settings: %v", problems)
	}
	tree, err := strictTreeCache.Load(cfg.BaseDir)
	if err != nil {
		return nil, oops.Wrapf(err, "index repository files")
	}
	sel := analyzerSelection(cfg)
	var opts []lint.Option
	if (cfg.Plugin != nil || cfg.Marketplace != nil) && lint.AnalyzerSelected(sel, lint.AnalyzerPlugin) {
		drift, driftErr := generator.NewGenerator(cfg).PluginVersionDrift("")
		if driftErr != nil {
			logger.Warn("Skipped the plugin version drift check", "error", driftErr)
		}
		opts = append(opts, lint.WithPluginDrift(drift))
	}
	if lint.AnalyzerSelected(sel, lint.AnalyzerDelivery, lint.AnalyzerLock) {
		if findings := deliveryFindings(cfg); !strictSecurityOnly && len(findings) > 0 {
			opts = append(opts, lint.WithDelivery(findings))
		}
	}
	if lint.AnalyzerSelected(sel, lint.AnalyzerLock) {
		if drift := lockDriftFor(cfg); len(drift) > 0 {
			opts = append(opts, lint.WithLockDrift(drift))
		}
		if findings := approvalFindingsFor(cfg); len(findings) > 0 {
			opts = append(opts, lint.WithApprovals(findings))
		}
		if findings := signingFindingsFor(cfg); len(findings) > 0 {
			opts = append(opts, lint.WithSigning(findings))
		}
	}
	opts = append(opts, sbomOptions(cfg, sel)...)
	if lint.AnalyzerSelected(sel, lint.AnalyzerTraps) {
		// The traps judge the files a run would write, whether or not generate has run.
		opts = append(opts, lint.WithPlanned(generator.NewPlannedFiles(cfg)))
	}
	if lint.AnalyzerSelected(sel, lint.AnalyzerOKF) {
		if okfRes, okfErr := checkOKFProject(cfg); okfErr != nil {
			logger.Warn("Skipped the OKF bundle checks", "error", okfErr)
		} else if okfRes != nil {
			opts = append(opts, lint.WithOKF(okfRes.Dir, okfRes.Findings))
		}
	}
	if validateVerifiers && lint.AnalyzerSelected(sel, lint.AnalyzerVerifiers) {
		opts = append(opts, lint.WithVerifiers(verifierFindingsFor(cmdContext(), cfg)))
	}
	scanner, err := scannerOptions()
	if err != nil {
		return nil, err
	}
	scanner = withScannerCache(scanner, cfg)
	return lint.RunWith(cfg, tree, lint.Options{
		SecurityOnly: strictSecurityOnly, External: validateExtern, AllowEgress: validateAllowEgress,
		Scanner: scanner, Analyzers: validateAnalyzers, NeedDeps: changedRev() != "", Cwd: workingDir(),
	}, opts...)
}

// analyzerSelection is the analyzer allow-list of a run: --analyzer, else
// [lint] analyzers, else (the scan command) the security analyzer; nil runs
// every analyzer.
func analyzerSelection(cfg *config.Config) []string {
	if len(validateAnalyzers) > 0 {
		return validateAnalyzers
	}
	if cfg.Lint != nil && len(cfg.Lint.Analyzers) > 0 {
		return cfg.Lint.Analyzers
	}
	if strictSecurityOnly {
		return []string{lint.AnalyzerSecurity}
	}
	return nil
}

// failOnFor resolves one root's threshold: the flag, else its [lint] fail_on,
// else error.
func failOnFor(cfg *config.Config) string {
	if validateFailOn != "" {
		return validateFailOn
	}
	if cfg != nil && cfg.Lint != nil {
		if cfg.Lint.FailOn != "" {
			return cfg.Lint.FailOn
		}
		if f := lint.ProfileFailOn(cfg.Lint.Profile); f != "" {
			return f
		}
	}
	return "error"
}

// reportStrict prints the combined report and returns the process exit code.
// Each root is judged against its own threshold, so one root's [lint] fail_on
// never silences or tightens another's.
func reportStrict(reports []*lint.Report, cfgs []*config.Config) int {
	if scannerDryRun() {
		return 0 // the scanner plan was printed; nothing ran, so there is no report
	}
	excess, code, done := prepareReports(reports, cfgs)
	if done {
		return code
	}
	combined := lint.Combine(reports)
	for _, e := range excess {
		combined.Ratchet = append(combined.Ratchet, e...)
	}
	if err := writeReport(combined, failOnFor(cfgAt(cfgs, 0))); err != nil {
		fmtError(err)
		return 1
	}
	code = 0
	for i, report := range reports {
		cfg := cfgAt(cfgs, i)
		if lint.FailedWithExcess(report.Findings, failOnFor(cfg), ratchetFor(cfg), excess[i]) {
			code = exitStrictFindings
		}
	}
	if baselineBlocks(reports) {
		code = exitStrictFindings
	}
	return code
}

// prepareReports runs the steps between linting and printing, in the order that
// keeps each one honest: fixes first (so fixed findings leave the report), then
// the baseline against every finding of the analyzers that ran (so stale
// entries are judged on the full set; entries of an analyzer that did not run
// are left alone), budgets on the full set, and only then the view that narrows
// the report (changed-only) and the risk score of what is shown. done is
// true when the run ends here with code (--update-baseline, or an error).
func prepareReports(reports []*lint.Report, cfgs []*config.Config) (excess [][]lint.RatchetExcess, code int, done bool) {
	fail := func(err error) ([][]lint.RatchetExcess, int, bool) {
		fmtError(err)
		return nil, 1, true
	}
	if fixRequested() {
		if err := applyFixes(reports, cfgs); err != nil {
			return fail(err)
		}
	}
	if validateUpdateBaseline {
		if err := updateBaselines(reports, cfgs); err != nil {
			return fail(err)
		}
		return nil, 0, true
	}
	if err := applyBaselines(reports, cfgs); err != nil {
		return fail(err)
	}
	reportRefusedRatchet(reports, cfgs)
	excess = make([][]lint.RatchetExcess, len(reports))
	for i, report := range reports {
		excess[i] = ratchetFor(cfgAt(cfgs, i)).Excess(report.Findings)
	}
	if err := narrowToChanged(reports, cfgs); err != nil {
		return fail(err)
	}
	for i, report := range reports {
		var rc *config.LintRisk
		if cfg := cfgAt(cfgs, i); cfg != nil && cfg.Lint != nil {
			rc = cfg.Lint.Risk
		}
		risk := lint.ComputeRisk(report.Findings, lint.RiskWeightsFrom(rc))
		report.Risk = &risk
	}
	return excess, 0, false
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

// checkAllowEgress rejects --allow-egress names that no [[lint.external]]
// entry of cfgs declares: a typo would otherwise silently allow nothing.
func checkAllowEgress(cfgs ...*config.Config) error {
	declared := map[string]bool{}
	for _, cfg := range cfgs {
		if cfg == nil || cfg.Lint == nil {
			continue
		}
		for _, ex := range cfg.Lint.External {
			declared[ex.Name] = true
		}
	}
	for _, name := range validateAllowEgress {
		if !declared[name] {
			return oops.Hint("Check the name against [[lint.external]] in config.toml.").Errorf("--allow-egress=%s names no [[lint.external]] scanner", name)
		}
	}
	return nil
}

func runStrictSingle(cfg *config.Config) int {
	if err := checkAllowEgress(cfg); err != nil {
		fmtError(err)
		return 1
	}
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

// warnFrontmatter logs the frontmatter problems that plain validate and generate
// report without --strict: an agent key no tool reads (dropped silently
// otherwise) and a skills: entry that names no skill.
func warnFrontmatter(cfg *config.Config) {
	if cfg == nil || cfg.Content == nil {
		return
	}
	tree, err := strictTreeCache.Load(cfg.BaseDir)
	if err != nil {
		return
	}
	for _, f := range lint.FrontmatterWarnings(cfg, tree, lint.WithCwd(workingDir())) {
		logger.Warn(fmt.Sprintf("%s:%d: %s %s: %s", f.File, f.Line, f.Severity, f.Code, f.Message))
	}
}

// enforceScanImports runs the security rules over imported content before
// anything is written, unless [lint.security] scan_imports is "off". An
// error-level finding (every finding at level "error") stops the run; the rest
// are logged.
func enforceScanImports(cfg *config.Config) error {
	findings, err := lint.ScanImports(cfg, lint.WithCwd(workingDir()))
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
	return oops.Hint("Review the imported source, or set [lint.security] scan_imports to \"warn\" (or \"off\")").
		Errorf("imported content failed the security scan; nothing was written:\n  %s", strings.Join(lines, "\n  "))
}

// checkOKFProject lints the configured OKF bundle (AR9B0-AR9B9), including
// drift against what the okf preset would write now. nil when none is configured.
func checkOKFProject(cfg *config.Config) (*okfbridge.ProjectResult, error) {
	if !okfbridge.Configured(cfg) {
		return nil, nil
	}
	var tree *config.ContentTree
	if cfg.OKFEnabled() {
		var err error
		if tree, err = generator.NewGenerator(cfg).ContentForProfile(""); err != nil {
			return nil, err
		}
	}
	return okfbridge.CheckProject(cfg, tree)
}
