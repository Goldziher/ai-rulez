package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

const formatText = "text"

var (
	lockCheck       bool
	lockDiffFlag    bool
	lockContentOnly bool
	lockFormat      string
	lockRecursive   bool
	lockKind        string
	lockProfile     string
)

// LockCmd writes and verifies ai-rulez.lock.
var LockCmd = &cobra.Command{
	Use:   "lock [name...]",
	Short: "Pin remote includes, installed skills and authored content in ai-rulez.lock",
	Long: `Resolve every remote include and installed skill, and record the commit it
points to and a digest of the imported files in .ai-rulez/ai-rulez.lock. The
lock also pins the authored content: a sha256 digest of every rule, skill (with
its resources), agent, command, context file, hook and role, and of the
generated outputs, plus one digest over the whole set. See docs/lockfile.md.

Once the lock is committed, "generate" fetches exactly the pinned commits and
fails if the fetched files do not match the recorded digest, so output no longer
depends on where a branch points today. Names limit the refresh to those
includes or skills; the other pins (and the content pins) are kept.

  ai-rulez lock                 pin everything (uses the network for remotes)
  ai-rulez lock shared          re-pin one include or skill
  ai-rulez lock --content-only  re-pin authored content and outputs, offline
  ai-rulez lock --check         verify the lock against the config, the local
                                cache and the sources, without the network
  ai-rulez lock --diff          show what "lock" would change (--format json)

--check names every added, removed or changed item and whether its source or its
generated output changed.

The lock also pins [[skill_sources]] (commit and tree digest, kind "source") and
the digest of every skill the skills server would serve (kind "served"), which
[lock] enforce = true checks before serving.

Exit codes: 0 ok, 1 the command could not run, 2 --check found a stale lock.`,
	Run: runLock,
}

func init() {
	LockCmd.Flags().BoolVar(&lockCheck, "check", false, "Verify ai-rulez.lock against the configuration and cached content without writing or using the network")
	LockCmd.Flags().BoolVar(&lockDiffFlag, "diff", false, "Show how the lock differs from the sources and outputs (for pull request review); exits 0")
	LockCmd.Flags().BoolVar(&lockContentOnly, "content-only", false, "Re-pin authored content and outputs only: no network, remote pins are kept")
	LockCmd.Flags().StringVar(&lockFormat, "format", "", "Output format of --diff: text (default) or json")
	LockCmd.Flags().StringVar(&lockProfile, "profile", "", "Profile whose outputs are pinned (default: the profile recorded in the lock, else the config default)")
	LockCmd.Flags().BoolVarP(&lockRecursive, "recursive", "r", false, "Process every configuration found recursively")
	LockCmd.Flags().StringVar(&lockKind, "kind", "", "Limit the refresh to include, skill, source or served entries")
	LockCmd.Flags().StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
}

func runLock(_ *cobra.Command, args []string) {
	if lockKind != "" && !knownLockKind(lockKind) {
		fmtError(oops.Errorf("unknown --kind %q (use include, skill, source or served)", lockKind))
		os.Exit(1)
	}
	if lockFormat != "" && lockFormat != formatText && lockFormat != formatJSON {
		fmtError(oops.Errorf("unknown --format %q (use text or json)", lockFormat))
		os.Exit(1)
	}
	if lockCheck && lockDiffFlag {
		fmtError(oops.Errorf("--check and --diff are mutually exclusive"))
		os.Exit(1)
	}
	if code := runLockFor(lockKind, args); code != 0 {
		os.Exit(code)
	}
}

// runLockFor locks (or checks) one root, or every root with --recursive. names
// restrict the refresh; kind restricts it to one entry kind.
func runLockFor(kind string, names []string) int {
	paths := []string{""}
	if lockRecursive {
		paths = findConfigFilesRecursively()
	}
	code := 0
	for _, path := range paths {
		var c int
		switch {
		case lockCheck:
			c = checkLockAt(path)
		case lockDiffFlag:
			c = diffLockAt(path)
		default:
			c = writeLockAt(path, kind, names)
		}
		if c > code {
			code = c
		}
	}
	return code
}

func loadForLock(path string, opts ...config.LoadOption) (*config.Config, error) {
	if path != "" {
		return config.LoadConfigFromFile(context.Background(), path, opts...)
	}
	return loadConfigForCommand(context.Background(), nil, opts...)
}

func writeLockAt(path, kind string, names []string) int {
	wanted := map[string]bool{}
	for _, n := range names {
		wanted[n] = true
	}
	remoteRefresh := !lockContentOnly
	defer prepareLockRun(remoteRefresh, kind, wanted)()

	cfg, err := loadForLock(path, config.WithoutLocal())
	if err != nil {
		fmtError(err)
		return 1
	}
	current, err := lockfile.Load(cfg.ConfigDir)
	if err != nil {
		fmtError(err)
		return 1
	}
	next, err := nextLock(cfg, current, kind, wanted, remoteRefresh)
	if err != nil {
		fmtError(err)
		return 1
	}
	if err := pinContent(cfg, current, next, kind, wanted); err != nil {
		fmtError(err)
		return 1
	}
	if err := lockfile.Save(cfg.ConfigDir, next); err != nil {
		fmtError(err)
		return 1
	}
	for _, e := range lockedEntries(next) {
		fmt.Printf("locked %s %s %s\n", e.Name, shortSHA(e.Commit), e.Digest)
	}
	if next.HasContentPins() {
		fmt.Printf("pinned %d item(s) and %d output(s), tree %s\n", len(next.Item), len(next.Output), next.Tree)
	}
	logger.Success("Wrote lock file", "path", lockfile.Path(cfg.ConfigDir))
	return 0
}

// prepareLockRun sets the include policy of a `lock` run (refresh the remotes, or
// stay offline for --content-only) and returns the function that restores it.
func prepareLockRun(remoteRefresh bool, kind string, wanted map[string]bool) (restore func()) {
	if remoteRefresh {
		includes.Mode = includes.LockRefresh
		includes.RefreshFilter = func(k, n string) bool {
			return (kind == "" || kind == k) && (len(wanted) == 0 || wanted[n])
		}
		includes.ResetObserved()
		return func() { includes.Mode, includes.RefreshFilter = includes.LockAuto, nil }
	}
	prev := includes.SkipFetch
	includes.SkipFetch = true
	return func() { includes.SkipFetch = prev }
}

// nextLock builds the remote, source and served entries of the new lock. The
// content pins are added by pinContent.
func nextLock(cfg *config.Config, current *lockfile.File, kind string, wanted map[string]bool, remoteRefresh bool) (*lockfile.File, error) {
	next := &lockfile.File{Version: lockfile.Version}
	if remoteRefresh {
		var problems []string
		next, problems = includes.BuildLock(cfg, current)
		if len(problems) > 0 {
			return nil, oops.With("config", cfg.ConfigDir).Errorf("cannot write %s:\n  %s", lockfile.FileName, strings.Join(problems, "\n  "))
		}
	} else if current != nil {
		next.Include, next.Skill = current.Include, current.Skill
	}

	// Source and served pins: refreshed with the remote pins; --content-only
	// refreshes only the served digests, and only when that works offline.
	dynamicKind := kind
	if !remoteRefresh {
		dynamicKind = lockfile.KindServed
	}
	if problems := mergeDynamicLock(cfg, current, next, dynamicKind, wanted); len(problems) > 0 {
		if remoteRefresh {
			return nil, oops.With("config", cfg.ConfigDir).Errorf("cannot write %s:\n  %s", lockfile.FileName, strings.Join(problems, "\n  "))
		}
		logger.Warn("Kept the served pins: they cannot be recomputed offline", "problems", strings.Join(problems, "; "))
	}
	for name := range wanted {
		if !lockHasName(next, name) {
			return nil, oops.Errorf("%q is not a remote include, installed skill, skill source or served skill in %s", name, cfg.ConfigDir)
		}
	}
	return next, nil
}

// pinContent adds the content pins to next. A refresh limited to some remote
// sources leaves the content pins alone; otherwise they are recomputed from the
// sources on disk.
func pinContent(cfg *config.Config, current, next *lockfile.File, kind string, wanted map[string]bool) error {
	if len(wanted) == 0 && kind == "" {
		profileName := lockProfile
		if profileName == "" && current != nil {
			profileName = current.Profile
		}
		snap, err := lockSnapshot(cfg, profileName, false)
		if err != nil {
			return err
		}
		for _, problem := range snap.Problems {
			logger.Warn("Cannot pin part of the configuration; lock --check will fail until it is fixed", "problem", problem)
		}
		contentlock.Build(next, snap)
		return nil
	}
	if current == nil {
		return nil
	}
	next.HashVersion, next.AIRulezVersion, next.Profile = current.HashVersion, current.AIRulezVersion, current.Profile
	next.Scope, next.OutputsPinned = current.Scope, current.OutputsPinned
	next.Item, next.Output = current.Item, current.Output
	if next.HasContentPins() {
		next.Tree = contentlock.TreeOf(next)
	}
	return nil
}

// lockProfileFor picks the profile a check renders: --profile, else the one the
// lock recorded.
func lockProfileFor(lock *lockfile.File) string {
	if lockProfile != "" {
		return lockProfile
	}
	if lock != nil {
		return lock.Profile
	}
	return ""
}

func checkLockAt(path string) int {
	cfg, remoteSkipped, err := loadForLockCheck(path)
	if err != nil {
		fmtError(err)
		if errors.Is(err, config.ErrLockViolation) {
			return exitDrift // fetched or cached remote content disagrees with the lock: drift, not a tool failure
		}
		return 1
	}
	lock, err := lockfile.Load(cfg.ConfigDir)
	if err != nil {
		fmtError(err)
		return 1
	}
	diff, err := lockDiff(cfg, lock, lockProfileFor(lock), remoteSkipped)
	if err != nil {
		fmtError(err)
		return 1
	}
	if !diff.InSync {
		fmt.Fprintf(os.Stderr, "%s does not match %s:\n", lockfile.FileName, cfg.ConfigDir)
		if werr := diff.WriteText(os.Stderr); werr != nil {
			fmtError(werr)
		}
		fmt.Fprintln(os.Stderr, "run `ai-rulez lock` to refresh it (after reviewing the change with `ai-rulez lock --diff`)")
		return exitDrift
	}
	for _, n := range diff.Notes {
		logger.Info(n)
	}
	logger.Success("Lock file is up to date", "config", cfg.ConfigDir)
	return 0
}

// diffLockAt prints how the sources and outputs differ from the lock. It exits 0
// even when they differ: use --check to gate.
func diffLockAt(path string) int {
	cfg, remoteSkipped, err := loadForLockCheck(path)
	if err != nil {
		fmtError(err)
		return 1
	}
	lock, err := lockfile.Load(cfg.ConfigDir)
	if err != nil {
		fmtError(err)
		return 1
	}
	diff, err := lockDiff(cfg, lock, lockProfileFor(lock), remoteSkipped)
	if err != nil {
		fmtError(err)
		return 1
	}
	switch {
	case lockFormat == formatJSON:
		err = diff.WriteJSON(os.Stdout)
	case diff.InSync:
		fmt.Println("ai-rulez.lock matches the sources and outputs")
		err = diff.WriteText(os.Stdout)
	default:
		err = diff.WriteText(os.Stdout)
	}
	if err != nil {
		fmtError(err)
		return 1
	}
	return 0
}

func shortSHA(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}
