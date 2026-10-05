package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator"
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
	// validateRepoRoot is --repo-root: the directory repo-relative paths and
	// tracked-file globs resolve against (default: the git toplevel, else the
	// directory holding the configuration).
	validateRepoRoot string
	// strictSecurityOnly restricts strict validation to the security family (the scan command).
	strictSecurityOnly bool
	strictTreeCache    lint.Loader
)

// checkStrictFlags rejects strict-only flags used without --strict.
func checkStrictFlags() error {
	if !validateStrict && (validateFormat != "" || validateFailOn != "" || validateExtern) {
		return oops.Errorf("--format, --fail-on and --external require --strict")
	}
	switch validateFormat {
	case "", "text", formatJSON:
	default:
		return oops.Errorf("unknown --format %q (use text or json)", validateFormat)
	}
	switch validateFailOn {
	case "", "error", "warning", "info", "none":
	default:
		return oops.Errorf("unknown --fail-on %q (use error, warning, info or none)", validateFailOn)
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
	strictTreeCache = lint.Loader{Root: abs}
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
	combined := lint.Combine(reports)
	if validateFormat == formatJSON {
		if err := lint.WriteJSON(os.Stdout, combined); err != nil {
			fmtError(err)
			return 1
		}
	} else if err := lint.WriteText(os.Stdout, combined); err != nil {
		fmtError(err)
		return 1
	}
	for i, report := range reports {
		var cfg *config.Config
		if i < len(cfgs) {
			cfg = cfgs[i]
		}
		if lint.Failed(report.Findings, failOnFor(cfg)) {
			return exitStrictFindings
		}
	}
	return 0
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
	for _, f := range lint.FrontmatterWarnings(cfg, tree) {
		logger.Warn(fmt.Sprintf("%s:%d: %s %s: %s", f.File, f.Line, f.Severity, f.Code, f.Message))
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
