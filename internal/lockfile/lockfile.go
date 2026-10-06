// Package lockfile reads, writes and verifies ai-rulez.lock, the committed
// record of what every remote include and installed skill resolved to: the
// requested ref, the commit it pointed to, and a digest of the imported files.
// With a lock, generation is reproducible even when a remote branch moves, and a
// tampered or swapped tree fails verification instead of becoming instructions.
package lockfile

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"

	"github.com/Goldziher/ai-rulez/v5/internal/semver"
	"github.com/pelletier/go-toml/v2"
	"github.com/samber/oops"
)

// FileName is the lock file, kept in the configuration directory.
const FileName = "ai-rulez.lock"

// Version is the lock format version. It is the only version field: the content
// hashing scheme of internal/contentlock (labels "ai-rulez/<kind>/v1") is part
// of the format, so a change to either is a new Version.
const Version = 1

// Entry kinds.
const (
	KindInclude = "include"
	KindSkill   = "skill"
	// KindSource pins one [[skill_sources]] entry: the commit its ref resolved
	// to and the digest of the fetched tree.
	KindSource = "source"
	// KindServed pins one served skill by name: Digest is the digest the skills
	// server reports (over the rendered files). [lock] enforce checks against it.
	KindServed = "served"
)

// Entry pins one remote source. Source has credentials redacted: the lock is committed.
type Entry struct {
	Name   string `toml:"name"`
	Source string `toml:"source"`
	Path   string `toml:"path,omitempty"`
	// Ref is the ref the config requested ("" when it set none, meaning HEAD).
	// For a source that asks for a version range it is the constraint as
	// written ("^1.2"), and Tag records what the range resolved to.
	Ref string `toml:"ref,omitempty"`
	// Tag is the tag a version constraint resolved to ("" for a plain ref).
	Tag string `toml:"tag,omitempty"`
	// TagObject is the annotated tag object id of Tag; "" for a lightweight tag.
	TagObject string `toml:"tag_object,omitempty"`
	// Commit is the full SHA Ref resolved to when the lock was written (for a
	// tag, the peeled commit).
	Commit string `toml:"commit"`
	// Digest is "sha256:<hex>" over the imported file tree (contentlock.DigestDir).
	Digest string `toml:"digest"`
	// View names the serve view a KindServed entry belongs to ("role:backend",
	// "profile:x", "static", joined by "+"); empty for the default view and for
	// every other kind, so a project without views writes the same lock as before.
	View string `toml:"view,omitempty"`
}

// Item pins one authored item: a rule, skill, agent, command, context file,
// hook, role or settings source. Digest is "sha256:<hex>" over its raw files
// (scheme in docs/lockfile.md).
type Item struct {
	Kind   string `toml:"kind"`
	ID     string `toml:"id"`
	Domain string `toml:"domain,omitempty"`
	// Path is the item's source, relative to the configuration directory with
	// forward slashes; empty for items declared in config.toml.
	Path    string `toml:"path,omitempty"`
	Digest  string `toml:"digest"`
	Owner   string `toml:"owner,omitempty"`
	Version string `toml:"version,omitempty"`
}

// Key identifies an item independently of its digest.
func (i Item) Key() string { return i.Kind + "\x00" + i.Domain + "\x00" + i.ID }

// OutputPin pins one generated output file: its path relative to the project
// root and the digest of its header-free rendering.
type OutputPin struct {
	Path   string `toml:"path"`
	Digest string `toml:"digest"`
}

// File is the parsed lock.
type File struct {
	Version int `toml:"version"`
	// AIRulezVersion is the ai-rulez release that wrote the content pins.
	AIRulezVersion string `toml:"ai_rulez_version,omitempty"`
	// Profile is the profile the output pins were rendered for ("" = the default).
	Profile string `toml:"profile,omitempty"`
	// Scope and OutputsPinned record the [lock] settings the pins were written with.
	Scope         string `toml:"scope,omitempty"`
	OutputsPinned bool   `toml:"outputs_pinned,omitempty"`
	// Tree is the "sha256:<hex>" digest over every content pin and remote entry.
	Tree    string  `toml:"tree,omitempty"`
	Include []Entry `toml:"include,omitempty"`
	Skill   []Entry `toml:"skill,omitempty"`
	// Source pins a [[skill_sources]] entry (commit and tree digest); Served pins
	// the digest of a skill the skills server serves.
	Source []Entry     `toml:"source,omitempty"`
	Served []Entry     `toml:"served,omitempty"`
	Item   []Item      `toml:"item,omitempty"`
	Output []OutputPin `toml:"output,omitempty"`
}

// HasContentPins reports whether the lock pins authored content.
func (f *File) HasContentPins() bool {
	return f != nil && (f.Tree != "" || len(f.Item) > 0 || len(f.Output) > 0)
}

// Path returns the lock path for a configuration directory.
func Path(configDir string) string { return filepath.Join(configDir, FileName) }

// Load reads the lock in configDir. A missing file returns (nil, nil).
func Load(configDir string) (*File, error) {
	f, err := read(configDir)
	if err != nil || f == nil {
		return nil, err
	}
	if f.Version != Version {
		return nil, oops.With("path", Path(configDir)).
			Hint("Run `ai-rulez lock` to regenerate it").
			Errorf("unsupported lock version %d (this ai-rulez reads version %d): run `ai-rulez lock` to regenerate %s", f.Version, Version, FileName)
	}
	return f, nil
}

func read(configDir string) (*File, error) {
	data, err := os.ReadFile(Path(configDir))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, oops.With("path", Path(configDir)).Wrapf(err, "read lock file")
	}
	var f File
	if err := toml.Unmarshal(data, &f); err != nil {
		return nil, oops.With("path", Path(configDir)).Wrapf(err, "parse lock file")
	}
	return &f, nil
}

// Save writes the lock deterministically: entries sorted by name, items by
// kind, domain and id, outputs by path, no timestamps. It always writes the
// current format version.
func Save(configDir string, f *File) error {
	out := *f
	out.Version = Version
	out.Include, out.Skill = sorted(f.Include), sorted(f.Skill)
	out.Source, out.Served = sorted(f.Source), sorted(f.Served)
	out.Item, out.Output = sortedItems(f.Item), sortedOutputs(f.Output)
	var buf bytes.Buffer
	buf.WriteString("# ai-rulez.lock: pins remote includes, installed skills and authored content. Commit this file.\n")
	buf.WriteString("# Refresh it with `ai-rulez lock`; `ai-rulez generate --locked` fails when it is stale.\n\n")
	enc := toml.NewEncoder(&buf)
	if err := enc.Encode(out); err != nil {
		return oops.Wrapf(err, "encode lock file")
	}
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return oops.With("dir", configDir).Wrapf(err, "create config directory")
	}
	if err := os.WriteFile(Path(configDir), buf.Bytes(), 0o644); err != nil {
		return oops.With("path", Path(configDir)).Wrapf(err, "write lock file")
	}
	return nil
}

func sortedItems(in []Item) []Item {
	out := append([]Item(nil), in...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
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
	return out
}

func sortedOutputs(in []OutputPin) []OutputPin {
	out := append([]OutputPin(nil), in...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func sorted(in []Entry) []Entry {
	out := append([]Entry(nil), in...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].View < out[j].View
	})
	return out
}

// Find returns the entry for kind and name (of the default view), or nil.
func (f *File) Find(kind, name string) *Entry { return f.FindView(kind, name, "") }

// FindView returns the entry for kind and name in the given serve view, or nil.
func (f *File) FindView(kind, name, view string) *Entry {
	if f == nil {
		return nil
	}
	list := *f.list(kind)
	for i := range list {
		if list[i].Name == name && list[i].View == view {
			return &list[i]
		}
	}
	return nil
}

// list returns the entries of a kind; an unknown kind means includes.
func (f *File) list(kind string) *[]Entry {
	switch kind {
	case KindSkill:
		return &f.Skill
	case KindSource:
		return &f.Source
	case KindServed:
		return &f.Served
	}
	return &f.Include
}

// Set replaces or adds an entry of the given kind, keyed by name and view.
func (f *File) Set(kind string, e Entry) {
	list := f.list(kind)
	for i := range *list {
		if (*list)[i].Name == e.Name && (*list)[i].View == e.View {
			(*list)[i] = e
			return
		}
	}
	*list = append(*list, e)
}

// Want is one configured remote source that the lock should cover. Source is
// already redacted.
type Want struct {
	Kind, Name, Source, Path, Ref string
	// Constraint is the version range of a source that asks for one; Ref then
	// holds the same text (what the lock records as the requested ref).
	Constraint string
	// TagPrefix and IncludePrerelease refine Constraint.
	TagPrefix         string
	IncludePrerelease bool
}

// Covers reports whether e pins exactly the source the config asks for. A
// changed source, path or ref makes the pin stale; for a version constraint the
// pinned tag must also still satisfy it, so editing the constraint, the tag
// prefix or the prerelease switch invalidates the pin until `ai-rulez lock`.
func (e *Entry) Covers(w Want) bool {
	if e == nil || e.Source != w.Source || e.Path != w.Path || e.Ref != w.Ref {
		return false
	}
	return w.Constraint == "" || e.TagSatisfies(w)
}

// TagSatisfies reports whether the entry's tag is a version tag of w's prefix
// that w's constraint allows. An entry with no tag does not.
func (e *Entry) TagSatisfies(w Want) bool {
	if e == nil || e.Tag == "" {
		return false
	}
	v, ok := semver.ParseTag(e.Tag, w.TagPrefix)
	if !ok {
		return false
	}
	c, err := semver.ParseConstraint(w.Constraint)
	return err == nil && c.Check(v, w.IncludePrerelease)
}

// IsFullSHA reports whether ref is a full 40-hex commit SHA.
func IsFullSHA(ref string) bool {
	if len(ref) != 40 {
		return false
	}
	for _, c := range ref {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
