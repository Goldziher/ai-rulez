package govview

import (
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// EdgeUses is the edge kind of an item that names a skill in its `skills:`
// frontmatter. Roles keep items; that relation is items[].roles, not an edge.
const EdgeUses = "uses"

// CatalogEdge is one dependency between items: From names the skill To in its
// `skills:` frontmatter. Both are item refs.
type CatalogEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

// buildEdges resolves the `skills:` frontmatter of every item into edges. A name
// resolves to the skill of the same domain, else to the root skill, else to the
// only skill of that name; a name that resolves to nothing or to several skills
// is left out and said in the notes, never guessed. files[i] is the source of
// items[i] (nil when unknown).
func buildEdges(items []CatalogItemV2, files []*config.ContentFile) (edges []CatalogEdge, notes []string) {
	edges = []CatalogEdge{}
	byID := map[string][]int{}
	for i := range items {
		if items[i].Kind == config.RoleKindSkill {
			byID[items[i].ID] = append(byID[items[i].ID], i)
		}
	}
	seen := map[CatalogEdge]bool{}
	noted := map[string]bool{}
	note := func(msg string) {
		if !noted[msg] {
			noted[msg] = true
			notes = append(notes, msg)
		}
	}
	for i := range items {
		cf := files[i]
		if cf == nil || cf.Metadata == nil {
			continue
		}
		for _, dep := range cf.Metadata.Skills {
			dep = strings.TrimSpace(dep)
			if dep == "" {
				continue
			}
			target, ok := resolveSkill(items, byID[dep], items[i].Domain)
			switch {
			case len(byID[dep]) == 0:
				note(items[i].Ref + " names skill " + dep + ", which is not in the catalog: the dependency is not drawn")
				continue
			case !ok:
				note(items[i].Ref + " names skill " + dep + ", which exists in several domains: the dependency is not drawn")
				continue
			case target == i:
				continue
			}
			edge := CatalogEdge{From: items[i].Ref, To: items[target].Ref, Kind: EdgeUses}
			if !seen[edge] {
				seen[edge] = true
				edges = append(edges, edge)
			}
		}
	}
	sort.Slice(edges, func(a, b int) bool {
		if edges[a].From != edges[b].From {
			return edges[a].From < edges[b].From
		}
		return edges[a].To < edges[b].To
	})
	sort.Strings(notes)
	return edges, notes
}

// resolveSkill picks the skill a name refers to from the candidates.
func resolveSkill(items []CatalogItemV2, candidates []int, domain string) (int, bool) {
	if len(candidates) == 1 {
		return candidates[0], true
	}
	for _, wantDomain := range []string{domain, ""} {
		var hit []int
		for _, c := range candidates {
			if items[c].Domain == wantDomain {
				hit = append(hit, c)
			}
		}
		if len(hit) == 1 {
			return hit[0], true
		}
	}
	return 0, false
}
