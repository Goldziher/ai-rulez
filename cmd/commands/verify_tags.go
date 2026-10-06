package commands

import (
	"context"
	"fmt"
	"os"
	"sort"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/tagresolve"
	"github.com/samber/oops"
)

var (
	lockVerifyTags     bool
	generateVerifyTags bool
)

func init() {
	const usage = "Ask the remotes whether a tag pinned in ai-rulez.lock moved (AR732) or was deleted (AR735); needs the network (also [lock] verify_tags = true)"
	LockCmd.Flags().BoolVar(&lockVerifyTags, "verify-tags", false, "With --check: "+usage)
	GenerateCmd.Flags().BoolVar(&generateVerifyTags, "verify-tags", false, usage)
}

// tagFinding is a pinned tag the remote disagrees with.
type tagFinding struct {
	kind, name, code, message string
	failing                   bool
}

func (f tagFinding) String() string {
	return fmt.Sprintf("%s %s %s: %s", f.code, f.kind, f.name, f.message)
}

// verifyPinnedTags compares every tag pinned in lock with the remote's tags: a
// tag that points to another commit is AR732 (an error, the sign of a
// force-pushed release), a tag that is gone AR735 (a warning, the pinned commit
// is still used). Only sources that use a version constraint have a tag. It
// reads refs only (one ls-remote per repository) and fetches nothing.
func verifyPinnedTags(ctx context.Context, cfg *config.Config, lock *lockfile.File) ([]tagFinding, error) {
	if lock == nil {
		return nil, nil
	}
	var lister tagLister
	var out []tagFinding
	for _, s := range versionSources(cfg, "", nil) {
		entry := lock.Find(s.kind, s.name)
		if entry == nil || entry.Tag == "" {
			continue
		}
		tags, err := lister.of(ctx, s)
		if err != nil {
			return nil, err
		}
		switch status, now := tagresolve.Check(tags, entry.Tag, entry.Commit); status {
		case tagresolve.StatusMoved:
			out = append(out, tagFinding{s.kind, s.name, tagresolve.CodeTagMoved,
				fmt.Sprintf("tag %s was pinned at %s but now points to %s (review the new commit, then `ai-rulez update --accept-moved-tag`)", entry.Tag, shortSHA(entry.Commit), shortSHA(now.Commit)), true})
		case tagresolve.StatusMissing:
			out = append(out, tagFinding{s.kind, s.name, tagresolve.CodeLockedTagMissed,
				fmt.Sprintf("tag %s no longer exists on the remote; the pinned commit %s is still used", entry.Tag, shortSHA(entry.Commit)), false})
		case tagresolve.StatusOK:
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out, nil
}

// verifyTagsWanted reports whether a run should ask the remotes: the flag, or
// [lock] verify_tags. Asking needs the network; an explicit flag with
// --no-fetch/--frozen is a usage error, the config key is skipped quietly.
func verifyTagsWanted(cfg *config.Config, flag bool) (bool, error) {
	if !flag && !cfg.LockVerifyTags() {
		return false, nil
	}
	if includes.SkipFetch || lockOffline {
		if flag {
			return false, oops.Hint("Drop --no-fetch/--frozen/--offline, or drop --verify-tags").
				Errorf("--verify-tags reads the remote's tags and needs the network")
		}
		logger.Debug("not verifying pinned tags: the run is offline")
		return false, nil
	}
	return true, nil
}

// report prints the findings and returns whether any is an error.
func reportTagFindings(findings []tagFinding) (failing bool) {
	for _, f := range findings {
		if f.failing {
			fmt.Fprintln(os.Stderr, f.String())
			failing = true
		} else {
			logger.Warn(f.String())
		}
	}
	return failing
}

// verifyTagsAt is the `lock --check --verify-tags` step: exit 2 for a moved tag, 1 when the remote cannot be read.
func verifyTagsAt(path string) int {
	cfg, _, err := loadForLockCheck(path)
	if err != nil {
		fmtError(err)
		return 1
	}
	want, err := verifyTagsWanted(cfg, lockVerifyTags)
	if err != nil {
		fmtError(err)
		return 1
	}
	if !want {
		return 0
	}
	lock, err := lockfile.Load(cfg.ConfigDir)
	if err != nil {
		fmtError(err)
		return 1
	}
	findings, err := verifyPinnedTags(context.Background(), cfg, lock)
	if err != nil {
		fmtError(err)
		return 1
	}
	if reportTagFindings(findings) {
		return exitDrift
	}
	return 0
}

// exitOnMovedTags is the `generate --verify-tags` gate: it runs before anything
// is written and ends the process with exitDrift when a pinned tag moved.
func exitOnMovedTags(cfg *config.Config) {
	want, err := verifyTagsWanted(cfg, generateVerifyTags)
	if err != nil {
		fmtError(err)
		os.Exit(1)
	}
	if !want {
		return
	}
	lock, err := lockfile.Load(cfg.ConfigDir)
	if err != nil {
		fmtError(err)
		os.Exit(1)
	}
	findings, err := verifyPinnedTags(context.Background(), cfg, lock)
	if err != nil {
		fmtError(err)
		os.Exit(1)
	}
	if reportTagFindings(findings) {
		os.Exit(exitDrift)
	}
}
