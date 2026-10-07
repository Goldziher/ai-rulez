package commands

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/contentlock"
	"github.com/Goldziher/ai-rulez/v5/internal/govview"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

var (
	lockCheck       bool
	lockDiffFlag    bool
	lockContentOnly bool
	lockFormat      string
	lockRecursive   bool
	lockKind        string
	lockProfile     string
	lockRoles       bool

	lockSubject       bool
	lockSubjectOutput string
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
  ai-rulez lock --check --format json
                                the same comparison as JSON on stdout (the
                                MCP lock_status tool returns this document)
  ai-rulez lock --outdated      list sources whose version constraint allows a
                                newer tag than the pinned one (network; --format
                                json, --fail-on-outdated); "ai-rulez update"
                                moves the pins
  ai-rulez lock --roles         also pin the rendered outputs of every role
                                (roles with pin = true are always pinned);
                                --role r pins the outputs of r too, and limits
                                the role comparison of --check and --diff to r
  ai-rulez lock --subject       print the lock-subject digest to sign with
                                cosign (--output <file> writes the statement,
                                --format json prints it); offline, read-only

--check names every added, removed or changed item and whether its source or its
generated output changed.

The lock also pins [[skill_sources]] (commit and tree digest, kind "source") and
the digest of every skill the skills server would serve (kind "served"), which
[lock] enforce = true checks before serving.

Before a remote tree is pinned to something new, it gets the security scan
(AR001-AR009), as "update" does: error findings refuse the pin (exit 2, nothing
written) unless --accept-findings.

Exit codes: 0 ok; 1 the command could not run (a tool error); 2 --check found
a stale lock (drift) or the pre-pin scan refused a tree; 3 the lock was written but served skills were left
unpinned because the security scan refuses them. Over several roots
(--recursive) the most severe code wins: 1, then 2, then 3.`,
	Run: runLock,
}

func init() {
	LockCmd.Flags().BoolVar(&lockCheck, "check", false, "Verify ai-rulez.lock against the configuration and cached content without writing or using the network")
	LockCmd.Flags().BoolVar(&lockDiffFlag, "diff", false, "Show how the lock differs from the sources and outputs (for pull request review); exits 0")
	LockCmd.Flags().BoolVar(&lockContentOnly, "content-only", false, "Re-pin authored content and outputs only: no network, remote pins are kept")
	LockCmd.Flags().BoolVar(&lockOutdated, "outdated", false, "Report sources whose version constraint allows a newer tag than the pinned one (uses the network, writes nothing)")
	LockCmd.Flags().BoolVar(&lockFailOnOutdated, "fail-on-outdated", false, "With --outdated: exit 2 when any source has an allowed update")
	LockCmd.Flags().BoolVar(&lockOffline, "offline", false, "With --outdated: refuse to run (it needs the network); use --check to verify the lock offline")
	LockCmd.Flags().BoolVar(&lockAcceptFindings, "accept-findings", false, "Pin a source although the security scan of its new tree has error findings (review them first)")
	LockCmd.Flags().BoolVar(&lockSubject, "subject", false, "Print the lock-subject digest and statement (the thing to sign); reads the lock only")
	LockCmd.Flags().StringVar(&lockSubjectOutput, "output", "", "With --subject: write the JSON statement to this file")
	addFormatFlag(LockCmd.Flags(), &lockFormat, "", formatText, formatText, formatJSON) // of --check, --diff, --outdated and --subject
	addJSONFlagAlias(LockCmd.Flags())
	LockCmd.Flags().StringVar(&lockProfile, "profile", "", "Profile whose outputs are pinned (default: the profile recorded in the lock, else the config default)")
	LockCmd.Flags().BoolVarP(&lockRecursive, "recursive", "r", false, "Process every configuration found recursively")
	LockCmd.Flags().BoolVar(&lockRoles, "roles", false, "Also pin the rendered outputs of every role (roles with pin = true are always pinned)")
	LockCmd.Flags().StringVar(&lockKind, "kind", "", "Limit the refresh to include, skill, source or served entries")
	LockCmd.Flags().StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
}

func runLock(_ *cobra.Command, args []string) {
	if err := validateLockFlags(args); err != nil {
		fmtError(err)
		os.Exit(1)
	}
	if code := runLockFor(lockKind, args); code != 0 {
		os.Exit(code)
	}
}

// validateLockFlags rejects flag combinations `lock` cannot honour.
func validateLockFlags(args []string) error {
	if lockKind != "" && !knownLockKind(lockKind) {
		return oops.Errorf("unknown --kind %q (use include, skill, source or served)", lockKind)
	}
	if err := checkFormatFlag(lockFormat); err != nil {
		return err
	}
	if lockFormat != "" && !lockCheck && !lockDiffFlag && !lockSubject && !lockOutdated {
		return oops.Errorf("--format applies to --check, --diff, --outdated and --subject only")
	}
	if lockCheck && lockDiffFlag {
		return oops.Errorf("--check and --diff are mutually exclusive")
	}
	if lockSubjectOutput != "" && !lockSubject {
		return oops.Errorf("--output needs --subject")
	}
	if lockSubjectOutput != "" && lockRecursive {
		return oops.Errorf("--output cannot be combined with --recursive: every root would overwrite the same file")
	}
	if lockVerifyTags && !lockCheck {
		return oops.Errorf("--verify-tags only applies to --check")
	}
	if (lockFailOnOutdated || lockOffline) && !lockOutdated {
		return oops.Errorf("--fail-on-outdated and --offline need --outdated")
	}
	if lockOutdated && (lockCheck || lockDiffFlag || lockContentOnly || lockSubject) {
		return oops.Errorf("--outdated only reads the remote: it cannot be combined with --check, --diff, --content-only or --subject")
	}
	if lockRoles && (lockCheck || lockDiffFlag || lockSubject || lockOutdated) {
		return oops.Errorf("--roles pins every role: use it without --check, --diff, --outdated or --subject (name roles with --role to limit a check)")
	}
	if lockRoles && (lockKind != "" || len(args) > 0) {
		return oops.Errorf("--roles pins rendered outputs: it cannot be combined with --kind or names")
	}
	if lockSubject && (lockCheck || lockDiffFlag || lockContentOnly || lockKind != "" || len(args) > 0) {
		return oops.Errorf("--subject only reads the lock: it cannot be combined with --check, --diff, --content-only, --kind or names")
	}
	return nil
}

// runLockFor locks (or checks) one root, or every root with --recursive. names
// restrict the refresh; kind restricts it to one entry kind.
func runLockFor(kind string, names []string) int {
	paths := []string{""}
	if lockRecursive {
		paths = findConfigFilesRecursively()
	}
	lockUnpinned = nil // per run: a long-lived process must not carry refusals over
	code := 0
	for _, path := range paths {
		var c int
		switch {
		case lockSubject:
			c = lockSubjectAt(path)
		case lockOutdated:
			c = outdatedAt(path, kind, names)
		case lockCheck:
			c = checkLockAt(path)
		case lockDiffFlag:
			c = diffLockAt(path)
		default:
			c = writeLockAt(path, kind, names)
		}
		code = worstExit(code, c)
	}
	if len(lockUnpinned) > 0 {
		fmt.Fprintf(os.Stderr, "%d served skill(s) were left unpinned because the security scan refuses them; fix them, or use --strict to fail instead\n", len(lockUnpinned))
	}
	return code
}

func loadForLock(path string, opts ...config.LoadOption) (*config.Config, error) {
	// lock reports an include it cannot resolve as a problem of its own.
	ctx := config.WithUnresolvedIncludesTolerated(cmdContext())
	if path != "" {
		return loadProjectFile(ctx, path, opts...)
	}
	return loadConfigForCommand(ctx, nil, opts...)
}

func writeLockAt(path, kind string, names []string) int {
	wanted := map[string]bool{}
	for _, n := range names {
		wanted[n] = true
	}
	remoteRefresh := !lockContentOnly
	unpinnedBefore := len(lockUnpinned)
	if remoteRefresh {
		defer installReleaseGate(path)() // min_release_age holds back young tags while ranges resolve
	}
	defer prepareLockRun(remoteRefresh, kind, wanted)()

	cfg, err := loadForLock(path, config.WithoutLocal())
	if err != nil {
		fmtError(err)
		return 1
	}
	// Pin only what generate would accept: a lock for a configuration that
	// fails validation would record content no run can use.
	if err := cfg.Validate(); err != nil {
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
	carryApprovals(current, next, len(wanted) == 0 && kind == "")
	if err := deniedPinsError(next); err != nil {
		fmtError(err)
		return 1
	}
	if refused := scanNewPins(cfg, current, next); refused > 0 {
		fmt.Fprintf(os.Stderr, "refused %d source(s): the security scan of the new tree has error findings; review them, then pass --accept-findings. Nothing was written\n", refused)
		return exitDrift
	}
	pinScans(cfg, current, next, len(wanted) == 0 && kind == "")
	if err := lockfile.Save(cfg.ConfigDir, next); err != nil {
		fmtError(err)
		return 1
	}
	for _, e := range lockedEntries(next) {
		fmt.Printf("locked %s %s %s\n", e.Name, shortSHA(e.Commit), e.Digest)
	}
	if next.HasContentPins() {
		fmt.Printf("pinned %d item(s) and %d output(s), tree %s\n", len(next.Item), len(next.DefaultOutputs()), next.Tree)
		for _, r := range next.RoleOutputs() {
			fmt.Printf("pinned outputs of role %s %s\n", r.Role, r.Digest)
		}
	}
	printScans(next)
	logger.Success("Wrote lock file", "path", lockfile.Path(cfg.ConfigDir))
	if len(lockUnpinned) > unpinnedBefore {
		return exitUnpinned
	}
	return 0
}

// prepareLockRun sets the include policy of a `lock` run (refresh the remotes, or
// stay offline for --content-only) and returns the function that restores it.
func prepareLockRun(remoteRefresh bool, kind string, wanted map[string]bool) (restore func()) {
	prev := cliLockPolicy
	if remoteRefresh {
		cliLockPolicy.Mode = config.LockRefresh
		cliLockPolicy.Refresh = func(k, n string) bool {
			return (kind == "" || kind == k) && (len(wanted) == 0 || wanted[n])
		}
		includes.ResetObserved()
	} else {
		cliLockPolicy.Offline = true
	}
	return func() { cliLockPolicy = prev }
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
		snap, err := lockRoleSnapshot(cfg, profileName, false, govview.RoleSelection{Write: true, All: lockRoles, Lock: current, Only: lockRoleNames()})
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
	next.AIRulezVersion, next.Profile = current.AIRulezVersion, current.Profile
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

// checkLockAt is `lock --check`: the content comparison, then the attestation
// check when [signing] require names the lock.
func checkLockAt(path string) int {
	code, upToDate := checkLockContentAt(path)
	if code == 1 {
		return code
	}
	// verifyTagsAt decides whether to ask the remotes: --verify-tags or [lock] verify_tags.
	c := verifyTagsAt(path)
	if c == 1 {
		return c
	}
	code = worstExit(code, c)
	code = worstExit(code, checkLockSignatureAt(path))
	if code == 0 && upToDate != nil {
		upToDate()
	}
	return code
}

// checkLockContentAt compares the lock with the sources. The second result is
// the success report, run by the caller once every other check has passed too, so
// "up to date" is never printed before a signature failure.
func checkLockContentAt(path string) (int, func()) {
	cfg, remoteSkipped, err := loadForLockCheck(path)
	if err != nil {
		fmtError(err)
		if errors.Is(err, config.ErrLockViolation) {
			return exitDrift, nil // fetched or cached remote content disagrees with the lock: drift, not a tool failure
		}
		return 1, nil
	}
	// A project that was never locked has nothing to verify: "up to date" would be a lie.
	// Under [lock] enforce the missing lock is a drift finding (exit 2) from the comparison below.
	if lock, loadErr := lockfile.Load(cfg.ConfigDir); loadErr == nil && lock == nil && !cfg.LockEnforced() {
		fmtError(oops.Hint("run `ai-rulez lock` to create it").Errorf("no %s in %s: nothing to check", lockfile.FileName, cfg.ConfigDir))
		return 1, nil
	}
	diff, err := govview.CheckLockRoles(cfg, remoteSkipped, lockProfile, Version, dynamicLockChanges, govview.RoleSelection{Only: lockRoleNames()})
	if err != nil {
		fmtError(err)
		return 1, nil
	}
	if lockFormat == formatJSON {
		// The same document as the MCP lock_status tool; the exit code still gates.
		if err := diff.WriteJSON(os.Stdout); err != nil {
			fmtError(err)
			return 1, nil
		}
		if !diff.InSync {
			return exitDrift, nil
		}
		return 0, nil
	}
	if !diff.InSync {
		fmt.Fprintf(os.Stderr, "%s does not match %s:\n", lockfile.FileName, cfg.ConfigDir)
		if werr := diff.WriteText(os.Stderr); werr != nil {
			fmtError(werr)
		}
		fmt.Fprintln(os.Stderr, "run `ai-rulez lock` to refresh it (after reviewing the change with `ai-rulez lock --diff`)")
		return exitDrift, nil
	}
	for _, n := range diff.Notes {
		logger.Info(n)
	}
	return 0, func() { logger.Success("Lock file is up to date", "config", cfg.ConfigDir) }
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
