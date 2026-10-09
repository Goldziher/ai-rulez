package commands

import (
	"context"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/lockrun"
	"github.com/Goldziher/ai-rulez/v5/internal/mcp"
)

// exitUnpinned is the exit code of a `lock` that wrote the lock but left served
// skills unpinned because the security scan refuses them (see --strict).
const exitUnpinned = lockrun.ExitUnpinned

// Flags that select the serve view `lock` pins besides the default one, the
// roles and the views the lock already records. They mirror `mcp --serve-skills`.
// --profile is shared with the output pins and also names a served view.
var (
	lockServeRole          string
	lockServeIncludeStatic bool
	lockServeTargets       string
	lockServeSources       []string
	lockStrict             bool
	// lockUnpinned collects the refusals of the current run (runLockFor resets it
	// per run); a root that left skills unpinned reports exitUnpinned.
	lockUnpinned []mcp.Refusal
)

func init() {
	f := LockCmd.Flags()
	f.StringVar(&lockServeRole, "role", "", "Also pin the skills this role serves, as a view of their own (see mcp --serve-skills --role)")
	f.StringVar(&lockServeTargets, "targets", "", "Also pin the view that serves this preset's rendering of the skills (see mcp --serve-skills --targets)")
	f.BoolVar(&lockServeIncludeStatic, "include-static", false, "Also pin the view that serves static skills too (see mcp --serve-skills --include-static)")
	f.StringArrayVar(&lockServeSources, "source", nil, "Also pin the view with this extra skill source, repeatable (see mcp --serve-skills --source)")
	f.BoolVar(&lockStrict, "refuse-findings", false, "Fail without writing when the security scan refuses any served skill (default: leave that skill unpinned, pin the rest and exit 3)")
}

// lockExtraViews is the view the serve-view flags select, if any.
func lockExtraViews() []mcp.ServeSetup {
	if lockServeRole == "" && lockProfile == "" && lockServeTargets == "" && !lockServeIncludeStatic && len(lockServeSources) == 0 {
		return nil
	}
	return []mcp.ServeSetup{{Role: lockServeRole, Profile: lockProfile, Preset: lockServeTargets, IncludeStatic: lockServeIncludeStatic, Sources: lockServeSources}}
}

func knownLockKind(kind string) bool { return lockrun.KnownKind(kind) }

// mergeDynamicLock refreshes the source and served pins of next (see
// lockrun.MergeDynamic) with the serve-view flags, and collects the skills left
// unpinned. scanOnly is true when every problem is a served skill the security
// scan refuses under --strict: findings, which exit 2, not a failure to run.
func mergeDynamicLock(ctx context.Context, cfg *config.Config, current, next *lockfile.File, kind string, wanted map[string]bool) (problems []string, scanOnly bool) {
	res := mergeDynamicViews(ctx, cfg, current, next, dynamicRun{kind: kind, wanted: wanted, extras: lockExtraViews(), strict: lockStrict})
	lockUnpinned = append(lockUnpinned, res.unpinned...)
	return res.problems, len(res.problems) > 0 && res.refused == len(res.problems)
}

// dynamicResult is what refreshing the dynamic pins found (lockrun.DynamicResult).
type dynamicResult struct {
	problems []string
	refused  int
	unpinned []mcp.Refusal
}

// dynamicRun is what one `lock` run refreshes (lockrun.DynamicRun).
type dynamicRun struct {
	kind   string
	wanted map[string]bool
	extras []mcp.ServeSetup
	strict bool
}

func mergeDynamicViews(ctx context.Context, cfg *config.Config, current, next *lockfile.File, run dynamicRun) dynamicResult {
	res := lockrun.MergeDynamic(ctx, cfg, current, next, lockrun.DynamicRun{Kind: run.kind, Wanted: run.wanted, Extras: run.extras, Strict: run.strict,
		Version: Version, Collector: lockWarnings})
	return dynamicResult{problems: res.Problems, refused: res.Refused, unpinned: res.Unpinned}
}

// dynamicLockChanges reports the source and served pins that disagree with the
// configuration and the local cache, as changes for `lock --check` and `--diff`.
func dynamicLockChanges(cfg *config.Config, lock *lockfile.File) []contentlock.Change {
	extras := lockExtraViews()
	if lockrun.ServesNothing(cfg, lock, extras) {
		return nil
	}
	return mcp.DynamicLockChanges(config.WithOfflineIncludes(cmdContext()), cfg, lock, Version, extras...)
}

// checkDynamicLock verifies the source and served pins against the configuration
// and the local cache without the network.
func checkDynamicLock(cfg *config.Config, lock *lockfile.File) []string {
	extras := lockExtraViews()
	if lockrun.ServesNothing(cfg, lock, extras) {
		return nil
	}
	return mcp.DynamicLockProblems(config.WithOfflineIncludes(cmdContext()), cfg, lock, Version, extras...)
}
