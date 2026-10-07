package commands

import (
	"strconv"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

var (
	// validateSince narrows the report to files changed since a git revision.
	validateSince string
	// validateChanged is --since HEAD: uncommitted and untracked changes.
	validateChanged bool
	// validateSinceDepth is --since-depth: reference hops to follow, or "all".
	validateSinceDepth string
	// validateSinceMax is --since-max-files: a cap on the files reported besides the changed ones.
	validateSinceMax int
)

// sinceDepth resolves --since-depth: 1 by default, a positive number, or
// lint.DepthAll for "all".
func sinceDepth() (int, error) {
	v := strings.ToLower(strings.TrimSpace(validateSinceDepth))
	if v == "" {
		return 1, nil
	}
	if v == "all" {
		return lint.DepthAll, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return 0, oops.Errorf("--since-depth %q must be a positive number or \"all\"", validateSinceDepth)
	}
	return n, nil
}

// changedRev resolves --since / --changed to a revision ("" when off).
func changedRev() string {
	if validateSince != "" {
		return validateSince
	}
	if validateChanged {
		return "HEAD"
	}
	return ""
}

// strictOnly words a flag description that only applies to the content checks:
// validate runs them by default, so it just capitalizes the text.
func strictOnly(_ *cobra.Command, text string) string {
	return strings.ToUpper(text[:1]) + text[1:]
}

func addChangedFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringVar(&validateSince, "since", "", strictOnly(cmd, "report only findings in files changed since this git revision (committed, staged, unstaged and untracked) and in files that refer to them; references are still resolved against the whole tree"))
	f.StringVar(&validateSinceDepth, "since-depth", "1", "With --since or --changed, how many reference hops to follow from the changed files: a number or \"all\" for every file that depends on them, directly or not (findings are marked changed, dependent or transitive(n) in json)")
	f.IntVar(&validateSinceMax, "since-max-files", 0, "With --since or --changed, report at most this many files besides the changed ones, nearest first (0: no cap)")
	f.BoolVar(&validateChanged, "changed", false, strictOnly(cmd, "shorthand for --since HEAD: only files with uncommitted or untracked changes"))
}

// narrowToChanged narrows each report to the changed files of its repository.
// Baselines are applied before this so stale entries are judged against every
// finding, not only the visible ones.
func narrowToChanged(reports []*lint.Report, cfgs []*config.Config) error {
	rev := changedRev()
	if rev == "" {
		return nil
	}
	depth, err := sinceDepth()
	if err != nil {
		return err
	}
	cache := map[string][]string{}
	for i, r := range reports {
		cfg := cfgAt(cfgs, i)
		if cfg == nil {
			continue
		}
		top := gitutil.TopLevel(cfg.BaseDir)
		changed, ok := cache[top]
		if !ok {
			var changedErr error
			changed, changedErr = gitutil.ChangedSince(cfg.BaseDir, rev)
			if changedErr != nil {
				return oops.Wrap(changedErr)
			}
			cache[top] = changed
		}
		scope := lint.NarrowToChangedWith(r, changed, rev, lint.NarrowOptions{Depth: depth, MaxFiles: validateSinceMax})
		r.Scope = &scope
	}
	return nil
}
