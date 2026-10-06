package govview

import (
	"bytes"
	"encoding/json"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/samber/oops"
)

// CatalogDiffSchemaVersion versions the JSON of `ai-rulez catalog diff --format json`.
const CatalogDiffSchemaVersion = 1

// DiffChange is one field of an item, server or role that differs.
type DiffChange struct {
	Field string `json:"field"`
	From  string `json:"from"`
	To    string `json:"to"`
}

// DiffEntry is an item, MCP server or role that exists on one side only.
type DiffEntry struct {
	Ref  string `json:"ref"`
	Kind string `json:"kind,omitempty"`
	// Tokens is the load cost of an added or removed item (listing plus body).
	Tokens int `json:"tokens,omitempty"`
}

// DiffChanged is an entry present on both sides whose fields differ.
type DiffChanged struct {
	Ref     string       `json:"ref"`
	Kind    string       `json:"kind,omitempty"`
	Changes []DiffChange `json:"changes"`
}

// DiffSection groups what differs for items, MCP servers or roles.
type DiffSection struct {
	Added   []DiffEntry   `json:"added"`
	Removed []DiffEntry   `json:"removed"`
	Changed []DiffChanged `json:"changed"`
}

// Empty reports whether nothing differs.
func (s *DiffSection) Empty() bool {
	return len(s.Added)+len(s.Removed)+len(s.Changed) == 0
}

// DiffSide describes one input of a diff.
type DiffSide struct {
	// Label is what the caller named: a revision, a file name or "working tree".
	Label string `json:"label"`
	// Commit is the commit a revision resolved to; empty for a file or the working tree.
	Commit   string `json:"commit,omitempty"`
	LockTree string `json:"lock_tree,omitempty"`
	Items    int    `json:"items"`
	// ListingTokens and BodyTokens total the load cost of every item.
	ListingTokens int `json:"listing_tokens"`
	BodyTokens    int `json:"body_tokens"`
}

// CatalogDiff is the difference between two version 2 catalogs.
type CatalogDiff struct {
	SchemaVersion int         `json:"schema_version"`
	From          DiffSide    `json:"from"`
	To            DiffSide    `json:"to"`
	Identical     bool        `json:"identical"`
	Items         DiffSection `json:"items"`
	MCPServers    DiffSection `json:"mcp_servers"`
	Roles         DiffSection `json:"roles"`
	// Edges are the dependencies between items ("from -> to"); only added and
	// removed ones exist.
	Edges DiffSection `json:"edges"`
	// Lint compares the project lint totals of the two sides; null when either side has none.
	Lint *LintDiff `json:"lint"`
	// Notes say what the comparison could not cover.
	Notes []string `json:"notes"`
}

// LintDiff is the project-wide lint summary of both sides.
type LintDiff struct {
	From LintSummary `json:"from"`
	To   LintSummary `json:"to"`
}

// DiffCatalogs compares two version 2 catalogs. Items, servers and roles are
// matched by their stable ref (roles by name); the result is sorted and
// deterministic, and the inputs are not modified. Excerpts are not compared: the
// digest covers the body.
func DiffCatalogs(from, to *CatalogDocV2, fromSide, toSide DiffSide) (*CatalogDiff, error) {
	for _, d := range []*CatalogDocV2{from, to} {
		if d == nil || d.SchemaVersion != CatalogSchemaVersionV2 {
			return nil, oops.Hint("diff needs version 2 catalogs: `ai-rulez catalog --format json --schema-version 2`").
				Errorf("not a version 2 catalog")
		}
	}
	out := &CatalogDiff{SchemaVersion: CatalogDiffSchemaVersion, From: fromSide, To: toSide, Notes: []string{}}
	out.From = fillSide(out.From, from)
	out.To = fillSide(out.To, to)
	out.Items = diffItems(from.Items, to.Items)
	out.MCPServers = diffMCP(from.MCPServers, to.MCPServers)
	out.Roles = diffRoles(from.Roles, to.Roles)
	out.Edges = diffEdges(from.Edges, to.Edges)
	if from.Lint.Available && to.Lint.Available {
		out.Lint = &LintDiff{From: from.Lint.Summary, To: to.Lint.Summary}
	} else {
		out.Notes = append(out.Notes, "lint totals are not compared: lint did not run on one side")
	}
	out.Identical = out.Items.Empty() && out.MCPServers.Empty() && out.Roles.Empty() && out.Edges.Empty() &&
		(out.Lint == nil || out.Lint.From == out.Lint.To)
	return out, nil
}

func fillSide(side DiffSide, doc *CatalogDocV2) DiffSide {
	side.LockTree = doc.Project.LockTree
	side.Items = len(doc.Items)
	for i := range doc.Items {
		side.ListingTokens += doc.Items[i].LoadCost.ListingTokens
		side.BodyTokens += doc.Items[i].LoadCost.BodyTokens
	}
	return side
}

func newSection() DiffSection {
	return DiffSection{Added: []DiffEntry{}, Removed: []DiffEntry{}, Changed: []DiffChanged{}}
}

func diffItems(from, to []CatalogItemV2) DiffSection {
	sec := newSection()
	old := map[string]*CatalogItemV2{}
	for i := range from {
		old[from[i].Ref] = &from[i]
	}
	seen := map[string]bool{}
	for i := range to {
		it := &to[i]
		seen[it.Ref] = true
		prev, ok := old[it.Ref]
		if !ok {
			sec.Added = append(sec.Added, DiffEntry{Ref: it.Ref, Kind: it.Kind, Tokens: it.LoadCost.ListingTokens + it.LoadCost.BodyTokens})
			continue
		}
		if changes := itemChanges(prev, it); len(changes) > 0 {
			sec.Changed = append(sec.Changed, DiffChanged{Ref: it.Ref, Kind: it.Kind, Changes: changes})
		}
	}
	for i := range from {
		if !seen[from[i].Ref] {
			it := &from[i]
			sec.Removed = append(sec.Removed, DiffEntry{Ref: it.Ref, Kind: it.Kind, Tokens: it.LoadCost.ListingTokens + it.LoadCost.BodyTokens})
		}
	}
	sortSection(&sec)
	return sec
}

type changeList []DiffChange

func (c *changeList) str(field, from, to string) {
	if from != to {
		*c = append(*c, DiffChange{Field: field, From: from, To: to})
	}
}

func (c *changeList) num(field string, from, to int) {
	c.str(field, strconv.Itoa(from), strconv.Itoa(to))
}

func itemChanges(a, b *CatalogItemV2) []DiffChange {
	var c changeList
	c.str("digest", a.Digest, b.Digest)
	c.str("description", a.Description, b.Description)
	c.str("owner", a.Owner, b.Owner)
	c.str("version", a.Version, b.Version)
	c.str("path", a.Path, b.Path)
	c.str("source", a.Source.Type, b.Source.Type)
	c.str("delivery", a.Delivery, b.Delivery)
	c.str("mode", a.Mode, b.Mode)
	c.num("listing_tokens", a.LoadCost.ListingTokens, b.LoadCost.ListingTokens)
	c.num("body_tokens", a.LoadCost.BodyTokens, b.LoadCost.BodyTokens)
	c.num("resource_tokens", a.LoadCost.ResourceTokens, b.LoadCost.ResourceTokens)
	c.str("lint", lintStatus(a.Lint), lintStatus(b.Lint))
	c.str("approval", approvalStatus(a.Approval), approvalStatus(b.Approval))
	c.str("roles", joinSorted(a.Roles), joinSorted(b.Roles))
	return c
}

func lintStatus(l *ItemLint) string {
	if l == nil {
		return "n/a"
	}
	return l.Status
}

func approvalStatus(a *ItemApproval) string {
	if a == nil {
		return "not required"
	}
	return a.Status
}

func joinSorted(values []string) string {
	v := slices.Clone(values)
	sort.Strings(v)
	return strings.Join(v, ", ")
}

func diffMCP(from, to []CatalogMCPServer) DiffSection {
	sec := newSection()
	old := map[string]*CatalogMCPServer{}
	for i := range from {
		old[from[i].Ref] = &from[i]
	}
	seen := map[string]bool{}
	for i := range to {
		s := &to[i]
		seen[s.Ref] = true
		prev, ok := old[s.Ref]
		if !ok {
			sec.Added = append(sec.Added, DiffEntry{Ref: s.Ref, Kind: "mcp"})
			continue
		}
		var c changeList
		c.str("transport", prev.Transport, s.Transport)
		c.str("command", prev.CommandBasename, s.CommandBasename)
		c.str("enabled", strconv.FormatBool(prev.Enabled), strconv.FormatBool(s.Enabled))
		c.str("profiles", joinSorted(prev.Profiles), joinSorted(s.Profiles))
		c.str("pinned", pinLabel(prev.Pinned), pinLabel(s.Pinned))
		c.str("env", mcpNameList(prev.Env), mcpNameList(s.Env))
		c.str("headers", mcpNameList(prev.Headers), mcpNameList(s.Headers))
		c.str("description", prev.Description, s.Description)
		if len(c) > 0 {
			sec.Changed = append(sec.Changed, DiffChanged{Ref: s.Ref, Kind: "mcp", Changes: c})
		}
	}
	for i := range from {
		if !seen[from[i].Ref] {
			sec.Removed = append(sec.Removed, DiffEntry{Ref: from[i].Ref, Kind: "mcp"})
		}
	}
	sortSection(&sec)
	return sec
}

func pinLabel(p *bool) string {
	switch {
	case p == nil:
		return "n/a"
	case *p:
		return "pinned"
	}
	return "unpinned"
}

// mcpNameList lists env or header entries by name and whether they hold a
// literal value; the values themselves are never in a catalog.
func mcpNameList(values []MCPValue) string {
	parts := make([]string, 0, len(values))
	for _, v := range values {
		label := v.Name
		if v.Literal {
			label += " (literal)"
		} else if v.Ref != "" {
			label += " (${" + v.Ref + "})"
		}
		parts = append(parts, label)
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

func diffRoles(from, to []CatalogRole) DiffSection {
	sec := newSection()
	old := map[string]*CatalogRole{}
	for i := range from {
		old[from[i].Name] = &from[i]
	}
	seen := map[string]bool{}
	for i := range to {
		r := &to[i]
		seen[r.Name] = true
		prev, ok := old[r.Name]
		if !ok {
			sec.Added = append(sec.Added, DiffEntry{Ref: r.Name, Kind: "role", Tokens: r.Totals.Tokens})
			continue
		}
		var c changeList
		c.str("description", prev.Description, r.Description)
		c.str("extends", prev.Extends, r.Extends)
		c.str("domains", joinSorted(prev.Domains), joinSorted(r.Domains))
		c.num("items", prev.Totals.Items, r.Totals.Items)
		c.num("tokens", prev.Totals.Tokens, r.Totals.Tokens)
		c.num("served_skills", prev.Totals.Served, r.Totals.Served)
		if len(c) > 0 {
			sec.Changed = append(sec.Changed, DiffChanged{Ref: r.Name, Kind: "role", Changes: c})
		}
	}
	for i := range from {
		if !seen[from[i].Name] {
			sec.Removed = append(sec.Removed, DiffEntry{Ref: from[i].Name, Kind: "role", Tokens: from[i].Totals.Tokens})
		}
	}
	sortSection(&sec)
	return sec
}

func sortSection(s *DiffSection) {
	sort.Slice(s.Added, func(i, j int) bool { return s.Added[i].Ref < s.Added[j].Ref })
	sort.Slice(s.Removed, func(i, j int) bool { return s.Removed[i].Ref < s.Removed[j].Ref })
	sort.Slice(s.Changed, func(i, j int) bool { return s.Changed[i].Ref < s.Changed[j].Ref })
}

// ParseCatalogV2 reads a version 2 catalog document (the output of `catalog
// --format json --schema-version 2`, or the catalog.json of a site). Any other
// schema_version is refused, as the contract requires; unknown fields of a version
// 2 document are ignored so a later additive field does not break a diff.
func ParseCatalogV2(data []byte) (*CatalogDocV2, error) {
	var head struct {
		SchemaVersion int `json:"schema_version"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return nil, oops.Wrapf(err, "not a catalog document")
	}
	if head.SchemaVersion != CatalogSchemaVersionV2 {
		return nil, oops.Hint("write one with `ai-rulez catalog --format json --schema-version 2`").
			Errorf("catalog schema_version %d is not supported (need %d)", head.SchemaVersion, CatalogSchemaVersionV2)
	}
	var doc CatalogDocV2
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&doc); err != nil {
		return nil, oops.Wrapf(err, "parse catalog document")
	}
	return &doc, nil
}

func diffEdges(from, to []CatalogEdge) DiffSection {
	sec := newSection()
	label := func(e CatalogEdge) string { return e.From + " -> " + e.To }
	old := map[string]bool{}
	for _, e := range from {
		old[label(e)] = true
	}
	now := map[string]bool{}
	for _, e := range to {
		now[label(e)] = true
		if !old[label(e)] {
			sec.Added = append(sec.Added, DiffEntry{Ref: label(e), Kind: e.Kind})
		}
	}
	for _, e := range from {
		if !now[label(e)] {
			sec.Removed = append(sec.Removed, DiffEntry{Ref: label(e), Kind: e.Kind})
		}
	}
	sortSection(&sec)
	return sec
}
