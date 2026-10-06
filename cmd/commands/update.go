package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/tagresolve"
	"github.com/samber/oops"
	"github.com/spf13/cobra"
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
)

// UpdateCmd moves the pins of sources that use a version constraint.
var UpdateCmd = &cobra.Command{
	Use:   "update [name...]",
	Short: "Move the pins of sources that use a version constraint to the newest allowed tag",
	Long: `Move the lock entries of remote includes, installed skills and skill sources
that ask for a version range (version = ^1.2) to the newest tag the range
allows, then record the tag, its commit and the tree digest in ai-rulez.lock.
Names limit the update to those sources; --kind limits it to include, skill or
source. Only the lock changes: config.toml is never edited.

Nothing else moves a range pin: "generate" never resolves a range and
"lock" keeps a pin that still satisfies its constraint. "update" is the one
deliberate step, and it prints what changes first:

  ai-rulez lock --outdated          which sources have newer tags
  ai-rulez update --dry-run         what update would change, writes nothing
  ai-rulez update shared            move one source
  ai-rulez update                   move every source that has an update

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
tool error); 2 a source was refused (AR732, AR730, AR731) and nothing was written.`,
	Run: func(_ *cobra.Command, args []string) {
		if code := runUpdate(args); code != 0 {
			os.Exit(code)
		}
	},
}

func init() {
	f := UpdateCmd.Flags()
	f.BoolVar(&updateDryRun, "dry-run", false, "Show what would change and write nothing")
	f.BoolVar(&updateAllowDowngrade, "allow-downgrade", false, "Allow a tag with lower precedence than the pinned one")
	f.BoolVar(&updateAcceptMoved, "accept-moved-tag", false, "Re-pin a tag that now points to another commit (AR732) after you reviewed it")
	f.StringVar(&updateKind, "kind", "", "Limit the update to include, skill or source")
	addFormatFlag(f, &updateFormat, "", formatText, formatText, formatJSON)
	f.BoolVar(&updateOffline, "offline", false, "Refuse to run: update reads the remote's tags")
	f.StringVarP(&configDir, "config-dir", "n", "", "Configuration directory name (default: .ai-rulez)")
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
}

// updateReport is the JSON document of `update --format json`.
type updateReport struct {
	SchemaVersion int                `json:"schema_version"`
	DryRun        bool               `json:"dry_run"`
	Updates       []updateItem       `json:"updates"`
	Blocked       []tagresolve.Row   `json:"blocked,omitempty"`
	Notes         []string           `json:"notes,omitempty"`
	Unchanged     []tagresolve.Row   `json:"unchanged,omitempty"`
	moves         map[string]*moveTo `json:"-"`
}

type moveTo struct {
	row tagresolve.Row
	src versionSrc
}

func runUpdate(names []string) int {
	if updateKind != "" && updateKind != lockfile.KindInclude && updateKind != lockfile.KindSkill && updateKind != lockfile.KindSource {
		fmtError(oops.Errorf("unknown --kind %q (use include, skill or source)", updateKind))
		return 1
	}
	if err := checkFormatFlag(updateFormat); err != nil {
		fmtError(err)
		return 1
	}
	lockOffline = updateOffline
	if err := requireOnline("update"); err != nil {
		fmtError(err)
		return 1
	}
	return updateAt("", updateKind, names)
}

func moveKey(kind, name string) string { return kind + "\x00" + name }

func updateAt(path, kind string, names []string) int {
	ctx := context.Background()
	cfg, err := loadForLock(path, config.WithoutLocal(), config.WithoutRemote())
	if err != nil {
		fmtError(err)
		return 1
	}
	current, err := lockfile.Load(cfg.ConfigDir)
	if err != nil {
		fmtError(err)
		return 1
	}
	wanted := map[string]bool{}
	for _, n := range names {
		wanted[n] = true
	}
	srcs := versionSources(cfg, kind, wanted)
	if err := checkNamesMatched(srcs, wanted); err != nil {
		fmtError(err)
		return 1
	}
	rows, err := evaluateSources(ctx, srcs, current)
	if err != nil {
		fmtError(err)
		return 1
	}
	report := &updateReport{SchemaVersion: UpdateSchemaVersion, DryRun: updateDryRun, Updates: []updateItem{}, moves: map[string]*moveTo{}}
	planUpdates(report, srcs, rows)
	if len(report.Blocked) > 0 {
		return finishUpdate(report, exitDrift)
	}
	if len(report.moves) == 0 {
		return finishUpdate(report, 0)
	}
	if err := applyUpdates(path, cfg, current, srcs, report); err != nil {
		fmtError(err)
		return 1
	}
	return finishUpdate(report, 0)
}

func checkNamesMatched(srcs []versionSrc, wanted map[string]bool) error {
	found := map[string]bool{}
	for _, s := range srcs {
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
	for i, row := range rows {
		src := srcs[i]
		// A moved tag is a finding before it is a downgrade: it must never land in Unchanged.
		if row.Downgrade && !updateAllowDowngrade && row.Status != tagresolve.StatusTagMoved {
			rep.Unchanged = append(rep.Unchanged, row)
			continue
		}
		switch row.Status {
		case tagresolve.StatusUpdatable, tagresolve.StatusNotLocked, tagresolve.StatusTagMissing:
			rep.moves[moveKey(row.Kind, row.Name)] = &moveTo{row: row, src: src}
		case tagresolve.StatusTagMoved:
			switch {
			case !updateAcceptMoved:
				rep.Blocked = append(rep.Blocked, row)
			case row.Downgrade && !updateAllowDowngrade:
				rep.Unchanged = append(rep.Unchanged, row)
			default:
				rep.moves[moveKey(row.Kind, row.Name)] = &moveTo{row: row, src: src}
				rep.Notes = append(rep.Notes, fmt.Sprintf("%s %s: accepted the moved tag %s", row.Kind, row.Name, row.Locked.Tag))
			}
		case tagresolve.StatusLockedNonVersion:
			// Moving off a pin that is not a version has no ordering to protect: ask for the same consent as a downgrade.
			if updateAllowDowngrade {
				rep.moves[moveKey(row.Kind, row.Name)] = &moveTo{row: row, src: src}
			} else {
				rep.Unchanged = append(rep.Unchanged, row)
			}
		case tagresolve.StatusUnsatisfied, tagresolve.StatusInvalid:
			rep.Blocked = append(rep.Blocked, row)
		case tagresolve.StatusLowerOnly:
			if updateAllowDowngrade {
				rep.moves[moveKey(row.Kind, row.Name)] = &moveTo{row: row, src: src}
			} else {
				rep.Unchanged = append(rep.Unchanged, row)
			}
		default:
			rep.Unchanged = append(rep.Unchanged, row)
		}
	}
}

// applyUpdates re-resolves the moving sources the way `lock <names>` does,
// advancing them to their allowed tag, and writes the lock unless --dry-run.
func applyUpdates(path string, cfg *config.Config, current *lockfile.File, srcs []versionSrc, rep *updateReport) error {
	// Make sure the cache holds the pinned trees (a dry run, or another project,
	// may have left a newer one), so the before/after comparison is accurate.
	if _, err := loadForLock(path, config.WithoutLocal()); err != nil {
		return err
	}
	before := map[string]map[string]string{}
	wanted := map[string]bool{}
	for key, m := range rep.moves {
		wanted[m.row.Name] = true
		before[key] = hashTree(m.src.treeDir, entryCommit(current, m.row))
	}
	includes.Advance = func(kind, name string) bool { return rep.moves[moveKey(kind, name)] != nil }
	includes.AllowDowngrade, includes.AcceptMovedTag = updateAllowDowngrade, updateAcceptMoved
	defer func() { includes.Advance, includes.AllowDowngrade, includes.AcceptMovedTag = nil, false, false }()
	defer prepareLockRun(true, "", wanted)()

	fresh, err := loadForLock(path, config.WithoutLocal())
	if err != nil {
		return err
	}
	if err := fresh.Validate(); err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	next, err := nextLock(fresh, current, "", wanted, true)
	if err != nil {
		return err
	}
	if err := pinContent(fresh, current, next, "", wanted); err != nil {
		return err
	}
	for _, key := range sortedKeys(rep.moves) {
		m := rep.moves[key]
		entry := next.Find(m.row.Kind, m.row.Name)
		if entry == nil || entry.Tag == "" {
			return oops.Errorf("%s %q was not resolved to a tag", m.row.Kind, m.row.Name)
		}
		item := updateItem{Kind: m.row.Kind, Name: m.row.Name, To: &tagresolve.TagRef{Tag: entry.Tag, Commit: entry.Commit}, DigestNew: entry.Digest}
		if old := current.Find(m.row.Kind, m.row.Name); old != nil {
			item.DigestOld = old.Digest
			if old.Tag != "" {
				item.From = &tagresolve.TagRef{Tag: old.Tag, Commit: old.Commit}
			}
		}
		if len(before[key]) > 0 { // an uncached old tree has nothing to compare with
			item.Files = tagresolve.DiffFiles(before[key], hashTree(m.src.treeDir, entry.Commit))
		}
		rep.Updates = append(rep.Updates, item)
	}
	if updateDryRun {
		return nil
	}
	if err := lockfile.Save(fresh.ConfigDir, next); err != nil {
		return err //nolint:wrapcheck // already contextual
	}
	logger.Success("Wrote lock file", "path", lockfile.Path(fresh.ConfigDir))
	return nil
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

func finishUpdate(rep *updateReport, code int) int {
	sort.Slice(rep.Updates, func(i, j int) bool {
		if rep.Updates[i].Kind != rep.Updates[j].Kind {
			return rep.Updates[i].Kind < rep.Updates[j].Kind
		}
		return rep.Updates[i].Name < rep.Updates[j].Name
	})
	if updateFormat == formatJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			fmtError(oops.Wrapf(err, "write the report"))
			return 1
		}
		return code
	}
	writeUpdateText(rep)
	return code
}

func writeUpdateText(rep *updateReport) {
	verb := "updated"
	if rep.DryRun {
		verb = "would update"
	}
	for _, u := range rep.Updates {
		from := "(unlocked)"
		if u.From != nil {
			from = u.From.Tag
		}
		fmt.Printf("%s %s: %s -> %s (%s)\n", u.Kind, u.Name, from, u.To.Tag, shortSHA(u.To.Commit))
		for _, f := range u.Files {
			fmt.Printf("  %s  %s\n", f.Change, f.Path)
		}
		if u.DigestOld != u.DigestNew {
			fmt.Printf("  tree %s -> %s\n", u.DigestOld, u.DigestNew)
		}
		fmt.Println("  run `ai-rulez generate`, then `ai-rulez lock` (it refreshes the output pins and the served-skill pins, which stay stale until then)")
	}
	for _, r := range rep.Blocked {
		fmt.Fprintf(os.Stderr, "refused %s %s: %s %s\n", r.Kind, r.Name, r.Code, r.Note)
	}
	for _, r := range rep.Unchanged {
		if r.Downgrade {
			fmt.Printf("%s %s: %s\n", r.Kind, r.Name, r.Note)
		}
	}
	for _, n := range rep.Notes {
		fmt.Println(n)
	}
	switch {
	case len(rep.Blocked) > 0:
		fmt.Fprintln(os.Stderr, "nothing was written")
	case len(rep.Updates) == 0:
		fmt.Println("everything is up to date within its constraints")
	default:
		fmt.Printf("%s %d source(s)\n", verb, len(rep.Updates))
	}
}
