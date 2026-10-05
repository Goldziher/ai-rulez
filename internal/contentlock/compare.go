package contentlock

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// DiffSchemaVersion versions the JSON written by `lock --diff --format json`.
const DiffSchemaVersion = 1

// Change scopes.
const (
	ScopeSource = "source"
	ScopeOutput = "output"
	// ScopeRemote is a remote include, installed skill or skill source that no
	// longer matches its pin.
	ScopeRemote = "remote"
	// ScopeServed is a skill the skills server serves whose digest differs from
	// its pin (or that is not pinned).
	ScopeServed = "served"
	// ScopeLock is a problem with the lock itself (settings mismatch, edited pins).
	ScopeLock = "lock"
)

// Change kinds.
const (
	Added   = "added"
	Removed = "removed"
	Changed = "changed"
)

// Change is one difference between the lock and the working tree.
type Change struct {
	Scope  string `json:"scope"`
	Change string `json:"change"`
	Kind   string `json:"kind,omitempty"`
	ID     string `json:"id,omitempty"`
	Domain string `json:"domain,omitempty"`
	// Path is the item's source path (relative to the configuration directory)
	// or the output's path (relative to the project root).
	Path string `json:"path,omitempty"`
	Old  string `json:"old,omitempty"`
	New  string `json:"new,omitempty"`
	// Detail is a short explanation, for example "moved from a to b".
	Detail string `json:"detail,omitempty"`
}

// Diff is the result of comparing a lock with the working tree.
type Diff struct {
	SchemaVersion int `json:"schema_version"`
	// InSync is true when nothing differs.
	InSync bool `json:"in_sync"`
	// NoPins is true when the lock carries no content pins (a version 1 lock).
	NoPins      bool     `json:"no_content_pins,omitempty"`
	LockVersion int      `json:"lock_version"`
	HashVersion int      `json:"hash_version,omitempty"`
	LockTool    string   `json:"lock_ai_rulez_version,omitempty"`
	ToolVersion string   `json:"ai_rulez_version,omitempty"`
	Changes     []Change `json:"changes"`
	// Notes are findings that do not fail a check, such as a tool version change.
	Notes []string `json:"notes,omitempty"`
}

// Sources returns the source-scope changes.
func (d *Diff) Sources() []Change { return d.scoped(ScopeSource) }

// Outputs returns the output-scope changes.
func (d *Diff) Outputs() []Change { return d.scoped(ScopeOutput) }

func (d *Diff) scoped(scope string) []Change {
	var out []Change
	for i := range d.Changes {
		if d.Changes[i].Scope == scope {
			out = append(out, d.Changes[i])
		}
	}
	return out
}

// Build fills the content pins of f from snap and recomputes the tree digest.
// Remote entries already in f are kept.
func Build(f *lockfile.File, snap *Snapshot) {
	f.HashVersion = lockfile.HashVersion
	f.AIRulezVersion = snap.Options.ToolVersion
	f.Profile = snap.Options.Profile
	f.Scope = snap.Options.scope()
	f.OutputsPinned = snap.Options.IncludeOutputs
	f.Item = append([]lockfile.Item(nil), snap.Items...)
	f.Output = append([]lockfile.OutputPin(nil), snap.Outputs...)
	f.Tree = TreeOf(f)
}

// TreeOf computes the top-level digest of everything a lock pins.
func TreeOf(f *lockfile.File) string {
	var entries []Entry
	for _, i := range f.Item {
		entries = append(entries, Entry{Kind: "item/" + i.Kind, Key: i.Domain + "\x00" + i.ID, Digest: i.Digest})
	}
	for _, o := range f.Output {
		entries = append(entries, Entry{Kind: "output", Key: o.Path, Digest: o.Digest})
	}
	for _, e := range f.Include {
		entries = append(entries, Entry{Kind: "include", Key: e.Name, Digest: e.Commit + " " + e.Digest})
	}
	for _, e := range f.Skill {
		entries = append(entries, Entry{Kind: "installed-skill", Key: e.Name, Digest: e.Commit + " " + e.Digest})
	}
	for _, e := range f.Source {
		entries = append(entries, Entry{Kind: "skill-source", Key: e.Name, Digest: e.Commit + " " + e.Digest})
	}
	for _, e := range f.Served {
		entries = append(entries, Entry{Kind: "served-skill", Key: e.Name, Digest: e.Commit + " " + e.Digest})
	}
	return TopDigest(entries)
}

// Compare compares the lock with a fresh snapshot. A lock without content pins
// yields a Diff with NoPins set and no changes.
func Compare(lock *lockfile.File, snap *Snapshot) *Diff {
	d := &Diff{
		SchemaVersion: DiffSchemaVersion, Changes: []Change{},
		LockVersion: lock.Version, HashVersion: lock.HashVersion,
		LockTool: lock.AIRulezVersion, ToolVersion: snap.Options.ToolVersion,
	}
	if !lock.HasContentPins() {
		d.NoPins = true
		return d
	}
	d.compareHeader(lock, snap)
	d.compareItems(lock.Item, snap.Items)
	if !snap.Options.SourcesOnly {
		d.compareOutputs(lock.Output, snap.Outputs)
	}
	SortChanges(d.Changes)
	if lock.AIRulezVersion != "" && snap.Options.ToolVersion != "" && lock.AIRulezVersion != snap.Options.ToolVersion {
		d.Notes = append(d.Notes, fmt.Sprintf("the lock was written by ai-rulez %s, this is %s; output digests can differ between releases", lock.AIRulezVersion, snap.Options.ToolVersion))
	}
	d.InSync = len(d.Changes) == 0
	return d
}

// compareHeader checks what the lock says about itself: unpinnable sources, the
// tree digest and the settings it was written with.
func (d *Diff) compareHeader(lock *lockfile.File, snap *Snapshot) {
	for _, problem := range snap.Problems {
		d.Changes = append(d.Changes, Change{Scope: ScopeLock, Change: Changed, Detail: problem})
	}
	switch {
	case lock.Tree == "":
		d.Changes = append(d.Changes, Change{Scope: ScopeLock, Change: Changed, Detail: "the lock has content pins but no tree digest; run `ai-rulez lock` to rewrite it"})
	case lock.Tree != TreeOf(lock):
		d.Changes = append(d.Changes, Change{Scope: ScopeLock, Change: Changed, Detail: "the tree digest does not match the pins in the lock; it was edited by hand"})
	}
	if want := snap.Options.scope(); lock.Scope != "" && lock.Scope != want {
		d.Changes = append(d.Changes, Change{Scope: ScopeLock, Change: Changed, Detail: fmt.Sprintf("the lock was written with scope %q, [lock] scope is %q", lock.Scope, want)})
	}
	if !snap.Options.SourcesOnly && lock.OutputsPinned != snap.Options.IncludeOutputs {
		d.Changes = append(d.Changes, Change{Scope: ScopeLock, Change: Changed, Detail: fmt.Sprintf("the lock was written with outputs pinned = %t, [lock] include_outputs is %t", lock.OutputsPinned, snap.Options.IncludeOutputs)})
	}
	if lock.Profile != snap.Options.Profile && snap.Options.IncludeOutputs && !snap.Options.SourcesOnly {
		d.Changes = append(d.Changes, Change{Scope: ScopeLock, Change: Changed, Detail: fmt.Sprintf("the lock pins outputs of profile %q, this check rendered %q", lock.Profile, snap.Options.Profile)})
	}
}

func (d *Diff) compareItems(locked, now []lockfile.Item) {
	old := map[string]lockfile.Item{}
	for _, i := range locked {
		old[i.Key()] = i
	}
	seen := map[string]bool{}
	for _, cur := range now {
		key := cur.Key()
		seen[key] = true
		prev, ok := old[key]
		switch {
		case !ok:
			d.Changes = append(d.Changes, itemChange(Added, cur, lockfile.Item{}))
		case prev.Digest != cur.Digest:
			d.Changes = append(d.Changes, itemChange(Changed, cur, prev))
		case prev.Path != cur.Path:
			c := itemChange(Changed, cur, prev)
			c.Detail = "moved from " + prev.Path
			d.Changes = append(d.Changes, c)
		}
	}
	for _, prev := range locked {
		if !seen[prev.Key()] {
			d.Changes = append(d.Changes, itemChange(Removed, lockfile.Item{}, prev))
		}
	}
}

func itemChange(change string, cur, prev lockfile.Item) Change {
	ref := cur
	if change == Removed {
		ref = prev
	}
	c := Change{Scope: ScopeSource, Change: change, Kind: ref.Kind, ID: ref.ID, Domain: ref.Domain, Path: ref.Path, Old: prev.Digest, New: cur.Digest}
	if change == Changed {
		var parts []string
		if prev.Version != cur.Version {
			parts = append(parts, fmt.Sprintf("version %q -> %q", prev.Version, cur.Version))
		}
		if prev.Owner != cur.Owner {
			parts = append(parts, fmt.Sprintf("owner %q -> %q", prev.Owner, cur.Owner))
		}
		c.Detail = strings.Join(parts, ", ")
	}
	return c
}

func (d *Diff) compareOutputs(locked, now []lockfile.OutputPin) {
	old := map[string]string{}
	for _, o := range locked {
		old[o.Path] = o.Digest
	}
	seen := map[string]bool{}
	for _, cur := range now {
		seen[cur.Path] = true
		prev, ok := old[cur.Path]
		switch {
		case !ok:
			d.Changes = append(d.Changes, Change{Scope: ScopeOutput, Change: Added, Path: cur.Path, New: cur.Digest})
		case prev != cur.Digest:
			d.Changes = append(d.Changes, Change{Scope: ScopeOutput, Change: Changed, Path: cur.Path, Old: prev, New: cur.Digest})
		}
	}
	for _, prev := range locked {
		if !seen[prev.Path] {
			d.Changes = append(d.Changes, Change{Scope: ScopeOutput, Change: Removed, Path: prev.Path, Old: prev.Digest})
		}
	}
}

var scopeOrder = map[string]int{ScopeLock: 0, ScopeRemote: 1, ScopeServed: 2, ScopeSource: 3, ScopeOutput: 4}

// SortChanges orders changes by scope, kind, domain, id and path.
func SortChanges(cs []Change) {
	sort.SliceStable(cs, func(i, j int) bool {
		a, b := cs[i], cs[j]
		if a.Scope != b.Scope {
			return scopeOrder[a.Scope] < scopeOrder[b.Scope]
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Domain != b.Domain {
			return a.Domain < b.Domain
		}
		if a.ID != b.ID {
			return a.ID < b.ID
		}
		return a.Path < b.Path
	})
}

// Line renders one change as a single line naming the item and whether its
// source or its output changed.
func (c Change) Line() string {
	switch c.Scope {
	case ScopeLock:
		return "lock: " + c.Detail
	case ScopeRemote:
		return fmt.Sprintf("remote %s %s: %s", c.Kind, c.ID, c.Detail)
	case ScopeServed:
		return fmt.Sprintf("served %s: %s", c.ID, c.Detail)
	}
	what := "output " + c.Path
	if c.Scope == ScopeSource {
		what = describe(c.Kind, c.Domain, c.ID)
		if c.Path != "" {
			what += " (" + c.Path + ")"
		}
	}
	line := fmt.Sprintf("%-15s %s", c.Scope+" "+c.Change, what)
	if c.Detail != "" {
		line += ": " + c.Detail
	}
	return line
}

// WriteText writes the changes, one per line, then the notes.
func (d *Diff) WriteText(w io.Writer) error {
	for i := range d.Changes {
		if _, err := fmt.Fprintln(w, "  "+d.Changes[i].Line()); err != nil {
			return err //nolint:wrapcheck // writer error
		}
	}
	for _, n := range d.Notes {
		if _, err := fmt.Fprintln(w, "  note: "+n); err != nil {
			return err //nolint:wrapcheck // writer error
		}
	}
	return nil
}

// WriteJSON writes the diff as indented JSON with a trailing newline.
func (d *Diff) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(d) //nolint:wrapcheck // writer error
}
