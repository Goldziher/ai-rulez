package publish

import (
	"fmt"
	"sort"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

// Change kinds of a release-notes entry.
const (
	ChangeAdded   = "added"
	ChangeChanged = "changed"
	ChangeRemoved = "removed"
)

// NoteChange is one difference between the previous release's lock and this one.
// It carries names and short digests only, never content.
type NoteChange struct {
	Change string
	// Kind is the item kind ("skill", "rule", ...) or "include", "installed-skill", "skill-source".
	Kind   string
	Domain string
	ID     string
	Digest string
}

// DiffLocks compares the pins of two locks: authored items by kind, domain and
// id, and remote entries by name. The result is sorted by change, kind, domain
// and id, so equal inputs give equal notes.
func DiffLocks(prev, cur *lockfile.File) []NoteChange {
	var out []NoteChange
	old := map[string]lockfile.Item{}
	for _, it := range prev.Item {
		old[it.Key()] = it
	}
	seen := map[string]bool{}
	for _, it := range cur.Item {
		seen[it.Key()] = true
		switch p, ok := old[it.Key()]; {
		case !ok:
			out = append(out, NoteChange{ChangeAdded, it.Kind, it.Domain, it.ID, it.Digest})
		case p.Digest != it.Digest:
			out = append(out, NoteChange{ChangeChanged, it.Kind, it.Domain, it.ID, it.Digest})
		}
	}
	for _, it := range prev.Item {
		if !seen[it.Key()] {
			out = append(out, NoteChange{ChangeRemoved, it.Kind, it.Domain, it.ID, it.Digest})
		}
	}
	out = append(out, diffEntries("include", prev.Include, cur.Include)...)
	out = append(out, diffEntries("installed-skill", prev.Skill, cur.Skill)...)
	out = append(out, diffEntries("skill-source", prev.Source, cur.Source)...)
	order := map[string]int{ChangeAdded: 0, ChangeChanged: 1, ChangeRemoved: 2}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if order[a.Change] != order[b.Change] {
			return order[a.Change] < order[b.Change]
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Domain != b.Domain {
			return a.Domain < b.Domain
		}
		return a.ID < b.ID
	})
	return out
}

func diffEntries(kind string, prev, cur []lockfile.Entry) []NoteChange {
	var out []NoteChange
	old := map[string]lockfile.Entry{}
	for _, e := range prev {
		old[e.Name+"\x00"+e.View] = e
	}
	seen := map[string]bool{}
	for _, e := range cur {
		key := e.Name + "\x00" + e.View
		seen[key] = true
		switch p, ok := old[key]; {
		case !ok:
			out = append(out, NoteChange{ChangeAdded, kind, "", e.Name, e.Digest})
		case p.Digest != e.Digest || p.Commit != e.Commit:
			out = append(out, NoteChange{ChangeChanged, kind, "", e.Name, e.Digest})
		}
	}
	for _, e := range prev {
		if !seen[e.Name+"\x00"+e.View] {
			out = append(out, NoteChange{ChangeRemoved, kind, "", e.Name, e.Digest})
		}
	}
	return out
}

// shortDigest is the first 12 hex characters of a "sha256:<hex>" digest.
func shortDigest(d string) string {
	hex := strings.TrimPrefix(d, "sha256:")
	if len(hex) > 12 {
		hex = hex[:12]
	}
	return hex
}

// notesSection renders the changes since a previous release. since is the tag
// (or label) the previous lock came from.
func notesSection(since string, changes []NoteChange) string {
	var sb strings.Builder
	sb.WriteString("\n## Changes since " + since + "\n\n")
	if len(changes) == 0 {
		sb.WriteString("No authored content or remote pin changed.\n")
		return sb.String()
	}
	for _, heading := range []struct{ change, title string }{
		{ChangeAdded, "Added"}, {ChangeChanged, "Changed"}, {ChangeRemoved, "Removed"},
	} {
		var lines []string
		for _, c := range changes {
			if c.Change != heading.change {
				continue
			}
			line := fmt.Sprintf("- %s `%s`", c.Kind, c.ID)
			if c.Domain != "" {
				line += " (domain " + c.Domain + ")"
			}
			if c.Change != ChangeRemoved && c.Digest != "" {
				line += " " + shortDigest(c.Digest)
			}
			lines = append(lines, line)
		}
		if len(lines) == 0 {
			continue
		}
		sb.WriteString("### " + heading.title + "\n\n" + strings.Join(lines, "\n") + "\n\n")
	}
	return strings.TrimRight(sb.String(), "\n") + "\n"
}

// parseLock reads lock bytes for the notes diff.
func parseLock(data []byte, what string) (*lockfile.File, error) {
	f, err := lockfile.Parse(data)
	if err != nil {
		return nil, oops.With("lock", what).Wrapf(err, "parse the lock for the release notes")
	}
	return f, nil
}
