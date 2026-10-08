package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"

	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/diag"
	"github.com/Goldziher/ai-rulez/v5/internal/forge"
	"github.com/Goldziher/ai-rulez/v5/internal/govview"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/lockrun"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/render"
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
	RunE: runLock,
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
	LockCmd.Flags().StringVar(&lockProfile, "profile", "", "Profile whose outputs are pinned (default: the profile recorded in the lock, else the config default)")
	LockCmd.Flags().BoolVarP(&lockRecursive, "recursive", "r", false, "Process every configuration found recursively")
	LockCmd.Flags().BoolVar(&lockRoles, "roles", false, "Also pin the rendered outputs of every role (roles with pin = true are always pinned)")
	LockCmd.Flags().StringVar(&lockKind, "kind", "", "Limit the refresh to include, skill, source or served entries")
	LockCmd.Flags().StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
}

// checkLockContentOnlyFlags refuses --content-only together with a selection of
// remote entries: --content-only keeps every remote pin, so the selection would
// silently refresh nothing.
func checkLockContentOnlyFlags(kind string, names []string) error {
	if !lockContentOnly {
		return nil
	}
	if len(names) > 0 || kind == lockfile.KindInclude || kind == lockfile.KindSkill || kind == keySource {
		return oops.Errorf("--content-only keeps every remote pin, so it cannot be combined with --kind %s or names; drop --content-only to refresh them", kindOrNames(kind))
	}
	return nil
}

func kindOrNames(kind string) string {
	if kind == "" {
		return "(names given)"
	}
	return kind
}

func runLock(_ *cobra.Command, args []string) error {
	if err := validateLockFlags(args); err != nil {
		return fail(err)
	}
	return exitStatus(runLockFor(lockKind, args))
}

// validateLockFlags rejects flag combinations `lock` cannot honor.
func validateLockFlags(args []string) error {
	if lockKind != "" && !knownLockKind(lockKind) {
		return oops.Errorf("unknown --kind %q (use include, skill, source or served)", lockKind)
	}
	if err := checkFormatFlag(lockFormat); err != nil {
		return err
	}
	if err := validateLockOutputFlags(args); err != nil {
		return err
	}
	if err := validateLockOutdatedFlags(); err != nil {
		return err
	}
	return validateLockRolesSubjectFlags(args)
}

// validateLockOutputFlags checks --format, --check/--diff and --output.
func validateLockOutputFlags(args []string) error {
	if lockFormat != "" && !lockCheck && !lockDiffFlag && !lockSubject && !lockOutdated {
		return oops.Errorf("--format applies to --check, --diff, --outdated and --subject only")
	}
	if lockCheck && lockDiffFlag {
		return oops.Errorf("--check and --diff are mutually exclusive")
	}
	if err := checkLockContentOnlyFlags(lockKind, args); err != nil {
		return err
	}
	if lockSubjectOutput != "" && !lockSubject {
		return oops.Errorf("--output needs --subject")
	}
	if lockSubjectOutput != "" && lockRecursive {
		return oops.Errorf("--output cannot be combined with --recursive: every root would overwrite the same file")
	}
	return nil
}

// validateLockOutdatedFlags checks --verify-tags and the --outdated family.
func validateLockOutdatedFlags() error {
	if lockVerifyTags && !lockCheck {
		return oops.Errorf("--verify-tags only applies to --check")
	}
	if (lockFailOnOutdated || lockOffline) && !lockOutdated {
		return oops.Errorf("--fail-on-outdated and --offline need --outdated")
	}
	if lockOutdated && (lockCheck || lockDiffFlag || lockContentOnly || lockSubject) {
		return oops.Errorf("--outdated only reads the remote: it cannot be combined with --check, --diff, --content-only or --subject")
	}
	return nil
}

// validateLockRolesSubjectFlags checks --roles and --subject against the rest.
func validateLockRolesSubjectFlags(args []string) error {
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
	defer func() { lockWarnings = nil }()
	code := 0
	for _, path := range paths {
		lockWarnings = onceCollector() // one per root: its loads say each warning once
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

// lockWarnings is the warning collector the loads of the root being locked
// share: lock loads the configuration more than once (the release-age gate,
// then the lock itself), and each load and render would repeat its warnings.
var lockWarnings *diag.Collector

// onceCollector returns a collector whose sink says each distinct warning once
// for its life, across loads and renders (a render starts a new de-duplication
// round of its own).
func onceCollector() *diag.Collector {
	var mu sync.Mutex
	said := map[string]bool{}
	out := diag.New(nil)
	return diag.New(func(msg string, args ...any) {
		key := msg + "\x00" + fmt.Sprint(args...)
		mu.Lock()
		first := !said[key]
		said[key] = true
		mu.Unlock()
		if first {
			out.Raise(msg, args...)
		}
	})
}

// loadForLock is loadForLockContext without a caller's context.
func loadForLock(path string, opts ...config.LoadOption) (*config.Config, error) {
	return loadForLockContext(cmdContext(), path, opts...)
}

// loadForLockContext is loadForLock under ctx (a lock run's context).
func loadForLockContext(ctx context.Context, path string, opts ...config.LoadOption) (*config.Config, error) {
	if lockWarnings != nil {
		opts = append(opts, config.WithCollector(lockWarnings))
	}
	// lock reports an include it cannot resolve as a problem of its own.
	ctx = config.WithUnresolvedIncludesTolerated(ctx)
	if path != "" {
		return loadProjectFile(ctx, path, opts...)
	}
	return loadConfigForCommand(ctx, nil, opts...)
}

// writeLockAt is writeLockAtContext without a caller's context.
func writeLockAt(path, kind string, names []string) int {
	return writeLockAtContext(cmdContext(), path, kind, names)
}

// writeLockAtContext writes the lock of the configuration at path under ctx.
func writeLockAtContext(ctx context.Context, path, kind string, names []string) int {
	res, err := lockrun.Write(ctx, lockRequest(kind, names), lockEnv(path))
	lockUnpinned = append(lockUnpinned, res.Unpinned...)
	if err != nil {
		var refused *lockrun.FindingsError
		if errors.As(err, &refused) && refused.Sources > 0 {
			fmt.Fprintln(os.Stderr, refused.Error())
		} else {
			renderError(os.Stderr, err)
		}
		return lockrun.ExitCode(res, err)
	}
	printLockWritten(res.Lock)
	printScans(res.Lock)
	logger.Success("Wrote lock file", "path", res.Path)
	return lockrun.ExitCode(res, nil)
}

// lockRequest is what the flags ask one `lock` run to pin.
func lockRequest(kind string, names []string) lockrun.Request {
	return lockrun.Request{Kind: kind, Names: names, ContentOnly: lockContentOnly, Profile: lockProfile, AllRoles: lockRoles,
		Roles: lockRoleNames(), AcceptFindings: lockAcceptFindings, Strict: lockStrict, Extras: lockExtraViews()}
}

// lockEnv is the command line as the host of a lock run over the root at path.
func lockEnv(path string) lockrun.Env {
	return lockrun.Env{
		Version: Version,
		Policy:  cliLockPolicy,
		Load: func(ctx context.Context, p config.LockPolicy, opts ...config.LoadOption) (*config.Config, error) {
			return loadForLockContext(ctx, path, append(opts, config.WithLockPolicy(p))...)
		},
		Collector: lockWarnings,
		Report:    os.Stderr,
		NewForge:  func(cfg *config.Config, offline bool) forge.Client { return newForgeClient(cfg, offline) },
		GitToken:  GetGitToken(),
		Tree:      strictTreeCache.Load,
		Cwd:       workingDir(),
	}
}

// printLockWritten lists what the written lock pins.
func printLockWritten(next *lockfile.File) {
	entries := lockrun.Entries(next)
	for i := range entries {
		e := &entries[i]
		fmt.Printf("locked %s %s %s\n", e.Name, shortSHA(e.Commit), e.Digest)
	}
	if next.HasContentPins() {
		fmt.Printf("pinned %d item(s) and %d output(s), tree %s\n", len(next.Item), len(next.DefaultOutputs()), next.Tree)
		for _, r := range next.RoleOutputs() {
			fmt.Printf("pinned outputs of role %s %s\n", r.Role, r.Digest)
		}
	}
}

// prepareLockRun sets the include policy of a run that refreshes the lock
// through the command line's loads (`update`): refresh the remotes, or stay
// offline, and returns the function that restores it.
func prepareLockRun(remoteRefresh bool, kind string, wanted map[string]bool) (restore func()) {
	prev := cliLockPolicy
	cliLockPolicy = lockrun.RunPolicy(cliLockPolicy, remoteRefresh, kind, wanted)
	return func() { cliLockPolicy = prev }
}

// nextLock builds the remote, source and served entries of the new lock (see
// lockrun.Next) with the serve-view flags.
func nextLock(ctx context.Context, cfg *config.Config, current *lockfile.File, kind string, wanted map[string]bool, remoteRefresh bool) (*lockfile.File, error) {
	req, env := lockRequest(kind, wantedNames(wanted)), lockEnv("")
	next, dyn, err := lockrun.Next(ctx, cfg, current, &req, &env, remoteRefresh)
	lockUnpinned = append(lockUnpinned, dyn.Unpinned...)
	return next, err //nolint:wrapcheck // already contextual
}

// pinContent adds the content pins to next (see lockrun.PinContent).
func pinContent(cfg *config.Config, current, next *lockfile.File, kind string, wanted map[string]bool) error {
	req := lockRequest(kind, wantedNames(wanted))
	return lockrun.PinContent(cfg, current, next, &req, Version) //nolint:wrapcheck // already contextual
}

func wantedNames(wanted map[string]bool) []string {
	names := make([]string, 0, len(wanted))
	for n := range wanted {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
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
func checkLockAt(path string) int { return checkLockOut(path, defaultOut()) }

// checkLockOut is checkLockAt writing its report and verdict to out, for a caller
// (publish) that embeds the check and keeps stdout for its own document.
func checkLockOut(path string, out render.Out) int {
	code, upToDate := checkLockContentAt(path, out)
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
func checkLockContentAt(path string, out render.Out) (code int, report func()) {
	cfg, remoteSkipped, err := loadForLockCheck(path)
	if err != nil {
		reportFailure(lockFormat, err)
		if errors.Is(err, config.ErrLockViolation) {
			return exitDrift, nil // fetched or cached remote content disagrees with the lock: drift, not a tool failure
		}
		return 1, nil
	}
	// A project that was never locked has nothing to verify: "up to date" would be a lie.
	// Under [lock] enforce the missing lock is a drift finding (exit 2) from the comparison below.
	if lock, loadErr := lockfile.Load(cfg.ConfigDir); loadErr == nil && lock == nil && !cfg.LockEnforced() {
		reportFailure(lockFormat, oops.Hint("run `ai-rulez lock` to create it").Errorf("no %s in %s: nothing to check", lockfile.FileName, cfg.ConfigDir))
		return 1, nil
	}
	diff, err := govview.CheckLockRoles(cmdContext(), cfg, remoteSkipped, lockProfile, Version, dynamicLockChanges, govview.RoleSelection{Only: lockRoleNames()})
	if err != nil {
		reportFailure(lockFormat, err)
		return 1, nil
	}
	if lockFormat == formatJSON {
		// The same document as the MCP lock_status tool; the exit code still gates.
		if err := diff.WriteJSON(os.Stdout); err != nil {
			renderError(os.Stderr, err)
			return 1, nil
		}
		if !diff.InSync {
			return exitDrift, nil
		}
		return 0, nil
	}
	if !diff.InSync {
		// The drift report is the result of --check: stdout, so -q keeps it; the remedy is a diagnostic.
		out.Result("%s does not match %s:\n", lockfile.FileName, cfg.ConfigDir)
		if werr := diff.WriteText(out.Stdout()); werr != nil {
			renderError(os.Stderr, werr)
		}
		fmt.Fprintln(os.Stderr, "run `ai-rulez lock` to refresh it (after reviewing the change with `ai-rulez lock --diff`)")
		return exitDrift, nil
	}
	for _, n := range diff.Notes {
		logger.Info(n)
	}
	// The verdict is the result of --check: stdout, so -q keeps it.
	return 0, func() { out.Result("Lock file is up to date (%s)\n", cfg.ConfigDir) }
}

// diffLockAt prints how the sources and outputs differ from the lock. It exits 0
// even when they differ: use --check to gate.
func diffLockAt(path string) int {
	cfg, remoteSkipped, err := loadForLockCheck(path)
	if err != nil {
		renderError(os.Stderr, err)
		return 1
	}
	lock, err := lockfile.Load(cfg.ConfigDir)
	if err != nil {
		renderError(os.Stderr, err)
		return 1
	}
	diff, err := lockDiff(cfg, lock, lockProfileFor(lock), remoteSkipped)
	if err != nil {
		renderError(os.Stderr, err)
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
		renderError(os.Stderr, err)
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
