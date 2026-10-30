package pinupdate

import (
	"context"
	"fmt"
	"io"
	"sort"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/includes"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
	"github.com/Goldziher/ai-rulez/v5/internal/lockrun"
	"github.com/Goldziher/ai-rulez/v5/internal/tagresolve"
)

// SchemaVersion versions the JSON of `update --format json`.
const SchemaVersion = 1

// Options are the consents and modes of one update.
type Options struct {
	// DryRun shows what would change and writes nothing.
	DryRun bool
	// AllowDowngrade allows a tag with lower precedence than the pinned one.
	AllowDowngrade bool
	// AcceptMoved re-pins a tag that now points to another commit (AR732).
	AcceptMoved bool
	// AcceptFindings writes a pin although the security scan of the new tree has error findings.
	AcceptFindings bool
}

// ScanSummary is the security scan (AR001-AR009) of a tree about to be pinned.
type ScanSummary = lockrun.ScanSummary

// Item is one pin that moves.
type Item struct {
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
	Scan *ScanSummary `json:"scan,omitempty"`
}

// MajorItem is one source with a newer major version than its constraint allows.
type MajorItem struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	From   string `json:"from"`
	To     string `json:"to"`
	Latest string `json:"latest"`
	// Written is true when --write-config rewrote the version line.
	Written bool `json:"written,omitempty"`
}

// Report is the JSON document of `update --format json`.
type Report struct {
	SchemaVersion int                `json:"schema_version"`
	DryRun        bool               `json:"dry_run"`
	Updates       []Item             `json:"updates"`
	Blocked       []tagresolve.Row   `json:"blocked,omitempty"`
	Notes         []string           `json:"notes,omitempty"`
	Unchanged     []tagresolve.Row   `json:"unchanged,omitempty"`
	Major         []MajorItem        `json:"major,omitempty"`
	Moves         map[string]*MoveTo `json:"-"`
	// LockPath is the lock file the run wrote; empty when nothing was written.
	LockPath string `json:"-"`
}

// MoveTo is a source whose pin moves.
type MoveTo struct {
	Row tagresolve.Row
	Src Source
}

// NewReport starts the report of a run.
func NewReport(dryRun bool) *Report {
	return &Report{SchemaVersion: SchemaVersion, DryRun: dryRun, Updates: []Item{}, Moves: map[string]*MoveTo{}}
}

// Refused reports whether the report holds a refusal: a blocked source, or a
// pin whose pre-pin scan has error findings that were not accepted.
func (r *Report) Refused() bool {
	if len(r.Blocked) > 0 {
		return true
	}
	for i := range r.Updates {
		if s := r.Updates[i].Scan; s != nil && s.Refused {
			return true
		}
	}
	return false
}

// Sort orders the updates by kind, then name.
func (r *Report) Sort() {
	sort.Slice(r.Updates, func(i, j int) bool {
		if r.Updates[i].Kind != r.Updates[j].Kind {
			return r.Updates[i].Kind < r.Updates[j].Kind
		}
		return r.Updates[i].Name < r.Updates[j].Name
	})
}

// Key identifies a source by kind and name: the name alone also matched a
// same-named include, skill or source of another kind.
func Key(kind, name string) string { return kind + "\x00" + name }

// RefreshFilter limits the remote refresh of an update to the sources that
// move, matched by kind and name.
func RefreshFilter(moves map[string]*MoveTo) func(kind, name string) bool {
	return func(kind, name string) bool { return moves[Key(kind, name)] != nil }
}

// Plan sorts the evaluated sources into moves, refusals and no-ops.
func Plan(rep *Report, srcs []Source, rows []tagresolve.Row, opt Options) {
	for i := range rows {
		row := &rows[i]
		src := srcs[i]
		// A moved tag is a finding before it is a downgrade: it must never land in Unchanged.
		if row.Downgrade && !opt.AllowDowngrade && row.Status != tagresolve.StatusTagMoved {
			rep.Unchanged = append(rep.Unchanged, *row)
			continue
		}
		switch row.Status {
		case tagresolve.StatusUpdatable, tagresolve.StatusNotLocked, tagresolve.StatusTagMissing:
			rep.Moves[Key(row.Kind, row.Name)] = &MoveTo{Row: *row, Src: src}
		case tagresolve.StatusTagMoved:
			switch {
			case !opt.AcceptMoved:
				rep.Blocked = append(rep.Blocked, *row)
			case row.Downgrade && !opt.AllowDowngrade:
				rep.Unchanged = append(rep.Unchanged, *row)
			default:
				rep.Moves[Key(row.Kind, row.Name)] = &MoveTo{Row: *row, Src: src}
				rep.Notes = append(rep.Notes, fmt.Sprintf("%s %s: accepted the moved tag %s", row.Kind, row.Name, row.Locked.Tag))
			}
		case tagresolve.StatusLockedNonVersion, tagresolve.StatusLowerOnly:
			// Moving off a pin that is not a version has no ordering to protect: ask for the same consent as a downgrade.
			if opt.AllowDowngrade {
				rep.Moves[Key(row.Kind, row.Name)] = &MoveTo{Row: *row, Src: src}
			} else {
				rep.Unchanged = append(rep.Unchanged, *row)
			}
		case tagresolve.StatusUnsatisfied, tagresolve.StatusInvalid:
			rep.Blocked = append(rep.Blocked, *row)
		default:
			rep.Unchanged = append(rep.Unchanged, *row)
		}
	}
}

// Run plans the update of p into rep and writes the moves unless something is
// refused or this is a dry run. It returns nil, ErrRefused for a refusal (the
// report says which sources), or the failure.
func (p *Prepared) Run(ctx context.Context, env Env, opt Options, rep *Report) error {
	Plan(rep, p.Srcs, p.Rows, opt)
	if len(rep.Blocked) > 0 {
		return ErrRefused
	}
	if len(rep.Moves) == 0 {
		return nil
	}
	refused, err := p.apply(ctx, env, opt, rep)
	if err != nil {
		return err
	}
	if refused {
		return ErrRefused
	}
	return nil
}

// Update prepares and runs an update of the sources of kind and names. The
// report is returned with ErrRefused too, so a caller can show the refusals.
func Update(ctx context.Context, env Env, opt Options, kind string, names []string) (*Report, error) {
	if err := RequireOnline("update", env.Policy.Offline); err != nil {
		return nil, err
	}
	p, err := Prepare(ctx, env, kind, names)
	if err != nil {
		return nil, err
	}
	rep := NewReport(opt.DryRun)
	err = p.Run(ctx, env, opt, rep)
	rep.Sort()
	return rep, err
}

// apply re-resolves the moving sources the way `lock <names>` does, advancing
// them to their allowed tag, and writes the lock unless this is a dry run.
//
// It returns refused = true (and writes nothing) when the security scan of a new
// tree has error findings and AcceptFindings was not given.
func (p *Prepared) apply(ctx context.Context, env Env, opt Options, rep *Report) (refused bool, err error) {
	// The policy of the run's loads: the release age gate, and the consents that
	// let a source move off its pin.
	policy := env.Policy
	policy.ReleaseGate = p.Gate
	policy.Advance = func(kind, name string) bool { return rep.Moves[Key(kind, name)] != nil }
	policy.AllowDowngrade, policy.AcceptMovedTag = opt.AllowDowngrade, opt.AcceptMoved

	// Make sure the cache holds the pinned trees (a dry run, or another project,
	// may have left a newer one), so the before/after comparison is accurate.
	if _, err := env.Load(ctx, policy, config.WithoutLocal()); err != nil {
		return false, err
	}
	before := map[string]map[string]string{}
	wanted := map[string]bool{}
	for key, m := range rep.Moves {
		wanted[m.Row.Name] = true
		before[key] = hashTree(m.Src.TreeDir, entryCommit(p.Current, m.Row))
	}
	run := lockrun.RunPolicy(policy, true, "", wanted)
	run.Refresh = RefreshFilter(rep.Moves) // by kind and name: a same-named source of another kind stays put

	fresh, err := env.Load(ctx, run, config.WithoutLocal())
	if err != nil {
		return false, err
	}
	if err := fresh.Validate(); err != nil {
		return false, err //nolint:wrapcheck // already contextual
	}
	req := lockrun.Request{Names: wantedNames(wanted), RefreshServed: true}
	lockEnv := lockrun.Env{Version: env.Version, Policy: run, Load: env.Load, Collector: env.Collector, Report: io.Discard,
		NewForge: env.NewForge, GitToken: env.GitToken, Cwd: env.Cwd}
	next, _, err := lockrun.Next(ctx, fresh, p.Current, &req, &lockEnv, true)
	if err != nil {
		return false, err //nolint:wrapcheck // already contextual
	}
	if err := lockrun.PinContent(fresh, p.Current, next, &req, env.Version); err != nil { //nolint:contextcheck // the generator renders without a context (as in lockrun.Write)
		return false, err //nolint:wrapcheck // already contextual
	}
	for _, key := range sortedKeys(rep.Moves) {
		item, err := itemFor(fresh, p.Current, next, rep.Moves[key], before[key], opt.AcceptFindings)
		if err != nil {
			return false, err
		}
		refused = refused || item.Scan.Refused
		rep.Updates = append(rep.Updates, item)
	}
	if refused || opt.DryRun {
		return refused, nil
	}
	lockrun.CarryApprovals(fresh.Log(), p.Current, next, false) // update moves some pins; the approvals of every item stay
	if err := lockfile.Save(fresh.ConfigDir, next); err != nil {
		return false, err //nolint:wrapcheck // already contextual
	}
	rep.LockPath = lockfile.Path(fresh.ConfigDir)
	return false, nil
}

// itemFor describes one move from the current lock to next: the tags, the
// digests, the files that changed (against before, the hashes of the old tree)
// and the security scan (AR001-AR009, the one skill installs and approvals use)
// of the new tree, whose error findings refuse the pin unless accept.
func itemFor(fresh *config.Config, current, next *lockfile.File, m *MoveTo, before map[string]string, accept bool) (Item, error) {
	entry := next.Find(m.Row.Kind, m.Row.Name)
	if entry == nil || entry.Tag == "" {
		return Item{}, oops.Errorf("%s %q was not resolved to a tag", m.Row.Kind, m.Row.Name)
	}
	item := Item{Kind: m.Row.Kind, Name: m.Row.Name, To: &tagresolve.TagRef{Tag: entry.Tag, Commit: entry.Commit}, DigestNew: entry.Digest,
		Held: m.Row.Held, Released: entry.Released, ReleasedFrom: entry.ReleasedFrom}
	if old := current.Find(m.Row.Kind, m.Row.Name); old != nil {
		item.DigestOld = old.Digest
		if old.Tag != "" {
			item.From = &tagresolve.TagRef{Tag: old.Tag, Commit: old.Commit}
		}
	}
	if len(before) > 0 { // an uncached old tree has nothing to compare with
		item.Files = tagresolve.DiffFiles(before, hashTree(m.Src.TreeDir, entry.Commit))
	}
	item.Scan = lockrun.ScanTree(fresh, m.Row.Name, m.Src.TreeDir(entry.Commit), accept)
	return item, nil
}

func entryCommit(lock *lockfile.File, row tagresolve.Row) string {
	if e := lock.Find(row.Kind, row.Name); e != nil {
		return e.Commit
	}
	return ""
}

func sortedKeys(m map[string]*MoveTo) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func wantedNames(wanted map[string]bool) []string {
	names := make([]string, 0, len(wanted))
	for n := range wanted {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
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
