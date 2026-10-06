package skillsearch

import (
	"container/list"
	"context"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
)

const queryCacheSize = 256

// SearchHit is one ranked result. Index points into Ranker.Items. LexRank and
// VecRank are the 1-based ranks in the two candidate lists, 0 when the item is
// not in that list. Score is the fused score (the BM25F score in lexical mode,
// the cosine in vector mode).
type SearchHit struct {
	Index    int
	Score    float64
	LexRank  int
	VecRank  int
	VecSim   float64
	StaleVec bool
}

// SearchResult is the answer to one query. Ranking is what produced Hits; it
// differs from the requested mode when Degraded says why.
type SearchResult struct {
	Ranking  string
	Degraded string
	Hits     []SearchHit
	// Embedded reports a query embedding call was made (not served from the in-memory LRU).
	Embedded bool
	// TopSim is the best cosine of any skill in scope to the query (0 without a vector list), before
	// VectorMinSim is applied: it is what `search --eval` calibrates the threshold from.
	TopSim float64
	// Abstained is true when VectorMinSim left no skill: the query matches nothing confidently.
	Abstained bool
	Tokens    int
	CostUSD   float64
	Elapsed   time.Duration
}

// Ranker answers queries over a fixed item list with the configured mode. It is
// safe for concurrent use. Index and Embedder may be nil: the ranking is then
// lexical, and Degraded says why when a vector mode was asked for.
type Ranker struct {
	Items    []Item
	Cfg      Config
	Index    *Index
	Embedder Embedder
	// Timeout bounds the query embedding; 0 means no limit beyond the provider's.
	Timeout time.Duration
	// Clock times a search (SearchResult.Elapsed); nil is the wall clock.
	Clock ambient.Clock

	once  sync.Once
	rows  []int // item index -> index row, -1 when none or stale
	stale []bool

	mu    sync.Mutex
	lru   *list.List
	cache map[string]*list.Element
}

type cachedQuery struct {
	query string
	vec   []float32
}

func (r *Ranker) prepare() {
	r.once.Do(func() {
		r.rows = make([]int, len(r.Items))
		r.stale = make([]bool, len(r.Items))
		for i := range r.Items {
			r.rows[i] = -1
			if r.Index == nil {
				continue
			}
			row := r.Index.Row(r.Items[i].Domain, r.Items[i].ID)
			if row < 0 {
				continue
			}
			if r.Index.Manifest.Items[row].TextDigest != TextDigest(EmbedText(&r.Items[i], r.Cfg)) {
				r.stale[i] = true
				continue
			}
			r.rows[i] = row
		}
		r.lru, r.cache = list.New(), map[string]*list.Element{}
	})
}

// Search ranks every item.
func (r *Ranker) Search(ctx context.Context, query string) SearchResult {
	return r.SearchScoped(ctx, query, nil)
}

// SearchScoped ranks the items allow admits (nil admits all); the lexical corpus
// statistics are those of the admitted items, as for a catalog built with --role.
func (r *Ranker) SearchScoped(ctx context.Context, query string, allow func(item int) bool) SearchResult {
	return r.SearchMode(ctx, r.Cfg.Resolved().Mode, query, allow)
}

// SearchMode is SearchScoped with an explicit mode (lexical, hybrid or vector),
// so one ranker, with one query cache, can serve every mode of an evaluation.
func (r *Ranker) SearchMode(ctx context.Context, mode, query string, allow func(item int) bool) SearchResult {
	start := r.Clock.Now()
	r.prepare()
	cfg := r.Cfg.Resolved()
	cfg.Mode = mode
	pool := make([]int, 0, len(r.Items)) // item indexes, in order
	for i := range r.Items {
		if allow == nil || allow(i) {
			pool = append(pool, i)
		}
	}
	lex := r.lexical(pool, query)
	res := SearchResult{Ranking: ModeLexical}
	if cfg.Mode == ModeLexical || cfg.Mode == "" {
		res.Hits = lexHits(lex)
		res.Elapsed = r.Clock.Now().Sub(start)
		return res
	}
	vec, reason, emb := r.vector(ctx, pool, query)
	res.Embedded, res.Tokens, res.CostUSD = emb.Calls > 0, emb.Tokens, emb.Cost
	res.TopSim = emb.TopSim
	if reason != "" {
		res.Degraded = reason
		res.Hits = lexHits(lex)
		r.markStale(res.Hits)
		res.Elapsed = r.Clock.Now().Sub(start)
		return res
	}
	res.Ranking = cfg.Mode
	switch {
	case cfg.Mode == ModeVector:
		res.Hits = r.withoutVectors(r.fuse(nil, vec, cfg, query, true), lex)
	case cfg.Fusion == FusionAuto && emb.Covered:
		// every skill in scope has a current vector: rank by cosine, keeping the exact-id pin
		res.Ranking = ModeVector
		res.Hits = r.pinFirst(r.fuse(nil, vec, cfg, query, true), query)
	default:
		res.Hits = r.fuse(lex, vec, cfg, query, false)
	}
	r.markStale(res.Hits)
	res.Abstained = cfg.VectorMinSim > 0 && emb.Dropped > 0 && len(res.Hits) == 0
	res.Elapsed = r.Clock.Now().Sub(start)
	return res
}

// pinFirst moves the skill whose id the query names to the front.
func (r *Ranker) pinFirst(hits []SearchHit, query string) []SearchHit {
	in := make(map[int]*SearchHit, len(hits))
	for i := range hits {
		in[hits[i].Index] = &hits[i]
	}
	pin := r.exactPin(query, in)
	if pin < 0 {
		return hits
	}
	for i := range hits {
		if hits[i].Index == pin {
			pinned := hits[i]
			copy(hits[1:i+1], hits[:i])
			hits[0] = pinned
			break
		}
	}
	return hits
}

// withoutVectors appends, in lexical order, the skills of a vector ranking that have no usable vector
// (new or changed since the index was built), so a vector search does not silently drop them.
func (r *Ranker) withoutVectors(hits []SearchHit, lex []lexHit) []SearchHit {
	have := make(map[int]bool, len(hits))
	for _, h := range hits {
		have[h.Index] = true
	}
	for i, h := range lex {
		if !have[h.item] && r.rows[h.item] < 0 {
			hits = append(hits, SearchHit{Index: h.item, LexRank: i + 1})
		}
	}
	return hits
}

func (r *Ranker) markStale(hits []SearchHit) {
	for i := range hits {
		hits[i].StaleVec = r.stale[hits[i].Index]
	}
}

type lexHit struct {
	item  int
	score float64
}

func (r *Ranker) lexical(pool []int, query string) []lexHit {
	docs := make([]Doc, len(pool))
	for i, it := range pool {
		docs[i] = r.Items[it].Doc
	}
	ranked := Rank(docs, query)
	out := make([]lexHit, len(ranked))
	for i, h := range ranked {
		out[i] = lexHit{pool[h.Index], h.Score}
	}
	return out
}

func lexHits(l []lexHit) []SearchHit {
	out := make([]SearchHit, len(l))
	for i, h := range l {
		out[i] = SearchHit{Index: h.item, Score: h.score, LexRank: i + 1}
	}
	return out
}

type vecHit struct {
	item int
	sim  float64
}

type embedUse struct {
	Calls  int
	Tokens int
	Cost   float64
	// TopSim is the best cosine before the threshold; Dropped how many skills VectorMinSim removed;
	// Covered whether every skill in scope has a current vector.
	TopSim  float64
	Dropped int
	Covered bool
}

// vector ranks the pool by cosine similarity, or says why it cannot.
func (r *Ranker) vector(ctx context.Context, pool []int, query string) ([]vecHit, string, embedUse) {
	var use embedUse
	if r.Index == nil || r.Index.Len() == 0 {
		return nil, DegradedNoIndex, use
	}
	if r.Embedder == nil {
		return nil, DegradedProvider, use
	}
	inPool := make(map[int]bool, len(pool))
	for _, it := range pool {
		inPool[it] = true
	}
	rowItem := map[int]int{}
	for it := range r.Items {
		if r.rows[it] >= 0 && inPool[it] {
			rowItem[r.rows[it]] = it
		}
	}
	if len(rowItem) == 0 && len(pool) > 0 {
		// Every skill in scope is new or changed since the index was built: nothing to rank by vector.
		return nil, DegradedNoIndex, use
	}
	q, used, err := r.queryVector(ctx, query)
	use = used
	if err != nil {
		return nil, DegradedReason(err), use
	}
	if len(q) != r.Index.Manifest.Dims {
		return nil, DegradedProvider, use
	}
	top := r.Index.TopK(q, len(rowItem), func(row int) bool { _, ok := rowItem[row]; return ok })
	out := make([]vecHit, len(top))
	for i, h := range top {
		out[i] = vecHit{rowItem[h.Row], h.Sim}
	}
	// Equal similarities order by name, so the result does not depend on index order.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].sim != out[j].sim {
			return out[i].sim > out[j].sim
		}
		return r.Items[out[i].item].ID < r.Items[out[j].item].ID
	})
	use.Covered = len(out) == len(pool)
	if len(out) > 0 {
		use.TopSim = math.Round(out[0].sim*1e6) / 1e6
	}
	if floor := r.Cfg.VectorMinSim; floor > 0 {
		kept := out[:0:0]
		for _, h := range out {
			if h.sim >= floor {
				kept = append(kept, h)
			}
		}
		use.Dropped = len(out) - len(kept)
		out = kept
	}
	return out, "", use
}

func (r *Ranker) queryVector(ctx context.Context, query string) ([]float32, embedUse, error) {
	query = capQuery(query)
	if v := r.cached(query); v != nil {
		return v, embedUse{}, nil
	}
	if r.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.Timeout)
		defer cancel()
	}
	emb, err := r.Embedder.Embed(ctx, []string{query})
	use := embedUse{Calls: 1, Tokens: emb.Tokens, Cost: emb.CostUSD}
	if err != nil {
		return nil, use, err
	}
	if len(emb.Vectors) != 1 {
		return nil, use, errEmbedCount
	}
	v := append([]float32(nil), emb.Vectors[0]...)
	if err := finite(v); err != nil {
		return nil, use, err
	}
	normalize(v)
	r.store(query, v)
	return v, use, nil
}

func (r *Ranker) cached(q string) []float32 {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.cache[q]; ok {
		r.lru.MoveToFront(e)
		return e.Value.(*cachedQuery).vec
	}
	return nil
}

func (r *Ranker) store(q string, v []float32) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.cache[q]; ok {
		r.lru.MoveToFront(e)
		return
	}
	r.cache[q] = r.lru.PushFront(&cachedQuery{q, v})
	for r.lru.Len() > queryCacheSize {
		old := r.lru.Back()
		r.lru.Remove(old)
		delete(r.cache, old.Value.(*cachedQuery).query)
	}
}

// fuse combines the two candidate lists. With vectorOnly the lexical list is
// ignored and the cosine is the score.
func (r *Ranker) fuse(lex []lexHit, vec []vecHit, cfg Config, query string, vectorOnly bool) []SearchHit {
	cand := cfg.Candidates
	if len(lex) > cand {
		lex = lex[:cand]
	}
	if len(vec) > cand {
		vec = vec[:cand]
	}
	hits := map[int]*SearchHit{}
	get := func(item int) *SearchHit {
		h, ok := hits[item]
		if !ok {
			h = &SearchHit{Index: item}
			hits[item] = h
		}
		return h
	}
	for i, h := range vec {
		e := get(h.item)
		e.VecRank, e.VecSim = i+1, math.Round(h.sim*1e6)/1e6
	}
	if !vectorOnly {
		for i, h := range lex {
			get(h.item).LexRank = i + 1
		}
	}
	score := map[int]float64{}
	switch {
	case vectorOnly:
		for item, h := range hits {
			score[item] = h.VecSim
		}
	case cfg.Fusion == FusionWeighted:
		weightedScores(score, lex, vec, cfg)
	default:
		k := float64(cfg.RRFK)
		for item, h := range hits {
			if h.LexRank > 0 {
				score[item] += cfg.Weights.Lexical / (k + float64(h.LexRank))
			}
			if h.VecRank > 0 {
				score[item] += cfg.Weights.Vector / (k + float64(h.VecRank))
			}
		}
	}
	pin := -1
	if !vectorOnly { // vector mode ranks by cosine alone
		pin = r.exactPin(query, hits)
	}
	out := make([]SearchHit, 0, len(hits))
	for item, h := range hits {
		h.Score = math.Round(score[item]*1e9) / 1e9
		out = append(out, *h)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if pi, pj := out[i].Index == pin, out[j].Index == pin; pi != pj {
			return pi
		}
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return r.Items[out[i].Index].ID < r.Items[out[j].Index].ID
	})
	return out
}

// weightedScores min-max normalises each list's scores to [0,1] and mixes them.
func weightedScores(score map[int]float64, lex []lexHit, vec []vecHit, cfg Config) {
	a := cfg.Weights.Lexical / (cfg.Weights.Lexical + cfg.Weights.Vector)
	norm := func(scores []float64) func(float64) float64 {
		if len(scores) == 0 {
			return func(float64) float64 { return 0 }
		}
		lo, hi := scores[0], scores[0]
		for _, s := range scores {
			lo, hi = math.Min(lo, s), math.Max(hi, s)
		}
		return func(s float64) float64 {
			if hi == lo {
				return 1
			}
			return (s - lo) / (hi - lo)
		}
	}
	ls := make([]float64, len(lex))
	for i, h := range lex {
		ls[i] = h.score
	}
	vs := make([]float64, len(vec))
	for i, h := range vec {
		vs[i] = h.sim
	}
	ln, vn := norm(ls), norm(vs)
	for _, h := range lex {
		score[h.item] += a * ln(h.score)
	}
	for _, h := range vec {
		score[h.item] += (1 - a) * vn(h.sim)
	}
}

// exactPin returns the item whose id appears in the query, or -1. An id of
// several words ("deploy-staging") pins when it appears as a whole word
// sequence; an id that is one word ("test", "build", "fix") is also an English
// word, so it pins only when the query calls it a skill ("use the build skill").
// When several match, the longest id wins. Only exact ids pin, so a
// keyword-stuffed description gains nothing from it.
func (r *Ranker) exactPin(query string, in map[int]*SearchHit) int {
	q := " " + strings.Join(idTokens(query), " ") + " "
	best, bestLen := -1, 0
	for item := range in {
		tokens := idTokens(r.Items[item].ID)
		id := strings.Join(tokens, " ")
		if id == "" || len(id) < bestLen || !pins(q, id, len(tokens) > 1) {
			continue
		}
		if len(id) > bestLen || r.Items[item].ID < r.Items[best].ID {
			best, bestLen = item, len(id)
		}
	}
	return best
}

func pins(q, id string, multiWord bool) bool {
	if multiWord {
		return strings.Contains(q, " "+id+" ")
	}
	return strings.Contains(q, " "+id+" skill ") || strings.Contains(q, " skill "+id+" ")
}

func idTokens(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r > 127)
	})
}
