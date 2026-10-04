package commands

import (
	"os"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator"
	"github.com/Goldziher/ai-rulez/internal/lint"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/samber/oops"
)

// Exit codes of `validate --strict`. 1 keeps its existing meaning (the
// configuration itself is invalid or could not be loaded).
const exitStrictFindings = 2

var (
	validateStrict  bool
	validateFormat  string
	validateFailOn  string
	strictTreeCache lint.Loader
)

// checkStrictFlags rejects strict-only flags used without --strict.
func checkStrictFlags() error {
	if !validateStrict && (validateFormat != "" || validateFailOn != "") {
		return oops.Errorf("--format and --fail-on require --strict")
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
	return lint.Run(cfg, tree, opts...)
}

// failOnFor resolves the threshold: the flag, else [lint] fail_on, else error.
func failOnFor(cfgs []*config.Config) string {
	if validateFailOn != "" {
		return validateFailOn
	}
	for _, c := range cfgs {
		if c.Lint != nil && c.Lint.FailOn != "" {
			return c.Lint.FailOn
		}
	}
	return "error"
}

// reportStrict prints the combined report and returns the process exit code.
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
	if lint.Failed(combined.Findings, failOnFor(cfgs)) {
		return exitStrictFindings
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
