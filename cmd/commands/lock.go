package commands

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/includes"
	"github.com/Goldziher/ai-rulez/internal/lockfile"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

var (
	lockCheck     bool
	lockRecursive bool
	lockKind      string
)

// LockCmd writes and verifies ai-rulez.lock.
var LockCmd = &cobra.Command{
	Use:   "lock [name...]",
	Short: "Pin remote includes and installed skills in ai-rulez.lock",
	Long: `Resolve every remote include and installed skill, and record the commit it
points to and a digest of the imported files in .ai-rulez/ai-rulez.lock.

Once the lock is committed, "generate" fetches exactly the pinned commits and
fails if the fetched files do not match the recorded digest, so output no longer
depends on where a branch points today. Names limit the refresh to those
includes or skills; the other pins are kept.

  ai-rulez lock                 pin everything (uses the network)
  ai-rulez lock shared          re-pin one include or skill
  ai-rulez lock --check         verify the lock against the config and the local
                                cache without using the network

The lock also pins [[skill_sources]] (commit and tree digest, kind "source") and
the digest of every skill the skills server would serve (kind "served"), which
[lock] enforce = true checks before serving.

Exit codes: 0 ok, 1 the command could not run, 2 --check found a stale lock.`,
	Run: runLock,
}

func init() {
	LockCmd.Flags().BoolVar(&lockCheck, "check", false, "Verify ai-rulez.lock against the configuration and cached content without writing or using the network")
	LockCmd.Flags().BoolVarP(&lockRecursive, "recursive", "r", false, "Process every configuration found recursively")
	LockCmd.Flags().StringVar(&lockKind, "kind", "", "Limit the refresh to include, skill, source or served entries")
	LockCmd.Flags().StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
}

func runLock(_ *cobra.Command, args []string) {
	if lockKind != "" && !knownLockKind(lockKind) {
		fmtError(oops.Errorf("unknown --kind %q (use include, skill, source or served)", lockKind))
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
		if lockCheck {
			c = checkLockAt(path)
		} else {
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
	includes.Mode = includes.LockRefresh
	includes.RefreshFilter = func(k, n string) bool {
		return (kind == "" || kind == k) && (len(wanted) == 0 || wanted[n])
	}
	includes.ResetObserved()
	defer func() { includes.Mode, includes.RefreshFilter = includes.LockAuto, nil }()

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
	next, problems := includes.BuildLock(cfg, current)
	if len(problems) > 0 {
		fmtError(oops.With("config", cfg.ConfigDir).Errorf("cannot write %s:\n  %s", lockfile.FileName, strings.Join(problems, "\n  ")))
		return 1
	}
	if problems := mergeDynamicLock(cfg, current, next, kind, wanted); len(problems) > 0 {
		fmtError(oops.With("config", cfg.ConfigDir).Errorf("cannot write %s:\n  %s", lockfile.FileName, strings.Join(problems, "\n  ")))
		return 1
	}
	for name := range wanted {
		if !lockHasName(next, name) {
			fmtError(oops.Errorf("%q is not a remote include or installed skill in %s", name, cfg.ConfigDir))
			return 1
		}
	}
	if len(next.Include)+len(next.Skill)+len(next.Source)+len(next.Served) == 0 {
		logger.Info("No remote includes, installed skills or skill sources to lock", "config", cfg.ConfigDir)
		return 0
	}
	if err := lockfile.Save(cfg.ConfigDir, next); err != nil {
		fmtError(err)
		return 1
	}
	for _, e := range lockedEntries(next) {
		fmt.Printf("locked %s %s %s\n", e.Name, shortSHA(e.Commit), e.Digest)
	}
	logger.Success("Wrote lock file", "path", lockfile.Path(cfg.ConfigDir))
	return 0
}

func checkLockAt(path string) int {
	cfg, err := loadForLock(path, config.WithoutLocal(), config.WithoutRemote())
	if err != nil {
		fmtError(err)
		return 1
	}
	lock, err := lockfile.Load(cfg.ConfigDir)
	if err != nil {
		fmtError(err)
		return 1
	}
	problems, cached := includes.CheckLock(cfg, lock)
	dynamic := checkDynamicLock(cfg, lock)
	if len(problems)+len(dynamic) > 0 {
		fmt.Fprintf(os.Stderr, "%s does not match %s:\n%s\nrun `ai-rulez lock` to refresh it\n", lockfile.FileName, cfg.ConfigDir,
			strings.TrimLeft(includes.FormatProblems(problems)+"\n"+strings.Join(dynamic, "\n"), "\n"))
		return exitDrift
	}
	logger.Success("Lock file is up to date", "config", cfg.ConfigDir, "verified_from_cache", cached)
	return 0
}

func shortSHA(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}
