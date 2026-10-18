// Package harnessnames holds the per-harness tables of built-in command, skill and
// agent names (data/<preset>.toml) and what a harness does when a user item has the
// same name. The lint rules AR808, AR9U0 and AR9U1 read it; ai-rulez does not
// invent a name: a group carries the page it was taken from and, where the outcome of
// a collision is recorded, a verbatim quote that documents it.
package harnessnames

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"sync"

	toml "github.com/pelletier/go-toml/v2"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

//go:embed data/*.toml
var dataFS embed.FS

// SchemaVersion is the value of the schema key of every file.
const SchemaVersion = 1

// Kinds of a group. A reserved group holds names the harness refuses to load.
const (
	KindCommand  = "command"
	KindSkill    = "skill"
	KindAgent    = "agent"
	KindReserved = "reserved"
)

// Collision outcomes: what the harness does with a user item named like a built-in.
const (
	UserWins    = "user_wins"    // the user item replaces the built-in
	BuiltinWins = "builtin_wins" // the built-in stays; the user item is renamed, ignored or unreachable
	BothListed  = "both_listed"  // both exist and are listed
	Error       = "error"        // the harness refuses to load the name
	NotApplied  = "n/a"          // no user item of this kind can collide
	Unknown     = "unknown"      // a built-in the vendor documents; the outcome is not documented
)

// Match modes of a group.
const (
	MatchExact  = "exact"
	MatchPrefix = "prefix"
)

// Completeness values: how much of the harness's built-in set the table holds.
const (
	Complete = "complete"
	Partial  = "partial"
	DocsOnly = "docs-only"
	None     = "none"
)

// Severities a file may set for its findings.
const (
	SeverityError   = "error"
	SeverityWarning = "warning"
)

// Source is a page a name or a claim was taken from.
type Source struct {
	ID         string `toml:"id"`
	URL        string `toml:"url"`
	Quote      string `toml:"quote"`
	VerifiedOn string `toml:"verified_on"`
}

// Group is a set of names of one kind with one collision outcome.
type Group struct {
	Kind string `toml:"kind"`
	// Source is the id of the source the names were taken from.
	Source string `toml:"source"`
	// Evidence is the id of the source whose quote documents UserCollision; empty
	// means Source.
	Evidence      string              `toml:"evidence"`
	UserCollision string              `toml:"user_collision"`
	Match         string              `toml:"match"`
	Names         []string            `toml:"names"`
	Aliases       map[string][]string `toml:"aliases"`
}

// Table is the file of one preset.
type Table struct {
	Schema         int    `toml:"schema"`
	Preset         string `toml:"preset"`
	HarnessVersion string `toml:"harness_version"`
	VerifiedOn     string `toml:"verified_on"`
	Completeness   string `toml:"completeness"`
	// Severity is the severity of a finding whose collision outcome is quoted: error
	// or warning (the default).
	Severity string `toml:"severity"`
	// InvocationSpaces lists the kinds that share one namespace: a skill and a command
	// of the same name collide when both are in one space.
	InvocationSpaces [][]string `toml:"invocation_spaces"`
	// Namespaced says the harness always invokes plugin outputs under a prefix.
	Namespaced bool     `toml:"namespaced"`
	Source     []Source `toml:"source"`
	Group      []Group  `toml:"group"`

	// Project is set on a table merged from .ai-rulez/harness-names.
	Project bool `toml:"-"`
	// declared maps the groups added from a project file (by index into Group) to the
	// severity of their findings.
	declared map[int]string
	index    map[string][]entry // normalized name -> entries
	prefixes []entry
	sources  map[string]*Source
}

type entry struct {
	group   int
	name    string // as recorded
	alias   bool
	aliasOf string
}

// embedded reads the embedded tables once; a Table is immutable after that.
var embedded = sync.OnceValues(loadEmbedded)

func loadEmbedded() (map[string]*Table, error) {
	tables := map[string]*Table{}
	entries, err := fs.ReadDir(dataFS, "data")
	if err != nil {
		return nil, fmt.Errorf("read harness name tables: %w", err)
	}
	for _, e := range entries {
		data, err := dataFS.ReadFile("data/" + e.Name())
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", e.Name(), err)
		}
		t, err := Parse(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		tables[t.Preset] = t
	}
	return tables, nil
}

// Parse decodes one table file and builds its lookup index. Unknown keys are an error.
func Parse(data []byte) (*Table, error) {
	var t Table
	dec := toml.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&t); err != nil {
		return nil, fmt.Errorf("parse harness names: %w", err)
	}
	t.reindex()
	return &t, nil
}

// Load returns the error, if any, of reading the embedded tables.
func Load() error {
	_, err := embedded()
	return err
}

// For returns the table of a preset.
func For(preset string) (*Table, bool) {
	tables, _ := embedded() //nolint:errcheck // an unreadable table set has no tables; Load reports it
	t, ok := tables[strings.ToLower(preset)]
	return t, ok
}

// Presets lists the presets that have a table, sorted.
func Presets() []string {
	tables, _ := embedded() //nolint:errcheck // an unreadable table set has no tables; Load reports it
	out := make([]string, 0, len(tables))
	for p := range tables {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// Normalize returns a name the way a harness compares it: surrounding space trimmed,
// NFKC-normalized and case-folded. A cases.Caser is not safe for concurrent use, so each
// call makes its own.
func Normalize(name string) string {
	return cases.Fold().String(norm.NFKC.String(strings.TrimSpace(name)))
}

func (t *Table) reindex() {
	t.index = map[string][]entry{}
	t.prefixes = nil
	t.sources = map[string]*Source{}
	for i := range t.Source {
		t.sources[t.Source[i].ID] = &t.Source[i]
	}
	for gi := range t.Group {
		g := &t.Group[gi]
		add := func(e entry) {
			if g.Match == MatchPrefix {
				t.prefixes = append(t.prefixes, entry{group: gi, name: Normalize(e.name), alias: e.alias, aliasOf: e.aliasOf})
				return
			}
			key := Normalize(e.name)
			t.index[key] = append(t.index[key], e)
		}
		for _, n := range g.Names {
			add(entry{group: gi, name: n})
			for _, a := range g.Aliases[n] {
				add(entry{group: gi, name: a, alias: true, aliasOf: n})
			}
		}
	}
}

// SourceByID returns a source of the table.
func (t *Table) SourceByID(id string) (*Source, bool) {
	s, ok := t.sources[id]
	return s, ok
}

// Hit is a table entry a name matched.
type Hit struct {
	Preset       string
	Name         string // the recorded name (or alias)
	Alias        bool
	AliasOf      string // the command an alias belongs to
	Kind         string // kind of the group
	Collision    string
	Match        string
	Source       *Source
	Evidence     *Source // the source that documents Collision; nil when none
	Quoted       bool    // Evidence carries a quote
	Declared     bool    // from the project, not the embedded table
	Completeness string
	Version      string
	VerifiedOn   string
	Severity     string // file severity: error or warning
}

// space reports whether a group of kind gk can collide with an item of kind k.
func (t *Table) space(k, gk string) bool {
	if gk == KindReserved {
		return k != KindAgent
	}
	if k == gk {
		return true
	}
	for _, s := range t.InvocationSpaces {
		var a, b bool
		for _, x := range s {
			a = a || x == k
			b = b || x == gk
		}
		if a && b {
			return true
		}
	}
	return false
}

// Find returns the entries an item of the kind (skill, command or agent) and the name
// collides with, strongest outcome first. The name is normalized here.
func (t *Table) Find(kind, name string) []Hit {
	if t == nil {
		return nil
	}
	key := Normalize(name)
	var hits []Hit
	add := func(e entry) {
		g := &t.Group[e.group]
		if !t.space(kind, g.Kind) {
			return
		}
		hits = append(hits, t.hit(e, g))
	}
	for _, e := range t.index[key] {
		add(e)
	}
	for _, e := range t.prefixes {
		if strings.HasPrefix(key, e.name) {
			add(e)
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return rank(hits[i].Collision) < rank(hits[j].Collision) })
	return hits
}

func rank(c string) int {
	switch c {
	case Error:
		return 0
	case BuiltinWins:
		return 1
	case UserWins:
		return 2
	case BothListed:
		return 3
	case Unknown:
		return 4
	}
	return 5
}

func (t *Table) hit(e entry, g *Group) Hit {
	h := Hit{
		Preset: t.Preset, Name: e.name, Alias: e.alias, AliasOf: e.aliasOf, Kind: g.Kind,
		Collision: g.UserCollision, Match: g.Match,
		Completeness: t.Completeness, Version: t.HarnessVersion, VerifiedOn: t.VerifiedOn, Severity: t.Severity,
	}
	if sev, ok := t.declared[e.group]; ok {
		h.Declared, h.Severity = true, sev
	}
	h.Source = t.sources[g.Source]
	ev := g.Evidence
	if ev == "" {
		ev = g.Source
	}
	if s := t.sources[ev]; s != nil {
		h.Evidence = s
		h.Quoted = strings.TrimSpace(s.Quote) != ""
	}
	return h
}

// Entry is one recorded name.
type Entry struct {
	Name    string
	Alias   bool
	AliasOf string
}

// Entries lists the recorded names (and aliases) of the groups of the kinds whose
// collision outcome is one of the outcomes; with no outcome every group matches.
func (t *Table) Entries(kinds []string, outcomes ...string) []Entry {
	var out []Entry
	for gi := range t.Group {
		g := &t.Group[gi]
		if g.Match == MatchPrefix || !contains(kinds, g.Kind) || (len(outcomes) > 0 && !contains(outcomes, g.UserCollision)) {
			continue
		}
		for _, n := range g.Names {
			out = append(out, Entry{Name: n})
			for _, a := range g.Aliases[n] {
				out = append(out, Entry{Name: a, Alias: true, AliasOf: n})
			}
		}
	}
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
