package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

var (
	// validateBaseline is an explicit baseline file; it must exist unless
	// --update-baseline creates it.
	validateBaseline       string
	validateUpdateBaseline bool
	validateBaselineReason string
	validateStrictBaseline bool
	// validateToday overrides the date used for baseline expiry (YYYY-MM-DD);
	// tests and reproducible CI runs pin it, the default reads the clock here
	// and nowhere else.
	validateToday string
)

// todayEnv pins the expiry date like --today does.
const todayEnv = "AI_RULEZ_TODAY"

func baselineFlagsSet() bool {
	return validateBaseline != "" || validateUpdateBaseline || validateBaselineReason != "" || validateStrictBaseline
}

// baselineToday returns the YYYY-MM-DD date entries expire against.
func baselineToday() (string, error) {
	today := validateToday
	if today == "" {
		today = os.Getenv(todayEnv)
	}
	if today == "" {
		return time.Now().UTC().Format("2006-01-02"), nil
	}
	if _, err := time.Parse("2006-01-02", today); err != nil {
		return "", oops.Errorf("--today / %s must be a YYYY-MM-DD date, got %q", todayEnv, today)
	}
	return today, nil
}

// baselinePathFor resolves one root's baseline file: --baseline, else
// <config dir>/lint-baseline.json.
func baselinePathFor(cfg *config.Config) string {
	if validateBaseline != "" {
		return validateBaseline
	}
	if cfg == nil || cfg.ConfigDir == "" {
		return ""
	}
	return filepath.Join(cfg.ConfigDir, lint.BaselineFile)
}

func cfgAt(cfgs []*config.Config, i int) *config.Config {
	if i < len(cfgs) {
		return cfgs[i]
	}
	return nil
}

// applyBaselines marks accepted findings in each report. The default file is
// used only when it exists; an explicit --baseline must exist.
func applyBaselines(reports []*lint.Report, cfgs []*config.Config) error {
	today, err := baselineToday()
	if err != nil {
		return err
	}
	for i, r := range reports {
		path := baselinePathFor(cfgAt(cfgs, i))
		if path == "" {
			continue
		}
		b, lerr := lint.LoadBaseline(path)
		if lerr != nil {
			return oops.Wrapf(lerr, "load baseline")
		}
		if b == nil {
			if validateBaseline != "" {
				return oops.Hint("Create it with `ai-rulez validate --strict --update-baseline`").Errorf("baseline %s does not exist", path)
			}
			continue
		}
		res := lint.ApplyBaseline(r, b, path, today)
		r.Baseline = &res
	}
	return nil
}

// updateBaselines rewrites each root's baseline to accept its current findings.
func updateBaselines(reports []*lint.Report, cfgs []*config.Config) error {
	for i, r := range reports {
		path := baselinePathFor(cfgAt(cfgs, i))
		if path == "" {
			return oops.Errorf("--update-baseline needs a configuration directory or --baseline <file>")
		}
		prev, err := lint.LoadBaseline(path)
		if err != nil {
			return oops.Wrapf(err, "load baseline")
		}
		next, err := lint.UpdateBaseline(r, prev, validateBaselineReason)
		if err != nil {
			return oops.Wrap(err)
		}
		if err := next.Save(path); err != nil {
			return oops.Wrap(err)
		}
		before := 0
		if prev != nil {
			before = len(prev.Entries)
		}
		fmt.Fprintf(os.Stderr, "baseline %s: %d entries (was %d)\n", path, len(next.Entries), before)
	}
	return nil
}

// budgetsFor resolves one root's [lint.tolerate] (or its deprecated [lint.budget]).
func budgetsFor(cfg *config.Config) lint.Budgets {
	if cfg == nil || cfg.Lint == nil {
		return nil
	}
	budgets, _ := lint.ResolveBudgets(cfg.Lint.Tolerated()).Without(lint.ProtectedCodes(cfg.PolicyOutcome))
	return budgets
}

// reportRefusedBudgets reports, once per report, a [lint.tolerate] entry for a
// code the organization policy protects: the entry is ignored.
func reportRefusedBudgets(reports []*lint.Report, cfgs []*config.Config) {
	for i, r := range reports {
		cfg := cfgAt(cfgs, i)
		if cfg == nil || cfg.Lint == nil {
			continue
		}
		_, dropped := lint.ResolveBudgets(cfg.Lint.Tolerated()).Without(lint.ProtectedCodes(cfg.PolicyOutcome))
		r.RefuseTolerate(dropped)
	}
}

// baselineBlocks reports whether --strict-baseline turns stale or expired
// entries into a failure.
func baselineBlocks(reports []*lint.Report) bool {
	if !validateStrictBaseline {
		return false
	}
	for _, r := range reports {
		if r.Baseline != nil && (len(r.Baseline.Stale) > 0 || len(r.Baseline.Expired) > 0) {
			return true
		}
	}
	return false
}

// addBaselineFlags registers the baseline flags on validate and scan.
func addBaselineFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringVar(&validateBaseline, "baseline", "", strictOnly(cmd, "accept the findings recorded in this baseline file (default: <config dir>/")+lint.BaselineFile+" when it exists); only new findings count toward the exit code")
	f.BoolVar(&validateUpdateBaseline, "update-baseline", false, strictOnly(cmd, "record every current finding in the baseline (keeping existing reasons and dropping stale entries) and exit 0"))
	f.StringVar(&validateBaselineReason, "baseline-reason", "", "With --update-baseline, the reason stored on new entries (required for security findings)")
	f.BoolVar(&validateStrictBaseline, "strict-baseline", false, strictOnly(cmd, "also fail (exit 2) when the baseline has stale or expired entries, so fixed findings must leave it"))
	f.StringVar(&validateToday, "today", "", "Date (YYYY-MM-DD) baseline expiry is judged against; default is today's date (or $"+todayEnv+")")
	_ = f.MarkHidden("today") //nolint:errcheck // the flag was just registered
}
