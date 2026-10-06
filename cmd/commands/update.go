package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
	"github.com/Goldziher/ai-rulez/v5/internal/semver"
	"github.com/Goldziher/ai-rulez/v5/internal/tagresolve"
	"github.com/Goldziher/ai-rulez/v5/internal/versionpatch"
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
	f.BoolVar(&updateMajor, "major", false, "Handle only sources that have a newer major version: print the constraint that would take it")
	f.BoolVar(&updateWriteConfig, "write-config", false, "With --major, rewrite the version line of those sources in config.toml and move their pins")
	f.BoolVar(&updateAcceptFindings, "accept-findings", false, "Write a pin although the security scan of the new tree has error findings (review them first)")
	f.BoolVar(&updateAcceptMoved, "accept-moved-tag", false, "Re-pin a tag that now points to another commit (AR732) after you reviewed it")
	f.StringVar(&updateKind, "kind", "", "Limit the update to include, skill or source")
	addFormatFlag(f, &updateFormat, "", formatText, formatText, formatJSON)
	addJSONFlagAlias(f)
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
	// Held lists the newer tags min_release_age held back (AR733).
	Held []tagresolve.Held `json:"held_back,omitempty"`
	// Released and ReleasedFrom are the recorded release time of the new tag.
	Released     string `json:"released,omitempty"`
	ReleasedFrom string `json:"released_from,omitempty"`
	// Scan is the security scan of the new tree.
	Scan *scanSummary `json:"scan,omitempty"`
}

// scanSummary is the security scan (AR001-AR009) of a tree about to be pinned.
type scanSummary struct {
	Errors   int           `json:"errors"`
	Warnings int           `json:"warnings"`
	Findings []scanFinding `json:"findings,omitempty"`
	// Refused is true when the error findings block the pin.
	Refused bool `json:"refused,omitempty"`
	// Accepted is true when --accept-findings let the pin through.
	Accepted bool `json:"accepted,omitempty"`
	// Note says what could not be scanned (a tree over the limits).
	Note string `json:"note,omitempty"`
}

type scanFinding struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	File     string `json:"file"`
	Line     int    `json:"line,omitempty"`
	Message  string `json:"message"`
}

// majorItem is one source with a newer major version than its constraint allows.
type majorItem struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	From   string `json:"from"`
	To     string `json:"to"`
	Latest string `json:"latest"`
	// Written is true when --write-config rewrote the version line.
	Written bool `json:"written,omitempty"`
}

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

func runUpdate(names []string) int {
	if updateKind != "" && updateKind != lockfile.KindInclude && updateKind != lockfile.KindSkill && updateKind != lockfile.KindSource {
		fmtError(oops.Errorf("unknown --kind %q (use include, skill or source)", updateKind))
		return 1
	}
	if updateWriteConfig && !updateMajor {
		fmtError(oops.Hint("config.toml is edited only to take a new major version: `ai-rulez update --major --write-config`").
			Errorf("--write-config needs --major"))
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
	defer installReleaseGateFor(cfg)()
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
	if updateMajor {
		return majorUpdate(path, cfg, current, srcs, rows, report)
	}
	return finishUpdate(report, planAndApply(path, cfg, current, srcs, rows, report))
}

// planAndApply sorts the evaluated sources into moves, refusals and no-ops, then
// writes the moves unless something is refused or this is a dry run. It returns
// the exit code and leaves printing the report to the caller.
func planAndApply(path string, cfg *config.Config, current *lockfile.File, srcs []versionSrc, rows []tagresolve.Row, report *updateReport) int {
	planUpdates(report, srcs, rows)
	if len(report.Blocked) > 0 {
		return exitDrift
	}
	if len(report.moves) == 0 {
		return 0
	}
	refused, err := applyUpdates(path, cfg, current, srcs, report)
	if err != nil {
		fmtError(err)
		return 1
	}
	if refused {
		return exitDrift
	}
	return 0
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
	includes.Advance = func(kind, name string) bool { return rep.moves[moveKey(kind, name)] != nil }
	includes.AllowDowngrade, includes.AcceptMovedTag = updateAllowDowngrade, updateAcceptMoved
	defer func() { includes.Advance, includes.AllowDowngrade, includes.AcceptMovedTag = nil, false, false }()
	defer prepareLockRun(true, "", wanted)()

	fresh, err := loadForLock(path, config.WithoutLocal())
	if err != nil {
		return false, err
	}
	if err := fresh.Validate(); err != nil {
		return false, err //nolint:wrapcheck // already contextual
	}
	next, err := nextLock(fresh, current, "", wanted, true)
	if err != nil {
		return false, err
	}
	if err := pinContent(fresh, current, next, "", wanted); err != nil {
		return false, err
	}
	for _, key := range sortedKeys(rep.moves) {
		m := rep.moves[key]
		entry := next.Find(m.row.Kind, m.row.Name)
		if entry == nil || entry.Tag == "" {
			return false, oops.Errorf("%s %q was not resolved to a tag", m.row.Kind, m.row.Name)
		}
		item := updateItem{Kind: m.row.Kind, Name: m.row.Name, To: &tagresolve.TagRef{Tag: entry.Tag, Commit: entry.Commit}, DigestNew: entry.Digest,
			Held: m.row.Held, Released: entry.Released, ReleasedFrom: entry.ReleasedFrom}
		if old := current.Find(m.row.Kind, m.row.Name); old != nil {
			item.DigestOld = old.Digest
			if old.Tag != "" {
				item.From = &tagresolve.TagRef{Tag: old.Tag, Commit: old.Commit}
			}
		}
		if len(before[key]) > 0 { // an uncached old tree has nothing to compare with
			item.Files = tagresolve.DiffFiles(before[key], hashTree(m.src.treeDir, entry.Commit))
		}
		item.Scan = scanNewTree(fresh, m, entry.Commit)
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

// maxScanFindingsListed bounds the findings printed per source; the counts are exact.
const maxScanFindingsListed = 20

// scanNewTree runs the security scan (AR001-AR009, the one skill installs and
// approvals use) over the tree a pin is about to point to. Error findings refuse
// the pin unless --accept-findings.
func scanNewTree(cfg *config.Config, m *moveTo, commit string) *scanSummary {
	sum := &scanSummary{}
	dir := m.src.treeDir(commit)
	if dir == "" {
		sum.Note = "the new tree is not in the local cache: it was not scanned"
		return sum
	}
	files, note := walkFiles(dir)
	sum.Note = note
	for _, f := range scanApproved(cfg, m.row.Name, files) {
		switch f.Severity {
		case lint.SeverityError:
			sum.Errors++
		case lint.SeverityWarning:
			sum.Warnings++
		}
		if len(sum.Findings) < maxScanFindingsListed && (f.Severity == lint.SeverityError || f.Severity == lint.SeverityWarning) {
			sum.Findings = append(sum.Findings, scanFinding{Code: f.Code, Severity: string(f.Severity), File: f.File, Line: f.Line, Message: f.Message})
		}
	}
	if sum.Errors > 0 {
		sum.Refused, sum.Accepted = !updateAcceptFindings, updateAcceptFindings
	}
	return sum
}

// majorUpdate is `update --major`: it lists the sources that have a newer major
// version than their constraint allows and, with --write-config, takes it.
func majorUpdate(path string, cfg *config.Config, current *lockfile.File, srcs []versionSrc, rows []tagresolve.Row, report *updateReport) int {
	for i, row := range rows {
		if !row.MajorAvailable || row.Latest == nil || blockedStatus(row.Status) {
			continue
		}
		if to, ok := majorConstraint(row.Latest.Tag, srcs[i].want.TagPrefix); ok {
			report.Major = append(report.Major, majorItem{Kind: row.Kind, Name: row.Name, From: row.Constraint, To: to, Latest: row.Latest.Tag})
		}
	}
	sort.Slice(report.Major, func(i, j int) bool {
		if report.Major[i].Kind != report.Major[j].Kind {
			return report.Major[i].Kind < report.Major[j].Kind
		}
		return report.Major[i].Name < report.Major[j].Name
	})
	if len(report.Major) == 0 || !updateWriteConfig || updateDryRun {
		return finishUpdate(report, 0)
	}
	original, err := patchMajor(cfg, report.Major)
	if err != nil {
		fmtError(err)
		return 1
	}
	rollback := func() {
		if rerr := safefs.WriteFileAtomic(filepath.Join(cfg.ConfigDir, configFileTOML), original); rerr != nil {
			fmtError(oops.Wrapf(rerr, "restore config.toml; it still holds the new version constraints"))
		}
		for i := range report.Major {
			report.Major[i].Written = false
		}
	}
	// The pins follow the new constraints: evaluate again against the patched config.
	fresh, err := loadForLock(path, config.WithoutLocal(), config.WithoutRemote())
	if err != nil {
		rollback()
		fmtError(err)
		return 1
	}
	names := map[string]bool{}
	for _, m := range report.Major {
		names[m.Name] = true
	}
	srcs2 := versionSources(fresh, updateKind, names)
	rows2, err := evaluateSources(context.Background(), srcs2, current)
	if err != nil {
		rollback()
		fmtError(err)
		return 1
	}
	code := planAndApply(path, fresh, current, srcs2, rows2, report)
	if code != 0 {
		rollback() // before the report is printed, so it never claims a write that was undone
	}
	return finishUpdate(report, code)
}

// blockedStatus reports the statuses where a constraint cannot be trusted enough to rewrite it.
func blockedStatus(status string) bool {
	switch status {
	case tagresolve.StatusTagMoved, tagresolve.StatusUnsatisfied, tagresolve.StatusInvalid:
		return true
	}
	return false
}

// majorConstraint is the constraint that takes the major version of tag: "^2.0" for v2.3.1.
func majorConstraint(tag, prefix string) (string, bool) {
	v, ok := semver.ParseTag(tag, prefix)
	if !ok {
		return "", false
	}
	return fmt.Sprintf("^%d.0", v.Major), true
}

// patchMajor rewrites the version line of every item in config.toml (only that
// value changes) and returns the original bytes for a rollback.
func patchMajor(cfg *config.Config, items []majorItem) (original []byte, err error) {
	file := filepath.Join(cfg.ConfigDir, configFileTOML)
	original, err = safefs.ReadRegular(file)
	if err != nil {
		return nil, oops.With("path", file).Wrapf(err, "read config.toml")
	}
	patched := original
	tables := map[string]string{lockfile.KindInclude: "includes", lockfile.KindSkill: "installed_skills", lockfile.KindSource: "skill_sources"}
	for _, it := range items {
		patched, err = versionpatch.SetConstraint(patched, tables[it.Kind], it.Name, it.To)
		if err != nil {
			return nil, oops.With("source", it.Name).Wrapf(err, "cannot update the constraint of %s %q", it.Kind, it.Name)
		}
	}
	if err := safefs.WriteFileAtomic(file, patched); err != nil {
		return nil, oops.With("path", file).Wrapf(err, "write config.toml")
	}
	for i := range items {
		items[i].Written = true
	}
	return original, nil
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
	refused := 0
	for _, u := range rep.Updates {
		from := "(unlocked)"
		if u.From != nil {
			from = u.From.Tag
		}
		fmt.Printf("%s %s: %s -> %s (%s)\n", u.Kind, u.Name, from, u.To.Tag, shortSHA(u.To.Commit))
		if u.Released != "" {
			fmt.Printf("  released %s (%s)\n", u.Released, u.ReleasedFrom)
		}
		for _, h := range u.Held {
			fmt.Printf("  %s\n", safeText(h.String()))
		}
		for _, f := range u.Files {
			fmt.Printf("  %s  %s\n", f.Change, f.Path)
		}
		if u.DigestOld != u.DigestNew {
			fmt.Printf("  tree %s -> %s\n", u.DigestOld, u.DigestNew)
		}
		if u.Scan != nil {
			writeScanText(u)
			if u.Scan.Refused {
				refused++
			}
		}
		fmt.Println("  run `ai-rulez generate`, then `ai-rulez lock` (it refreshes the output pins and the served-skill pins, which stay stale until then)")
	}
	for _, m := range rep.Major {
		switch {
		case m.Written:
			fmt.Printf("%s %s: wrote version = %q to config.toml (was %q; latest %s)\n", m.Kind, m.Name, m.To, m.From, m.Latest)
		default:
			fmt.Printf("%s %s: newer major %s: version = %q (now %q); `update --major --write-config` applies it\n", m.Kind, m.Name, m.Latest, m.To, m.From)
		}
	}
	for _, r := range rep.Blocked {
		fmt.Fprintf(os.Stderr, "refused %s %s: %s %s\n", r.Kind, r.Name, r.Code, r.Note)
	}
	for _, r := range rep.Unchanged {
		if r.Downgrade {
			fmt.Printf("%s %s: %s\n", r.Kind, r.Name, r.Note)
		}
		for _, h := range r.Held {
			fmt.Printf("%s %s: %s\n", r.Kind, r.Name, safeText(h.String()))
		}
	}
	for _, n := range rep.Notes {
		fmt.Println(n)
	}
	switch {
	case len(rep.Blocked) > 0:
		fmt.Fprintln(os.Stderr, "nothing was written")
	case refused > 0:
		fmt.Fprintf(os.Stderr, "refused %d source(s): the security scan of the new tree has error findings; review them, then pass --accept-findings. Nothing was written\n", refused)
	case len(rep.Updates) > 0:
		fmt.Printf("%s %d source(s)\n", verb, len(rep.Updates))
	case len(rep.Major) == 0:
		fmt.Println("everything is up to date within its constraints")
	}
}

// writeScanText prints the scan result of one update; the text of a finding is
// untrusted content, so it is printed escaped.
func writeScanText(u updateItem) {
	sc := u.Scan
	switch {
	case sc.Errors == 0 && sc.Warnings == 0:
		fmt.Println("  scan: 0 findings")
	default:
		fmt.Printf("  scan: %d error(s), %d warning(s)\n", sc.Errors, sc.Warnings)
	}
	for _, f := range sc.Findings {
		fmt.Printf("    %s %s %s:%d %s\n", f.Code, f.Severity, safeText(f.File), f.Line, safeText(f.Message))
	}
	if sc.Note != "" {
		fmt.Printf("  scan note: %s\n", safeText(sc.Note))
	}
	if sc.Accepted {
		fmt.Println("  scan findings accepted with --accept-findings")
	}
}
