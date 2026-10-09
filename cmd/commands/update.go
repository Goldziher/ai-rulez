package commands

import (
	"fmt"
	"sort"

	"github.com/samber/oops"
	"github.com/spf13/cobra"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/lockrun"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/tagresolve"
)

// UpdateSchemaVersion versions the JSON of `update --format json`.
const UpdateSchemaVersion = 1

var (
	updateDryRun         bool
	updateAllowDowngrade bool
	updateAcceptMoved    bool
	updateKind           string
	updateFormat         string
	updateOffline        bool
	updateMajor          bool
	updateWriteConfig    bool
	updateAcceptFindings bool
)

// UpdateCmd moves the pins of sources that use a version constraint.
var UpdateCmd = &cobra.Command{
	Use:   "update [name...]",
	Short: "Move the pins of sources that use a version constraint to the newest allowed tag",
	Long: `Move the lock entries of remote includes, installed skills and skill sources
that ask for a version range (version = ^1.2) to the newest tag the range
allows, then record the tag, its commit and the tree digest in ai-rulez.lock.
Names limit the update to those sources; --kind limits it to include, skill or
source. Only the lock changes: config.toml is edited only by --major --write-config.

Nothing else moves a range pin: "generate" never resolves a range and
"lock" keeps a pin that still satisfies its constraint. "update" is the one
deliberate step, and it prints what changes first:

  ai-rulez lock --outdated          which sources have newer tags
  ai-rulez update --dry-run         what update would change, writes nothing
  ai-rulez update shared            move one source
  ai-rulez update                   move every source that has an update

A source may hold tags back with min_release_age ("7d"): update takes the
newest tag that is old enough and says which newer tags it held back (AR733).
Before a pin is written, the new tree gets the security scan (AR001-AR009) and
a pin whose scan has error findings is refused unless --accept-findings.

Major versions: --major lists the sources that have a newer major version than
their constraint allows and the constraint that would take it (version = "^2.0").
--write-config applies that: it rewrites only the version line of that source in
config.toml (comments and layout stay) and moves the pin; without it nothing is
written. Other sources are left alone by --major.

Defenses:
  - A tag that moved since it was pinned (AR732) is refused. Review the new
    commit, then --accept-moved-tag re-pins that tag.
  - update never selects a tag below the pinned one (a truncated tag list can
    roll a pin back) unless --allow-downgrade.
  - A constraint no tag satisfies (AR730) is an error.

Plain "ref" sources are not touched: "ai-rulez lock <name>" and
"ai-rulez skill update" re-resolve those (a branch follows its tip). They keep
a range pin as it is; use "update" to move it.

Exit codes: 0 done (or nothing to do); 1 the command could not run (network,
tool error); 2 a source was refused (AR732, AR730, AR731, scan findings) and
nothing was written.`,
	RunE: func(_ *cobra.Command, args []string) error {
		return runUpdate(args)
	},
}

func init() {
	f := UpdateCmd.Flags()
	specDryRun.Bool(f, &updateDryRun, "Show what would change and write nothing")
	f.BoolVar(&updateAllowDowngrade, "allow-downgrade", false, "Allow a tag with lower precedence than the pinned one")
	f.BoolVar(&updateMajor, "major", false, "Handle only sources that have a newer major version: print the constraint that would take it")
	f.BoolVar(&updateWriteConfig, "write-config", false, "With --major, rewrite the version line of those sources in config.toml and move their pins")
	f.BoolVar(&updateAcceptFindings, "accept-findings", false, "Write a pin although the security scan of the new tree has error findings (review them first)")
	f.BoolVar(&updateAcceptMoved, "accept-moved-tag", false, "Re-pin a tag that now points to another commit (AR732) after you reviewed it")
	f.StringVar(&updateKind, "kind", "", "Limit the update to include, skill or source")
	addFormatFlag(f, &updateFormat, "", formatText, formatText, formatJSON)
	f.BoolVar(&updateOffline, "offline", false, "Refuse to run: update reads the remote's tags")
	RootCmd.AddCommand(UpdateCmd)
}

// updateItem is one pin that moves.
type updateItem struct {
	Kind      string                  `json:"kind"`
	Name      string                  `json:"name"`
	From      *tagresolve.TagRef      `json:"from,omitempty"`
	To        *tagresolve.TagRef      `json:"to"`
	DigestOld string                  `json:"digest_old,omitempty"`
	DigestNew string                  `json:"digest_new,omitempty"`
	Files     []tagresolve.FileChange `json:"files,omitempty"`
	Reason    string                  `json:"reason,omitempty"`
	// Held lists the newer tags min_release_age held back (AR733).
	Held []tagresolve.Held `json:"held_back,omitempty"`
	// Released and ReleasedFrom are the recorded release time of the new tag.
	Released     string `json:"released,omitempty"`
	ReleasedFrom string `json:"released_from,omitempty"`
	// Scan is the security scan of the new tree.
	Scan *scanSummary `json:"scan,omitempty"`
}

// scanSummary is the security scan (AR001-AR009) of a tree about to be pinned.
type scanSummary = lockrun.ScanSummary

// updateReport is the JSON document of `update --format json`.
type updateReport struct {
	SchemaVersion int                `json:"schema_version"`
	DryRun        bool               `json:"dry_run"`
	Updates       []updateItem       `json:"updates"`
	Blocked       []tagresolve.Row   `json:"blocked,omitempty"`
	Notes         []string           `json:"notes,omitempty"`
	Unchanged     []tagresolve.Row   `json:"unchanged,omitempty"`
	Major         []majorItem        `json:"major,omitempty"`
	moves         map[string]*moveTo `json:"-"`
}

type moveTo struct {
	row tagresolve.Row
	src versionSrc
}

func runUpdate(names []string) error {
	if updateKind != "" && updateKind != lockfile.KindInclude && updateKind != lockfile.KindSkill && updateKind != lockfile.KindSource {
		return fail(oops.Errorf("unknown --kind %q (use include, skill or source)", updateKind))
	}
	if updateWriteConfig && !updateMajor {
		return fail(oops.Hint("config.toml is edited only to take a new major version: `ai-rulez update --major --write-config`").
			Errorf("--write-config needs --major"))
	}
	if err := checkFormatFlag(updateFormat); err != nil {
		return fail(err)
	}
	lockOffline = updateOffline
	if err := requireOnline("update"); err != nil {
		return fail(err)
	}
	return updateAt("", updateKind, names)
}

func moveKey(kind, name string) string { return kind + "\x00" + name }

// updateRefreshFilter limits the remote refresh of an update to the sources
// that move, matched by kind and name (the name alone also matched a same-named
// include, skill or source of another kind).
func updateRefreshFilter(moves map[string]*moveTo) func(kind, name string) bool {
	return func(kind, name string) bool { return moves[moveKey(kind, name)] != nil }
}

func updateAt(path, kind string, names []string) error {
	ctx := cmdContext()
	cfg, err := loadForLock(path, config.WithoutLocal(), config.WithoutRemote())
	if err != nil {
		return fail(err)
	}
	defer installReleaseGateFor(cfg)()
	current, err := lockfile.Load(cfg.ConfigDir)
	if err != nil {
		return fail(err)
	}
	wanted := map[string]bool{}
	for _, n := range names {
		wanted[n] = true
	}
	srcs := versionSources(cfg, kind, wanted)
	if err := checkNamesMatched(srcs, wanted); err != nil {
		return fail(err)
	}
	rows, err := evaluateSources(ctx, srcs, current)
	if err != nil {
		return fail(err)
	}
	report := &updateReport{SchemaVersion: UpdateSchemaVersion, DryRun: updateDryRun, Updates: []updateItem{}, moves: map[string]*moveTo{}}
	if updateMajor {
		return majorUpdate(path, cfg, current, srcs, rows, report)
	}
	return finishUpdate(report, planAndApply(path, cfg, current, srcs, rows, report))
}

// planAndApply sorts the evaluated sources into moves, refusals and no-ops, then
// writes the moves unless something is refused or this is a dry run. It returns
// nil, an exitFindings ExitError for a refusal, or the failure; printing the
// report is left to the caller.
func planAndApply(path string, cfg *config.Config, current *lockfile.File, srcs []versionSrc, rows []tagresolve.Row, report *updateReport) error {
	planUpdates(report, srcs, rows)
	if len(report.Blocked) > 0 {
		return exitStatus(exitFindings)
	}
	if len(report.moves) == 0 {
		return nil
	}
	refused, err := applyUpdates(path, cfg, current, srcs, report)
	if err != nil {
		return fail(err)
	}
	if refused {
		return exitStatus(exitFindings)
	}
	return nil
}

func checkNamesMatched(srcs []versionSrc, wanted map[string]bool) error {
	found := map[string]bool{}
	for i := range srcs {
		s := &srcs[i]
		found[s.name] = true
	}
	var missing []string
	for n := range wanted {
		if !found[n] {
			missing = append(missing, n)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		return oops.Hint("`ai-rulez lock --outdated` lists the sources that use a version constraint").
			Errorf("%v: not a remote include, installed skill or skill source with a version constraint", missing)
	}
	return nil
}

// planUpdates sorts the evaluated sources into moves, refusals and no-ops.
func planUpdates(rep *updateReport, srcs []versionSrc, rows []tagresolve.Row) {
	for i := range rows {
		row := &rows[i]
		src := srcs[i]
		// A moved tag is a finding before it is a downgrade: it must never land in Unchanged.
		if row.Downgrade && !updateAllowDowngrade && row.Status != tagresolve.StatusTagMoved {
			rep.Unchanged = append(rep.Unchanged, *row)
			continue
		}
		switch row.Status {
		case tagresolve.StatusUpdatable, tagresolve.StatusNotLocked, tagresolve.StatusTagMissing:
			rep.moves[moveKey(row.Kind, row.Name)] = &moveTo{row: *row, src: src}
		case tagresolve.StatusTagMoved:
			switch {
			case !updateAcceptMoved:
				rep.Blocked = append(rep.Blocked, *row)
			case row.Downgrade && !updateAllowDowngrade:
				rep.Unchanged = append(rep.Unchanged, *row)
			default:
				rep.moves[moveKey(row.Kind, row.Name)] = &moveTo{row: *row, src: src}
				rep.Notes = append(rep.Notes, fmt.Sprintf("%s %s: accepted the moved tag %s", row.Kind, row.Name, row.Locked.Tag))
			}
		case tagresolve.StatusLockedNonVersion:
			// Moving off a pin that is not a version has no ordering to protect: ask for the same consent as a downgrade.
			if updateAllowDowngrade {
				rep.moves[moveKey(row.Kind, row.Name)] = &moveTo{row: *row, src: src}
			} else {
				rep.Unchanged = append(rep.Unchanged, *row)
			}
		case tagresolve.StatusUnsatisfied, tagresolve.StatusInvalid:
			rep.Blocked = append(rep.Blocked, *row)
		case tagresolve.StatusLowerOnly:
			if updateAllowDowngrade {
				rep.moves[moveKey(row.Kind, row.Name)] = &moveTo{row: *row, src: src}
			} else {
				rep.Unchanged = append(rep.Unchanged, *row)
			}
		default:
			rep.Unchanged = append(rep.Unchanged, *row)
		}
	}
}

// applyUpdates re-resolves the moving sources the way `lock <names>` does,
// advancing them to their allowed tag, and writes the lock unless --dry-run.
//
// It returns refused = true (and writes nothing) when the security scan of a new
// tree has error findings and --accept-findings was not given.
func applyUpdates(path string, cfg *config.Config, current *lockfile.File, srcs []versionSrc, rep *updateReport) (refused bool, err error) {
	// Make sure the cache holds the pinned trees (a dry run, or another project,
	// may have left a newer one), so the before/after comparison is accurate.
	if _, err := loadForLock(path, config.WithoutLocal()); err != nil {
		return false, err
	}
	before := map[string]map[string]string{}
	wanted := map[string]bool{}
	for key, m := range rep.moves {
		wanted[m.row.Name] = true
		before[key] = hashTree(m.src.treeDir, entryCommit(current, m.row))
	}
	cliLockPolicy.Advance = func(kind, name string) bool { return rep.moves[moveKey(kind, name)] != nil }
	cliLockPolicy.AllowDowngrade, cliLockPolicy.AcceptMovedTag = updateAllowDowngrade, updateAcceptMoved
	defer func() {
		cliLockPolicy.Advance, cliLockPolicy.AllowDowngrade, cliLockPolicy.AcceptMovedTag = nil, false, false
	}()
	defer prepareLockRun(true, "", wanted)()
	cliLockPolicy.Refresh = updateRefreshFilter(rep.moves) // by kind and name: a same-named source of another kind stays put

	fresh, err := loadForLock(path, config.WithoutLocal())
	if err != nil {
		return false, err
	}
	if err := fresh.Validate(); err != nil {
		return false, err //nolint:wrapcheck // already contextual
	}
	next, err := nextLock(cmdContext(), fresh, current, "", wanted, true)
	if err != nil {
		return false, err
	}
	if err := pinContent(fresh, current, next, "", wanted); err != nil {
		return false, err
	}
	for _, key := range sortedKeys(rep.moves) {
		item, err := updateItemFor(fresh, current, next, rep.moves[key], before[key])
		if err != nil {
			return false, err
		}
		refused = refused || item.Scan.Refused
		rep.Updates = append(rep.Updates, item)
	}
	if refused || updateDryRun {
		return refused, nil
	}
	carryApprovals(current, next, false) // update moves some pins; the approvals of every item stay
	if err := lockfile.Save(fresh.ConfigDir, next); err != nil {
		return false, err //nolint:wrapcheck // already contextual
	}
	logger.Success("Wrote lock file", "path", lockfile.Path(fresh.ConfigDir))
	return false, nil
}

// updateItemFor describes one move from the current lock to next: the tags, the
// digests, the files that changed (against before, the hashes of the old tree)
// and the security scan of the new tree.
func updateItemFor(fresh *config.Config, current, next *lockfile.File, m *moveTo, before map[string]string) (updateItem, error) {
	entry := next.Find(m.row.Kind, m.row.Name)
	if entry == nil || entry.Tag == "" {
		return updateItem{}, oops.Errorf("%s %q was not resolved to a tag", m.row.Kind, m.row.Name)
	}
	item := updateItem{Kind: m.row.Kind, Name: m.row.Name, To: &tagresolve.TagRef{Tag: entry.Tag, Commit: entry.Commit}, DigestNew: entry.Digest,
		Held: m.row.Held, Released: entry.Released, ReleasedFrom: entry.ReleasedFrom}
	if old := current.Find(m.row.Kind, m.row.Name); old != nil {
		item.DigestOld = old.Digest
		if old.Tag != "" {
			item.From = &tagresolve.TagRef{Tag: old.Tag, Commit: old.Commit}
		}
	}
	if len(before) > 0 { // an uncached old tree has nothing to compare with
		item.Files = tagresolve.DiffFiles(before, hashTree(m.src.treeDir, entry.Commit))
	}
	item.Scan = scanNewTree(fresh, m, entry.Commit)
	return item, nil
}

// scanNewTree runs the security scan (AR001-AR009, the one skill installs and
// approvals use) over the tree a pin is about to point to. Error findings refuse
// the pin unless --accept-findings.
func scanNewTree(cfg *config.Config, m *moveTo, commit string) *scanSummary {
	return scanTreeDir(cfg, m.row.Name, m.src.treeDir(commit), updateAcceptFindings)
}

// scanTreeDir scans the cached tree in dir (see lockrun.ScanTree).
func scanTreeDir(cfg *config.Config, name, dir string, accept bool) *scanSummary {
	return lockrun.ScanTree(cfg, name, dir, accept)
}

func entryCommit(lock *lockfile.File, row tagresolve.Row) string {
	if e := lock.Find(row.Kind, row.Name); e != nil {
		return e.Commit
	}
	return ""
}

func sortedKeys(m map[string]*moveTo) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// hashTree hashes the cached tree of a source; an uncached tree has no files.
func hashTree(dirOf func(string) string, commit string) map[string]string {
	dir := dirOf(commit)
	if dir == "" {
		return map[string]string{}
	}
	hashes, err := includes.FileHashes(dir)
	if err != nil {
		return map[string]string{}
	}
	return hashes
}
