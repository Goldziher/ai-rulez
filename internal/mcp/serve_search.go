package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/Goldziher/ai-rulez/v5/internal/skillsearch"
	"github.com/Goldziher/ai-rulez/v5/internal/skillsearch/setup"
)

// SearchRuntime ranks find_skill queries with the configured mode: lexical (the
// default, no network, unchanged ranking), hybrid or vector over the embedding
// index. It never builds the index and never embeds a document: it loads the
// index `ai-rulez search index` wrote and embeds only the query. Any failure
// ends in the lexical ranking with a `degraded` reason, never an error.
type SearchRuntime struct {
	get func() *config.Config

	mu       sync.Mutex
	cfg      *config.Config
	resolved *setup.Resolved
	resErr   error
	embedder skillsearch.Embedder
	release  func()
	embErr   error
	cat      *Catalog
	stamp    string
	ranker   *skillsearch.Ranker
	warned   map[string]bool
	// newEmbedder builds the embedder; tests replace it.
	newEmbedder func(*setup.Resolved) (skillsearch.Embedder, func(), error)
}

// NewSearchRuntime returns a runtime reading the configuration of the latest build.
func NewSearchRuntime(get func() *config.Config) *SearchRuntime {
	return &SearchRuntime{get: get, warned: map[string]bool{}, newEmbedder: func(r *setup.Resolved) (skillsearch.Embedder, func(), error) { return r.Embedder() }}
}

// Close releases the embedding client.
func (rt *SearchRuntime) Close() {
	if rt == nil {
		return
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.release != nil {
		rt.release()
		rt.release = nil
	}
}

// ranked is one find_skill ranking.
type ranked struct {
	hits     []FindHit
	ranking  string
	degraded string
	log      *skillsearch.QueryLog
}

// rank ranks the catalog's skills against a task.
func (rt *SearchRuntime) rank(ctx context.Context, cat *Catalog, task string) ranked {
	lexical := ranked{hits: bm25Rank(cat.Skills(), task), ranking: skillsearch.ModeLexical}
	if rt == nil || rt.get == nil {
		return lexical
	}
	rt.mu.Lock()
	cfg := rt.get()
	if cfg != rt.cfg || rt.resolved == nil && rt.resErr == nil {
		rt.cfg = cfg
		rt.resolved, rt.resErr = setup.Resolve(cfg, setup.Options{})
		rt.dropClient()
		rt.warnNotes()
	}
	resolved, resErr := rt.resolved, rt.resErr
	if resErr != nil {
		rt.warnOnce("config", resErr.Error())
		rt.mu.Unlock()
		return lexical
	}
	lexical.log = resolved.QueryLog(lint.DetectSecret)
	if !resolved.Wants() {
		rt.mu.Unlock()
		return lexical
	}
	r := rt.rankerFor(cat, resolved)
	rt.mu.Unlock()
	if r == nil {
		lexical.degraded = skillsearch.DegradedProvider
		return lexical
	}
	res := r.Search(ctx, task)
	if res.Degraded != "" {
		rt.mu.Lock()
		rt.warnOnce(res.Degraded, skillsearch.DegradedMessage(res.Degraded))
		rt.mu.Unlock()
	}
	out := ranked{ranking: res.Ranking, degraded: res.Degraded, log: lexical.log}
	for _, h := range res.Hits {
		out.hits = append(out.hits, FindHit{Skill: cat.Skills()[h.Index], Score: h.Score, LexRank: h.LexRank, VecRank: h.VecRank, StaleVector: h.StaleVec})
	}
	return out
}

// rankerFor returns the ranker of this catalog and index state, rebuilding it
// when the catalog, the config or the index files changed. Caller holds rt.mu.
func (rt *SearchRuntime) rankerFor(cat *Catalog, res *setup.Resolved) *skillsearch.Ranker {
	dir, err := res.IndexPath()
	if err != nil {
		rt.warnOnce("index-dir", err.Error())
		return nil
	}
	stamp := indexStamp(dir)
	if rt.ranker != nil && rt.cat == cat && rt.stamp == stamp {
		return rt.ranker
	}
	items := catalogItems(cat.Skills())
	r := &skillsearch.Ranker{Items: items, Cfg: res.Search, Timeout: msDuration(res.Search.QueryTimeoutMS)}
	if rt.embedder == nil && rt.embErr == nil {
		rt.embedder, rt.release, rt.embErr = rt.newEmbedder(res)
		if rt.embErr != nil {
			rt.warnOnce("embedder", rt.embErr.Error())
		}
	}
	r.Embedder = rt.embedder
	idx, err := skillsearch.LoadIndex(dir)
	switch {
	case err == nil && rt.embedder != nil:
		// Vectors of another model or text template are never mixed with this query's embedding.
		if reason := idx.Manifest.Reason(rt.embedder.Fingerprint(), rt.embedder.Model(), res.Search); reason != "" {
			rt.warnOnce("index-stale", "search: the index is not usable ("+reason+"); run 'ai-rulez search index --rebuild'")
		} else {
			r.Index = idx
		}
	case err == nil:
		r.Index = idx
	case !isNoIndex(err):
		r.IndexErr = err
		rt.warnOnce("index-load", "search: cannot read the index: "+err.Error())
	}
	rt.ranker, rt.cat, rt.stamp = r, cat, stamp
	return r
}

// warnNotes reports what Resolve ignored (a repository command, a repository log_queries), once per resolution.
// Caller holds rt.mu.
func (rt *SearchRuntime) warnNotes() {
	if rt.resolved == nil {
		return
	}
	for _, note := range rt.resolved.Notes {
		rt.warnOnce("note:"+note, note)
	}
}

func (rt *SearchRuntime) dropClient() {
	if rt.release != nil {
		rt.release()
	}
	rt.embedder, rt.release, rt.embErr, rt.ranker = nil, nil, nil, nil
}

func (rt *SearchRuntime) warnOnce(key, msg string) {
	if rt.warned[key] {
		return
	}
	rt.warned[key] = true
	logger.Warn(msg)
}

// indexStamp changes whenever the index files do, so a rebuilt index is picked up without a restart.
func indexStamp(dir string) string {
	var out string
	for _, name := range []string{skillsearch.ManifestFile, skillsearch.VectorsFile} {
		if info, err := os.Stat(filepath.Join(dir, name)); err == nil {
			out += fmt.Sprintf("%d/%d;", info.ModTime().UnixNano(), info.Size())
		}
	}
	return out
}

func catalogItems(skills []*CatalogSkill) []skillsearch.Item {
	items := make([]skillsearch.Item, len(skills))
	for i, s := range skills {
		items[i] = skillsearch.Item{
			ID: s.Name, Domain: s.Domain, Digest: s.LockDigest,
			Doc: skillsearch.Doc{Name: s.Name, Description: s.Description, Triggers: s.Triggers, Keywords: s.Keywords},
		}
		if len(s.Files) > 0 && s.Files[0].RelPath == skillMarkdown {
			_, items[i].Body = skillsearch.SplitSkill(s.Files[0].Content)
		}
	}
	return items
}

// CatalogItems returns the catalog's skills as search items (with the bodies
// index_body embeds), for `ai-rulez search`.
func (c *Catalog) CatalogItems() []skillsearch.Item { return catalogItems(c.skills) }

// SkillAt returns the i-th skill of the catalog, the order CatalogItems uses.
func (c *Catalog) SkillAt(i int) *CatalogSkill { return c.skills[i] }

// RoleScope resolves a role of the project's [[roles]] the way find_skill does.
func (s *Server) RoleScope(role string) (RoleScope, bool) { return s.serve.scope(role) }

// Log returns the query log when the project enabled [search] log_queries, else nil.
func (rt *SearchRuntime) Log() *skillsearch.QueryLog {
	if rt == nil || rt.get == nil {
		return nil
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	cfg := rt.get()
	if cfg != rt.cfg || rt.resolved == nil && rt.resErr == nil {
		rt.cfg = cfg
		rt.resolved, rt.resErr = setup.Resolve(cfg, setup.Options{})
		rt.dropClient()
		rt.warnNotes()
	}
	if rt.resErr != nil || rt.resolved == nil {
		return nil
	}
	return rt.resolved.QueryLog(lint.DetectSecret)
}
