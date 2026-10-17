package commands

import (
	"context"

	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockrun"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

const (
	flagRefreshRemote  = "refresh-remote"
	flagAcceptFindings = "accept-findings"
)

// acceptFindingsUsage is the help text of generate's --accept-findings.
const acceptFindingsUsage = "With --refresh-remote or [lock] refresh_on_generate: pin a remote tree although its security scan has error findings (review them first), as 'ai-rulez lock --accept-findings'"

// refreshRemoteUsage is the help text of --refresh-remote.
const refreshRemoteUsage = "Before generating, re-resolve remote installed skills, includes and skill sources to their latest allowed version " +
	"and rewrite ai-rulez.lock; the security scan of the new trees runs first and a refusal aborts before any output is written " +
	"(or set [lock] refresh_on_generate)"

// refreshRemoteFlag reports whether --refresh-remote was given.
func refreshRemoteFlag(cmd *cobra.Command) bool {
	on, err := cmd.Flags().GetBool(flagRefreshRemote)
	return err == nil && on
}

// acceptFindingsFlag reports whether generate's --accept-findings was given.
func acceptFindingsFlag(cmd *cobra.Command) bool {
	on, err := cmd.Flags().GetBool(flagAcceptFindings)
	return err == nil && on
}

// refreshBlockedBy names the flag of this run that makes a lock refresh
// impossible or contradictory ("" when none): a refresh writes ai-rulez.lock and
// reads the network, so a run that must not write, must match the lock, or must
// stay offline cannot take it.
func refreshBlockedBy() string {
	for _, f := range []struct {
		name string
		set  bool
	}{
		{"--check", generateCheck}, {"--dry-run", dryRun}, {"--emit-plan", generateEmitPlan != ""},
		{"--locked", generateLocked}, {"--frozen", generateFrozen}, {"--offline", noFetch},
		{"--watch", generateWatch}, {"--recursive", recursive}, {"--user", userScope}, {"--plugin", pluginMode},
	} {
		if f.set {
			return f.name
		}
	}
	return ""
}

// checkRefreshFlags rejects --refresh-remote together with a flag that
// contradicts it.
func checkRefreshFlags(cmd *cobra.Command) error {
	if !refreshRemoteFlag(cmd) {
		return nil
	}
	if blocked := refreshBlockedBy(); blocked != "" {
		return oops.Hint("--refresh-remote rewrites ai-rulez.lock from the network before a single-project generate").
			Errorf("--refresh-remote cannot be combined with %s", blocked)
	}
	return nil
}

// refreshOnGenerate reports whether [lock] refresh_on_generate asks for a
// refresh in this run. A run that cannot refresh (see refreshBlockedBy) skips it.
func refreshOnGenerate(cfg *config.Config) bool {
	if cfg == nil || cfg.Lock == nil || !cfg.Lock.RefreshOnGenerate {
		return false
	}
	if blocked := refreshBlockedBy(); blocked != "" {
		logger.Info("[lock] refresh_on_generate is not applied", "reason", blocked)
		return false
	}
	return true
}

// refreshRemoteLock re-resolves every remote include, installed skill and skill
// source to the newest version its constraint allows (a plain ref follows its
// tip) and rewrites ai-rulez.lock, exactly as `ai-rulez lock` with every source
// advanced. The pre-pin security scan of lockrun.Write refuses a tree with error
// findings before the lock is saved, and a moved tag (AR732) or a downgrade is
// refused as in `update`; the import gate generate runs next scans the result
// again. A failure ends the run before any output is written.
func refreshRemoteLock(ctx context.Context) error {
	return refreshRemoteLockWith(ctx, false)
}

// refreshRemoteLockWith is refreshRemoteLock that pins a tree whose scan has
// error findings when acceptFindings is set, as `lock --accept-findings`.
func refreshRemoteLockWith(ctx context.Context, acceptFindings bool) error {
	defer applyLockFlags() // lockrun and the loads below change the command's lock policy
	env := lockEnv("")
	env.Policy.Advance = func(string, string) bool { return true }
	res, err := lockrun.Write(ctx, lockrun.Request{AcceptFindings: acceptFindings}, env)
	if err != nil {
		return failWithCode(lockrun.ExitCode(res, err), oops.Wrapf(err, "refresh the remote sources"))
	}
	if len(res.Unpinned) > 0 {
		logger.Warn("Some served skills were left unpinned because the security scan refuses them", "count", len(res.Unpinned))
	}
	logger.Info("Refreshed the remote sources", "lock", res.Path, "pinned", len(lockrun.Entries(res.Lock)))
	return nil
}
