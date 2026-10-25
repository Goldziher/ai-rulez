// Package repoindex indexes the agent configuration a repository keeps at any
// depth: skills, instruction files (AGENTS.md, CLAUDE.md, GEMINI.md, Copilot
// instructions), rules, agents and commands. Each item belongs to a scope, the
// directory that holds the convention file or directory, and a path resolves to
// the scopes that are the path or one of its ancestors, nearest first, as a
// harness would load them. The index stores summaries only, never file bodies.
package repoindex

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/samber/oops"
)

// Kind is what an indexed item is.
type Kind string

// The item kinds.
const (
	KindSkill       Kind = "skill"
	KindInstruction Kind = "instruction"
	KindRule        Kind = "rule"
	KindAgent       Kind = "agent"
	KindCommand     Kind = "command"
)

// RootScope is the scope of the project root.
const RootScope = "."

// DefaultMaxItems bounds the items one index holds.
const DefaultMaxItems = 20000

// Item is one indexed unit.
type Item struct {
	Kind        Kind   `json:"kind"`
	Scope       string `json:"scope"`
	Source      string `json:"source"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Hash        string `json:"hash"`
	Path        string `json:"path"`
	// Only, when set, restricts a rule or a skill to the files that match it; nil
	// means it applies everywhere in its scope.
	Only *PathGlobs `json:"only,omitempty"`
}

// PathGlobs are the globs a rule or skill names the files it applies to by,
// relative to its scope: `paths:` in the Claude Code form (rules and skills),
// `globs:` in a Cursor rule that is not always applied.
type PathGlobs struct {
	Paths []string `json:"paths,omitempty"`
	Globs []string `json:"globs,omitempty"`
}

// Scope is a directory that holds convention files.
type Scope struct {
	Path    string       `json:"scope"`
	Sources []string     `json:"sources"`
	Counts  map[Kind]int `json:"counts"`
}

// Refused is an item a Screen turned away.
type Refused struct {
	Item   Item
	Reason string
}

// Problem is a path the index could not read.
type Problem struct {
	Path string
	Err  string
}

// MaxFileBytes bounds a file the index reads: a larger one is reported as a
// Problem and left out.
const MaxFileBytes = 1 << 20

// maxDescriptionBytes bounds the description an item carries.
const maxDescriptionBytes = 500

// ErrTooLarge is the error of a file over the size the index reads.
var ErrTooLarge = errors.New("the file is larger than the index reads")

// Options configure Build.
type Options struct {
	// Root is the project root; every path the index holds is relative to it.
	Root string
	// Presets are the enabled presets; their skills directories are indexed, and
	// the skills and .claude/skills directories always are. Rules, agents and
	// commands are indexed in every known convention, whatever the presets.
	Presets []string
	// MaxItems caps the items held (0 selects DefaultMaxItems). Past it the
	// shallowest scopes are kept.
	MaxItems int
	// Skip lists project-relative directories the index never enters.
	Skip []string
	// Exclude lists globs, relative to Root and slash separated, of the paths the
	// index never enters: a file a glob matches, or one below a directory it
	// matches, is not read and marks no scope. A pattern without a slash matches
	// at any depth and a trailing slash names a directory.
	Exclude []string
	// Files, when set, is the file listing to index instead of listing the
	// project (slash separated, relative to Root).
	Files []string
	// Generated, when set, reports whether the marker file at an absolute path
	// (SKILL.md for a skill) was written by a generator; such an item is left out.
	Generated func(path string, content []byte) bool
	// Screen, when set, judges an item with its content in hand. keep false with
	// an empty reason drops it silently; with a reason it is recorded as refused.
	Screen func(it Item, content []byte) (keep bool, reason string)
	// Cache, when set, lets Build skip reading a file whose size and
	// modification time are those of an earlier build (see Cache).
	Cache Cache
	// ScreenKnown, when set, judges an item that came from the cache by its
	// hash alone, so Screen need not read its content: known false means the
	// content is needed and Screen runs. It does not apply to an item that was
	// read.
	ScreenKnown func(it Item) (keep bool, reason string, known bool)
}

// CachedItem is the summary of one file a Cache keeps. It never holds the file
// body.
type CachedItem struct {
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Hash        string     `json:"hash"`
	Only        *PathGlobs `json:"only,omitempty"`
	// Generated records that a generator's header was found in the file.
	Generated bool `json:"generated,omitempty"`
}

// Cache holds the summaries of files by path, size and modification time
// (nanoseconds since the epoch). An implementation decides how far to trust an
// entry; Build only asks.
type Cache interface {
	Lookup(file string, size, mtime int64) (CachedItem, bool)
	Store(file string, size, mtime int64, item CachedItem)
}

// Index is the result of Build. It is immutable.
type Index struct {
	items      []Item
	scopes     []Scope
	scopeByKey map[string]*Scope
	byScope    map[string][]Item
	lookup     map[string]Item
	roots      []string
	generated  map[Kind]int
	refused    []Refused
	problems   []Problem
	found      int
	limit      int
}

// Build indexes the project at opts.Root: it lists the files not ignored (git's
// listing, else a walk), recognizes the units that mark an item, reads each one
// for its description and hash, and groups them into scopes. It never follows a
// symlink and never keeps a file body.
func Build(ctx context.Context, opts Options) (*Index, error) {
	if opts.Root == "" {
		return nil, oops.Errorf("build the repository index: no project root")
	}
	limit := opts.MaxItems
	if limit <= 0 {
		limit = DefaultMaxItems
	}
	exclude, err := NewExcluder(opts.Exclude)
	if err != nil {
		return nil, err
	}
	files := opts.Files
	if files == nil {
		files = listFiles(ctx, opts.Root)
	}
	b := &builder{opts: opts, limit: limit, exclude: exclude, markers: map[string]map[string]bool{}}
	b.collect(files)
	b.read()
	return b.finish(), nil
}

type builder struct {
	opts    Options
	limit   int
	exclude *Excluder
	// cands are the units to read, in index order, after the cap.
	cands []candidate
	found int
	roots []string
	// markers holds the scopes that exist by a convention directory alone, with their sources.
	markers map[string]map[string]bool

	items     []Item
	refused   []Refused
	problems  []Problem
	generated map[Kind]int
}

// collect classifies the listed files and applies the cap.
func (b *builder) collect(files []string) {
	m := newMatcher(b.opts.Presets, b.opts.Skip, b.exclude)
	var skills, others []candidate
	for _, file := range files {
		segs := strings.Split(file, "/")
		if m.skipped(segs, file) {
			continue
		}
		if scope, source, ok := marker(segs); ok {
			b.mark(scope, source)
		}
		c, ok := m.classify(segs, file)
		switch {
		case !ok:
		case c.kind == KindSkill:
			skills = append(skills, c)
		default:
			others = append(others, c)
		}
	}
	skills = outermost(skills)
	inside := map[string]bool{}
	rootSet := map[string]bool{}
	for _, c := range skills {
		inside[c.unit] = true
		rootSet[c.root] = true
	}
	all := skills
	for _, c := range others {
		if !withinAny(c.file, inside) {
			all = append(all, c)
		}
	}
	for root := range rootSet {
		b.roots = append(b.roots, root)
	}
	sort.Strings(b.roots)
	sort.Slice(all, func(i, j int) bool {
		a, c := all[i], all[j]
		if da, dc := depth(a.scope), depth(c.scope); da != dc {
			return da < dc
		}
		if a.scope != c.scope {
			return a.scope < c.scope
		}
		return a.unit < c.unit
	})
	b.found = len(all)
	if len(all) > b.limit {
		all = all[:b.limit]
	}
	b.cands = all
}

// withinAny reports whether a file lies inside one of the directories.
func withinAny(file string, dirs map[string]bool) bool {
	for dir := parentOf(file); dir != RootScope; dir = parentOf(dir) {
		if dirs[dir] {
			return true
		}
	}
	return false
}

func (b *builder) mark(scope, source string) {
	if b.markers[scope] == nil {
		b.markers[scope] = map[string]bool{}
	}
	b.markers[scope][source] = true
}

// read turns each candidate into an item.
func (b *builder) read() {
	b.generated = map[Kind]int{}
	for i := range b.cands {
		c := &b.cands[i]
		if b.opts.Cache == nil {
			content, ok, err := ReadRegular(b.opts.Root, c.file, MaxFileBytes)
			if b.readFailed(c, ok, err) {
				continue
			}
			b.admit(c, content)
			continue
		}
		b.readCached(c)
	}
}

// readFailed records a read error as a problem and reports whether the
// candidate is to be skipped (an error, or a path that is gone or no regular file).
func (b *builder) readFailed(c *candidate, ok bool, err error) bool {
	if err != nil {
		b.problems = append(b.problems, Problem{Path: c.file, Err: err.Error()})
		return true
	}
	return !ok
}

// admit builds the item of a candidate whose content was read, applies
// Generated and Screen, and keeps it. It returns the item and whether it was
// generated, for the cache.
func (b *builder) admit(c *candidate, content []byte) (it Item, generated bool) {
	if b.opts.Generated != nil && b.opts.Generated(absOf(b.opts.Root, c.file), content) {
		b.generated[c.kind]++
		return newItem(c, content), true
	}
	it = newItem(c, content)
	if b.opts.Screen != nil {
		keep, reason := b.opts.Screen(it, content)
		if !keep {
			if reason != "" {
				b.refused = append(b.refused, Refused{Item: it, Reason: reason})
			}
			return it, false
		}
	}
	b.items = append(b.items, it)
	return it, false
}

// readCached reads a candidate through the cache: a file whose size and
// modification time are unchanged is not read again.
func (b *builder) readCached(c *candidate) {
	abs := filepath.Join(b.opts.Root, filepath.FromSlash(c.file))
	info, err := os.Lstat(abs)
	switch {
	case err != nil && os.IsNotExist(err):
		return
	case err != nil:
		b.problems = append(b.problems, Problem{Path: c.file, Err: err.Error()})
		return
	case !info.Mode().IsRegular():
		return
	case info.Size() > MaxFileBytes:
		b.problems = append(b.problems, Problem{Path: c.file, Err: ErrTooLarge.Error()})
		return
	}
	size, mtime := info.Size(), info.ModTime().UnixNano()
	if ci, ok := b.opts.Cache.Lookup(c.file, size, mtime); ok && b.fromCache(c, ci, abs) {
		return
	}
	content, ok, err := ReadRegular(b.opts.Root, c.file, MaxFileBytes)
	if b.readFailed(c, ok, err) {
		return
	}
	it, generated := b.admit(c, content)
	b.opts.Cache.Store(c.file, size, mtime, CachedItem{
		Name: it.Name, Description: it.Description, Hash: it.Hash, Only: it.Only, Generated: generated,
	})
}

// fromCache applies a cached summary. It reports false when the item cannot be
// judged without its content, which the caller then reads.
func (b *builder) fromCache(c *candidate, ci CachedItem, abs string) bool {
	if ci.Generated || (b.opts.Generated != nil && b.opts.Generated(abs, nil)) {
		b.generated[c.kind]++
		return true
	}
	it := Item{Kind: c.kind, Scope: c.scope, Source: c.source, Path: c.unit, Name: ci.Name, Description: ci.Description, Hash: ci.Hash, Only: ci.Only}
	if b.opts.Screen != nil {
		if b.opts.ScreenKnown == nil {
			return false
		}
		keep, reason, known := b.opts.ScreenKnown(it)
		if !known {
			return false
		}
		if !keep {
			if reason != "" {
				b.refused = append(b.refused, Refused{Item: it, Reason: reason})
			}
			return true
		}
	}
	b.items = append(b.items, it)
	return true
}

// finish groups the items into scopes.
func (b *builder) finish() *Index {
	b.namespaceCollisions()
	ix := &Index{
		byScope: map[string][]Item{}, lookup: map[string]Item{}, scopeByKey: map[string]*Scope{},
		roots: b.roots, generated: b.generated, refused: b.refused, problems: b.problems,
		found: b.found, limit: b.limit,
	}
	sortItems(b.items)
	ix.items = b.items
	sources := map[string]map[string]bool{RootScope: {}}
	for scope, set := range b.markers {
		sources[scope] = set
	}
	for _, it := range b.items {
		ix.byScope[it.Scope] = append(ix.byScope[it.Scope], it)
		if sources[it.Scope] == nil {
			sources[it.Scope] = map[string]bool{}
		}
		sources[it.Scope][it.Source] = true
		if it.Kind != KindSkill {
			ix.lookup[lookupKey(it.Scope, it.Name)] = it
		}
	}
	for path, set := range sources {
		s := Scope{Path: path, Counts: map[Kind]int{}}
		for source := range set {
			s.Sources = append(s.Sources, source)
		}
		sort.Strings(s.Sources)
		for _, it := range ix.byScope[path] {
			s.Counts[it.Kind]++
		}
		ix.scopes = append(ix.scopes, s)
	}
	sort.Slice(ix.scopes, func(i, j int) bool {
		if di, dj := depth(ix.scopes[i].Path), depth(ix.scopes[j].Path); di != dj {
			return di < dj
		}
		return ix.scopes[i].Path < ix.scopes[j].Path
	})
	for i := range ix.scopes {
		ix.scopeByKey[ix.scopes[i].Path] = &ix.scopes[i]
	}
	return ix
}

// namespaceCollisions renames the non-skill items of one scope that share a
// name: the first by path keeps it, the others take their path from the root as
// discovery names a colliding skill. One that still collides is a problem.
func (b *builder) namespaceCollisions() {
	sort.SliceStable(b.items, func(i, j int) bool {
		if b.items[i].Scope != b.items[j].Scope {
			return b.items[i].Scope < b.items[j].Scope
		}
		return b.items[i].Path < b.items[j].Path
	})
	taken := map[string]bool{}
	kept := b.items[:0]
	for _, it := range b.items {
		if it.Kind == KindSkill {
			kept = append(kept, it)
			continue
		}
		key := lookupKey(it.Scope, it.Name)
		if taken[key] {
			it.Name = safeName(NamespacedName(strings.TrimSuffix(it.Path, pathExt(it.Path))))
			key = lookupKey(it.Scope, it.Name)
			if taken[key] {
				b.problems = append(b.problems, Problem{Path: it.Path, Err: "the name " + it.Name + " is already taken in the scope"})
				continue
			}
		}
		taken[key] = true
		kept = append(kept, it)
	}
	b.items = kept
}

func lookupKey(scope, name string) string { return scope + "\x00" + name }

func absOf(root, rel string) string { return strings.TrimRight(root, "/\\") + "/" + rel }

// Items lists every item: the shallowest scope first, then by kind and name.
func (ix *Index) Items() []Item { return append([]Item(nil), ix.items...) }

// Skills lists the skill items, shallowest skill directory first, then by path.
func (ix *Index) Skills() []Item {
	var out []Item
	for _, it := range ix.items {
		if it.Kind == KindSkill {
			out = append(out, it)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if di, dj := strings.Count(out[i].Path, "/"), strings.Count(out[j].Path, "/"); di != dj {
			return di < dj
		}
		return out[i].Path < out[j].Path
	})
	return out
}

// Scopes lists the scopes, the root first, then by depth and path.
func (ix *Index) Scopes() []Scope { return append([]Scope(nil), ix.scopes...) }

// Scope returns one scope by its path.
func (ix *Index) Scope(p string) (Scope, bool) {
	s, ok := ix.scopeByKey[p]
	if !ok {
		return Scope{}, false
	}
	return *s, true
}

// Lookup returns the rule, agent, command or instruction item named name in a
// scope. Skills are found through Skills, whose names are not unique.
func (ix *Index) Lookup(scope, name string) (Item, bool) {
	it, ok := ix.lookup[lookupKey(scope, name)]
	return it, ok
}

// Found is the number of candidate items before the cap.
func (ix *Index) Found() int { return ix.found }

// Limit is the item cap in effect.
func (ix *Index) Limit() int { return ix.limit }

// Truncated reports whether the cap cut the index.
func (ix *Index) Truncated() bool { return ix.found > ix.limit }

// SkillRoots lists the skills directories that hold a skill, generated ones
// included, so a watcher can follow them.
func (ix *Index) SkillRoots() []string { return append([]string(nil), ix.roots...) }

// GeneratedSkipped counts the items of a kind left out because a generator wrote them.
func (ix *Index) GeneratedSkipped(kind Kind) int { return ix.generated[kind] }

// Refusals lists the items a Screen turned away with a reason.
func (ix *Index) Refusals() []Refused { return append([]Refused(nil), ix.refused...) }

// Problems lists the paths that could not be read.
func (ix *Index) Problems() []Problem { return append([]Problem(nil), ix.problems...) }

func sortItems(items []Item) {
	sort.SliceStable(items, func(i, j int) bool {
		a, c := items[i], items[j]
		if da, dc := depth(a.Scope), depth(c.Scope); da != dc {
			return da < dc
		}
		if a.Scope != c.Scope {
			return a.Scope < c.Scope
		}
		if a.Kind != c.Kind {
			return a.Kind < c.Kind
		}
		if a.Name != c.Name {
			return a.Name < c.Name
		}
		return a.Path < c.Path
	})
}
