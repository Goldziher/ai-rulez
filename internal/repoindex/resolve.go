package repoindex

import (
	"strings"

	"github.com/samber/oops"
)

// Resolved is an item that applies to a path, with the scope that shadows it
// when a nearer scope has an item of the same kind and name.
type Resolved struct {
	Item
	ShadowedBy string `json:"shadowed_by,omitempty"`
}

// Resolution is what applies to a path: the scopes that are the path or one of
// its ancestors, nearest first, and the items of those scopes, nearest first.
type Resolution struct {
	Path   string     `json:"path"`
	Scopes []string   `json:"scopes"`
	Items  []Resolved `json:"items"`
}

// Resolve returns what applies to a project-relative path, as a harness working
// there would load it. The path is cleaned and must stay inside the project; it
// need not exist. A skill, agent or command is shadowed by one of the same name
// in a nearer scope (harnesses shadow those by name); instruction files and
// rules add up, so none of them is shadowed.
func (ix *Index) Resolve(p string) (Resolution, error) {
	clean, err := CleanPath(p)
	if err != nil {
		return Resolution{}, err
	}
	out := Resolution{Path: clean, Scopes: []string{}, Items: []Resolved{}}
	owner := map[string]string{}
	for cur := clean; ; cur = parentOf(cur) {
		if _, ok := ix.scopeByKey[cur]; ok {
			out.Scopes = append(out.Scopes, cur)
			out.Items = append(out.Items, ix.resolveScope(cur, owner)...)
		}
		if cur == RootScope {
			break
		}
	}
	return out, nil
}

// resolveScope lists the items of one scope, marking those a nearer scope
// shadows. owner holds the nearest scope that claimed each shadowing name so far.
func (ix *Index) resolveScope(scope string, owner map[string]string) []Resolved {
	items := ix.byScope[scope]
	out := make([]Resolved, 0, len(items))
	for i := range items {
		it := &items[i]
		r := Resolved{Item: *it}
		if shadows(it.Kind) {
			key := string(it.Kind) + "\x00" + it.Name
			if first, seen := owner[key]; !seen {
				owner[key] = scope
			} else if first != scope {
				r.ShadowedBy = first
			}
		}
		out = append(out, r)
	}
	return out
}

// shadows reports whether a kind is resolved by name, the nearest scope winning.
func shadows(k Kind) bool { return k == KindSkill || k == KindAgent || k == KindCommand }

// Within lists the items of a scope, and of its descendants when recursive. The
// scope must exist.
func (ix *Index) Within(scope string, recursive bool) ([]Item, error) {
	clean, err := CleanPath(scope)
	if err != nil {
		return nil, err
	}
	if _, ok := ix.scopeByKey[clean]; !ok {
		return nil, oops.Errorf("no scope %q in the repository index", clean)
	}
	var out []Item
	for i := range ix.items {
		if it := &ix.items[i]; it.Scope == clean || recursive && (clean == RootScope || strings.HasPrefix(it.Scope, clean+"/")) {
			out = append(out, *it)
		}
	}
	sortItems(out)
	return out, nil
}
