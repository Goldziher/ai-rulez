package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"path/filepath"

	"github.com/Goldziher/ai-rulez/v5/internal/mcp/handlers"
	"github.com/Goldziher/ai-rulez/v5/internal/repoindex"
)

// Scopes: the repository index behind skill discovery knows where each item
// lives, so a planner can ask what applies to one path (find_skill path, the
// ai-rulez://catalog scopes section) instead of getting one flat list.

// errNeedsDiscovery is what a path or scope query answers when the server did
// not index the repository.
var errNeedsDiscovery = errors.New("path and scope need repository discovery: start the server with --discover-skills or set [mcp] discover_skills")

const (
	argPath      = "path"
	argScope     = "scope"
	argRecursive = "recursive"
)

// scopeQuery narrows a listing to what applies to a path or to one scope.
type scopeQuery struct {
	// Path resolves to the scopes that are the path or an ancestor, nearest first.
	Path string
	// Scope is one exact scope; Recursive adds its descendants.
	Scope     string
	Recursive bool
}

func (q scopeQuery) active() bool { return q.Path != "" || q.Scope != "" }

// newScopeQuery validates the untrusted arguments of a scope query: exactly
// one of path and scope, both relative to the project and inside it.
func newScopeQuery(path, scope string, recursive bool) (scopeQuery, error) {
	var q scopeQuery
	switch {
	case path != "" && scope != "":
		return q, fmt.Errorf("pass either path or scope, not both")
	case recursive && scope == "":
		return q, fmt.Errorf("recursive adds the descendants of a scope: pass scope with it")
	}
	var err error
	if path != "" {
		if q.Path, err = repoindex.CleanPath(path); err != nil {
			return scopeQuery{}, fmt.Errorf("path: %w", err)
		}
	}
	if scope != "" {
		if q.Scope, err = repoindex.CleanPath(scope); err != nil {
			return scopeQuery{}, fmt.Errorf("scope: %w", err)
		}
		q.Recursive = recursive
	}
	return q, nil
}

func scopeQueryOf(req *handlers.ToolRequest) (scopeQuery, error) {
	return newScopeQuery(req.GetString(argPath, ""), req.GetString(argScope, ""), req.GetBool(argRecursive, false))
}

// scopeQueryFromURI reads the query string of an ai-rulez://catalog URI.
func scopeQueryFromURI(raw string) (scopeQuery, error) {
	values, err := url.ParseQuery(raw)
	if err != nil {
		return scopeQuery{}, fmt.Errorf("catalog query: %w", err)
	}
	for key := range values {
		if key != argPath && key != argScope && key != argRecursive {
			return scopeQuery{}, fmt.Errorf("catalog query: unknown parameter %q (use path, scope or recursive)", key)
		}
	}
	recursive := false
	switch values.Get(argRecursive) {
	case "", "false":
	case "true":
		recursive = true
	default:
		return scopeQuery{}, fmt.Errorf("catalog query: recursive must be true or false")
	}
	return newScopeQuery(values.Get(argPath), values.Get(argScope), recursive)
}

// repoView is what a catalog knows about the repository it discovered from: the
// index and the directory each discovered skill was served from.
type repoView struct {
	index *repoindex.Index
	// skillDir maps a served skill name to its directory in the repository, and
	// skillScope to the scope that holds the directory.
	skillDir, skillScope map[string]string
	// root is the project root; the index's paths are relative to it.
	root string
	// adm is the scan setup an item of context goes through, again at load time.
	adm Admission
	// generated tells a file ai-rulez wrote from one a person did.
	generated func(path string, content []byte) bool
	// refused maps a scope and name to why the scan refused the item at start-up.
	refused map[string]string
	// held names the skills withheld from the model (disable-model-invocation);
	// the scope counts leave them out.
	held map[string]bool
}

// counts is the per-kind count of a scope without the skills held back from the model.
func (v *repoView) counts(s *repoindex.Scope) map[repoindex.Kind]int {
	out := maps.Clone(s.Counts)
	for name := range v.held {
		if v.skillScope[name] == s.Path && out[repoindex.KindSkill] > 0 {
			out[repoindex.KindSkill]--
		}
	}
	return out
}

func refusedKey(scope, name string) string { return scope + "\x00" + name }

func joinRoot(root, rel string) string { return filepath.Join(root, filepath.FromSlash(rel)) }

// repository returns the repository view of the catalog; nil when discovery did
// not index (and for the authoring server, which has no catalog).
func (c *Catalog) repository() *repoView {
	if c == nil {
		return nil
	}
	return c.repo
}

// view returns the repository view of a discovery run; nil when it did not index.
func (d *discovery) view() *repoView {
	if d == nil || d.index == nil {
		return nil
	}
	dirs := make(map[string]string, len(d.report.Served))
	scopes := make(map[string]string, len(d.report.Served))
	for _, s := range d.report.Served {
		dirs[s.Name], scopes[s.Name] = s.Path, s.Scope
	}
	v := &repoView{
		index: d.index, skillDir: dirs, skillScope: scopes, root: d.cfg.BaseDir,
		adm: d.admission(), generated: d.isGenerated, refused: map[string]string{},
	}
	refusals := d.index.Refusals()
	for i := range refusals {
		v.refused[refusedKey(refusals[i].Item.Scope, refusals[i].Item.Name)] = refusals[i].Reason
	}
	return v
}

// skillSelection says which served skills a query keeps.
type skillSelection struct {
	keep func(name string) bool
	// scopes are the resolved scopes of a path query, nearest first.
	scopes []string
}

// selectSkills builds the test a served skill must pass for the query. A skill
// that came from the repository passes when its directory is an effective item
// of the selection (a path query drops one a nearer scope shadows, and one
// whose `paths:` do not reach the path); a skill
// ai-rulez defines or fetches has no scope and applies everywhere, so it passes
// a path query and the root scope.
func (v *repoView) selectSkills(q scopeQuery) (skillSelection, error) {
	dirs := map[string]bool{}
	global := true
	var scopes []string
	if q.Path != "" {
		res, err := v.index.Resolve(q.Path)
		if err != nil {
			return skillSelection{}, err //nolint:wrapcheck // names the path
		}
		scopes = res.Scopes
		asDir := v.isDir(q.Path)
		for i := range res.Items {
			if r := &res.Items[i]; r.Kind == repoindex.KindSkill && r.ShadowedBy == "" && appliesTo(&r.Item, q.Path, asDir) {
				dirs[r.Path] = true
			}
		}
	} else {
		items, err := v.index.Within(q.Scope, q.Recursive)
		if err != nil {
			return skillSelection{}, err //nolint:wrapcheck // names the scope
		}
		global = q.Scope == repoindex.RootScope
		for _, it := range items {
			if it.Kind == repoindex.KindSkill {
				dirs[it.Path] = true
			}
		}
	}
	return skillSelection{scopes: scopes, keep: func(name string) bool {
		dir, discovered := v.skillDir[name]
		if !discovered {
			return global
		}
		return dirs[dir]
	}}, nil
}

// scopeOf is the scope of a discovered skill, "" for any other.
func (v *repoView) scopeOf(name string) string {
	if v == nil {
		return ""
	}
	return v.skillScope[name]
}

// filterHits keeps the ranked skills a selection keeps.
func filterHits(hits []FindHit, keep func(string) bool) []FindHit {
	out := make([]FindHit, 0, len(hits))
	for _, h := range hits {
		if keep(h.Skill.Name) {
			out = append(out, h)
		}
	}
	return out
}

// scopeEntry is one scope of the catalog's scopes section.
type scopeEntry struct {
	Scope   string                 `json:"scope"`
	Sources []string               `json:"sources"`
	Counts  map[repoindex.Kind]int `json:"counts"`
}

// scopeEntries lists the scopes a query selects: all of them, the scopes a path
// resolves to (nearest first), or one scope with its descendants when asked.
func (v *repoView) scopeEntries(q scopeQuery) ([]scopeEntry, error) {
	var paths []string
	switch {
	case q.Path != "":
		res, err := v.index.Resolve(q.Path)
		if err != nil {
			return nil, err //nolint:wrapcheck // names the path
		}
		paths = res.Scopes
	case q.Scope != "":
		if _, ok := v.index.Scope(q.Scope); !ok {
			return nil, fmt.Errorf("no scope %q in the repository index", q.Scope)
		}
		for _, s := range v.index.Scopes() {
			if s.Path == q.Scope || q.Recursive && (q.Scope == repoindex.RootScope || hasScopePrefix(s.Path, q.Scope)) {
				paths = append(paths, s.Path)
			}
		}
	default:
		for _, s := range v.index.Scopes() {
			paths = append(paths, s.Path)
		}
	}
	out := make([]scopeEntry, 0, len(paths))
	for _, p := range paths {
		s, _ := v.index.Scope(p) //nolint:errcheck // every path came from the index
		out = append(out, scopeEntry{Scope: s.Path, Sources: s.Sources, Counts: v.counts(&s)})
	}
	return out, nil
}

func hasScopePrefix(scope, parent string) bool {
	return len(scope) > len(parent) && scope[:len(parent)] == parent && scope[len(parent)] == '/'
}

// IndexReport sizes the repository index; the catalog resource carries it under
// skill_discovery.index.
type IndexReport struct {
	// Items is how many items the index holds.
	Items int `json:"items"`
	// Limit is the index size cap; past it the shallowest scopes are kept.
	Limit     int  `json:"limit"`
	Truncated bool `json:"truncated"`
	// Found is how many items the repository holds before the cap.
	Found int `json:"found"`
	// Cache says how the build used the index cache; nil when the build made no
	// use of one.
	Cache *IndexCacheReport `json:"cache,omitempty"`
}

// withScopes adds the scopes section to the JSON document of the catalog
// resource when discovery indexed the repository. Text that is not a JSON
// object is returned unchanged.
func withScopes(text string, v *repoView, q scopeQuery) (string, error) {
	if v == nil {
		if q.active() {
			return "", errNeedsDiscovery
		}
		return text, nil
	}
	entries, err := v.scopeEntries(q)
	if err != nil {
		return "", err
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(text), &doc); err != nil || doc == nil {
		return text, nil //nolint:nilerr // not a JSON object: nothing to add to
	}
	doc["scopes"] = entries
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return text, nil //nolint:nilerr // keep the document as it was
	}
	return string(out), nil
}
