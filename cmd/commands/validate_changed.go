package commands

import (
	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/gitutil"
	"github.com/Goldziher/ai-rulez/internal/lint"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

var (
	// validateSince narrows the report to files changed since a git revision.
	validateSince string
	// validateChanged is --since HEAD: uncommitted and untracked changes.
	validateChanged bool
)

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

func addChangedFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringVar(&validateSince, "since", "", "With --strict, report only findings in files changed since this git revision (committed, staged, unstaged and untracked) and in files that refer to them; references are still resolved against the whole tree")
	f.BoolVar(&validateChanged, "changed", false, "With --strict, shorthand for --since HEAD: only files with uncommitted or untracked changes")
}

// narrowToChanged narrows each report to the changed files of its repository.
// Baselines are applied before this so stale entries are judged against every
// finding, not only the visible ones.
func narrowToChanged(reports []*lint.Report, cfgs []*config.Config) error {
	rev := changedRev()
	if rev == "" {
		return nil
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
			var err error
			changed, err = gitutil.ChangedSince(cfg.BaseDir, rev)
			if err != nil {
				return oops.Wrap(err)
			}
			cache[top] = changed
		}
		scope := lint.NarrowToChanged(r, changed, rev)
		r.Scope = &scope
	}
	return nil
}
