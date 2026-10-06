package mcp

import (
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/skillsearch"
)

// find_skill ranks with the shared lexical ranker in internal/skillsearch
// (BM25F over name, triggers, keywords and description).

// FindHit is one ranked find_skill result.
type FindHit struct {
	Skill *CatalogSkill
	Score float64
}

// bm25Rank scores skills against the query. Skills with no matching term are
// omitted. An empty query (or one that is only stopwords) returns nothing: use
// list_skill_resources or skills/list to enumerate.
func bm25Rank(skills []*CatalogSkill, query string) []FindHit {
	docs, _ := searchDocs(skills)
	ranked := skillsearch.Rank(docs, query)
	if len(ranked) == 0 {
		return nil
	}
	hits := make([]FindHit, len(ranked))
	for i, h := range ranked {
		hits[i] = FindHit{Skill: skills[h.Index], Score: h.Score}
	}
	return hits
}

// searchDocs is the searchable text of the skills and their stable ids (the
// catalog name), in catalog order.
func searchDocs(skills []*CatalogSkill) (docs []skillsearch.Doc, ids []string) {
	docs = make([]skillsearch.Doc, len(skills))
	ids = make([]string, len(skills))
	for i, s := range skills {
		docs[i] = skillsearch.Doc{Name: s.Name, Description: s.Description, Triggers: s.Triggers, Keywords: s.Keywords}
		ids[i] = s.Name
	}
	return docs, ids
}

// Rank ranks the served skills against a query exactly as find_skill does, for
// `ai-rulez search`.
func (c *Catalog) Rank(query string) []FindHit { return bm25Rank(c.skills, query) }

// SearchDocs returns the catalog's skills as search documents with their ids,
// for `ai-rulez search --eval`.
func (c *Catalog) SearchDocs() (docs []skillsearch.Doc, ids []string) { return searchDocs(c.skills) }

// listField reads a frontmatter value that is a list of strings or one
// comma-separated string.
func listField(front map[string]any, key string) []string {
	var out []string
	switch v := front[key].(type) {
	case []any:
		for _, e := range v {
			if s, ok := e.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
	case string:
		for _, p := range strings.Split(v, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}
