package skillsearch

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Goldziher/ai-rulez/v5/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func builtRanker(t *testing.T, mode string, emb *conceptEmbedder) *Ranker {
	t.Helper()
	items := refundCatalog()
	cfg := Config{Mode: mode}
	res, err := Build(t.Context(), items, &BuildOptions{Config: cfg, Embedder: emb})
	require.NoError(t, err)
	require.NoError(t, res.Err)
	return &Ranker{Items: items, Cfg: cfg, Index: res.Index, Embedder: emb}
}

func names(r *Ranker, hits []SearchHit) []string {
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = r.Items[h.Index].ID
	}
	return out
}

func TestRanker_HybridFindsAParaphraseLexicalMisses(t *testing.T) {
	t.Parallel()
	// Arrange: the query shares no stem with the right skill's text
	emb := refundEmbedder()
	r := builtRanker(t, ModeHybrid, emb)
	query := "money back"

	// Act
	lex := r.SearchMode(t.Context(), ModeLexical, query, nil)
	hyb := r.Search(t.Context(), query)

	// Assert
	assert.Equal(t, ModeLexical, lex.Ranking)
	assert.Empty(t, lex.Hits, "no word of the query is in any skill: BM25F finds nothing")
	assert.Equal(t, ModeHybrid, hyb.Ranking)
	assert.Empty(t, hyb.Degraded)
	assert.Equal(t, "issue-refund", names(r, hyb.Hits)[0])
	assert.True(t, hyb.Embedded)
	assert.Positive(t, hyb.Hits[0].VecRank)
}

func TestRanker_DegradesToLexicalWithAReason(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(r *Ranker, e *conceptEmbedder)
		want   string
	}{
		{"no index", func(r *Ranker, _ *conceptEmbedder) { r.Index = nil }, DegradedNoIndex},
		{"no embedder", func(r *Ranker, _ *conceptEmbedder) { r.Embedder = nil }, DegradedProvider},
		{"provider down", func(_ *Ranker, e *conceptEmbedder) { e.err = errProvider }, DegradedProvider},
		{"network disabled", func(_ *Ranker, e *conceptEmbedder) { e.err = llm.ErrNetworkDisabled }, DegradedNetworkDisabled},
		{"budget", func(_ *Ranker, e *conceptEmbedder) { e.err = llm.ErrBudget }, DegradedBudget},
		{"timeout", func(_ *Ranker, e *conceptEmbedder) { e.err = context.DeadlineExceeded }, DegradedTimeout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			emb := refundEmbedder()
			r := builtRanker(t, ModeHybrid, emb)
			tt.mutate(r, emb)
			want := r.SearchMode(t.Context(), ModeLexical, "staging rollout", nil)

			// Act
			got := r.Search(t.Context(), "staging rollout")

			// Assert
			assert.Equal(t, tt.want, got.Degraded)
			assert.Equal(t, ModeLexical, got.Ranking)
			assert.Equal(t, want.Hits, got.Hits, "the fallback is exactly the lexical ranking")
		})
	}
}

func TestRanker_LexicalModeNeverEmbeds(t *testing.T) {
	t.Parallel()
	emb := refundEmbedder()
	r := builtRanker(t, ModeLexical, emb)
	emb.calls = 0

	res := r.Search(t.Context(), "staging")

	assert.Equal(t, 0, emb.calls)
	assert.Empty(t, res.Degraded)
	assert.Equal(t, ModeLexical, res.Ranking)
}

func TestRanker_QueryTimeoutFallsBack(t *testing.T) {
	t.Parallel()
	emb := &slowEmbedder{conceptEmbedder: refundEmbedder()}
	items := refundCatalog()
	res, err := Build(t.Context(), items, &BuildOptions{Embedder: emb.conceptEmbedder})
	require.NoError(t, err)
	r := &Ranker{Items: items, Cfg: Config{Mode: ModeHybrid}, Index: res.Index, Embedder: emb, Timeout: 20 * time.Millisecond}

	got := r.Search(t.Context(), "money back")

	assert.Equal(t, DegradedTimeout, got.Degraded)
}

type slowEmbedder struct{ *conceptEmbedder }

func (s *slowEmbedder) Embed(ctx context.Context, texts []string) (Embedding, error) {
	select {
	case <-ctx.Done():
		return Embedding{}, ctx.Err()
	case <-time.After(5 * time.Second):
		return s.conceptEmbedder.Embed(ctx, texts)
	}
}

func TestRanker_QueryCacheEmbedsOnce(t *testing.T) {
	t.Parallel()
	emb := refundEmbedder()
	r := builtRanker(t, ModeHybrid, emb)
	emb.calls = 0

	first := r.Search(t.Context(), "money back")
	second := r.Search(t.Context(), "money back")

	assert.Equal(t, 1, emb.calls)
	assert.True(t, first.Embedded)
	assert.False(t, second.Embedded)
	assert.Equal(t, first.Hits, second.Hits)
}

func TestRanker_QueryCacheIsBounded(t *testing.T) {
	t.Parallel()
	emb := refundEmbedder()
	r := builtRanker(t, ModeHybrid, emb)
	for i := range queryCacheSize + 50 {
		r.Search(t.Context(), fmt.Sprintf("query %d money", i))
	}
	assert.LessOrEqual(t, r.lru.Len(), queryCacheSize)
	assert.LessOrEqual(t, len(r.cache), queryCacheSize)
}

func TestRanker_StaleItemsRankLexicallyAndAreFlagged(t *testing.T) {
	t.Parallel()
	emb := refundEmbedder()
	r := builtRanker(t, ModeHybrid, emb)
	// the refund skill's description changed after indexing
	r.Items[0].Doc.Description = "Reimburse a customer for a returned purchase, now also with credit notes"
	r.once, r.rows = onceReset(), nil

	got := r.Search(t.Context(), "reimburse customer")

	require.NotEmpty(t, got.Hits)
	var flagged []string
	for _, h := range got.Hits {
		if h.StaleVec {
			flagged = append(flagged, r.Items[h.Index].ID)
			assert.Zero(t, h.VecRank, "a stale vector is not used")
		}
	}
	assert.Equal(t, []string{"issue-refund"}, flagged)
	for _, h := range got.Hits {
		if r.Items[h.Index].ID == "issue-refund" {
			assert.Equal(t, 1, h.LexRank, "it still ranks through the lexical list")
		}
	}
}

func TestRanker_ExactNamePin(t *testing.T) {
	t.Parallel()
	emb := refundEmbedder()
	r := builtRanker(t, ModeHybrid, emb)

	got := r.Search(t.Context(), "use the deploy-staging skill to handle a chargeback dispute from a bank")

	assert.Equal(t, "deploy-staging", names(r, got.Hits)[0], "an exact skill id in the query pins that skill first")
}

func TestRanker_VectorModeScoresByCosine(t *testing.T) {
	t.Parallel()
	emb := refundEmbedder()
	r := builtRanker(t, ModeVector, emb)

	got := r.Search(t.Context(), "reimburse")

	assert.Equal(t, ModeVector, got.Ranking)
	assert.Equal(t, "issue-refund", names(r, got.Hits)[0])
	assert.InDelta(t, got.Hits[0].VecSim, got.Hits[0].Score, 1e-9)
	assert.Zero(t, got.Hits[0].LexRank, "vector mode has no lexical list")
}

func TestRanker_RoleScopeFiltersBothLists(t *testing.T) {
	t.Parallel()
	emb := refundEmbedder()
	r := builtRanker(t, ModeHybrid, emb)
	allow := func(i int) bool { return r.Items[i].ID != "issue-refund" }

	got := r.SearchScoped(t.Context(), "money back", allow)

	assert.NotContains(t, names(r, got.Hits), "issue-refund")
}

func TestFuse_RRFHandComputed(t *testing.T) {
	t.Parallel()
	// Arrange: lexical [A, B], vector [B, C], k = 60
	r := &Ranker{Items: []Item{{ID: "a"}, {ID: "b"}, {ID: "c"}}}
	lex := []lexHit{{0, 9}, {1, 8}}
	vec := []vecHit{{1, 0.9}, {2, 0.8}}
	cfg := Config{Fusion: FusionRRF}.Resolved()

	// Act
	got := r.fuse(lex, vec, cfg, "query", false)

	// Assert: B = 1/62 + 1/61, A = 1/61, C = 1/62
	require.Len(t, got, 3)
	assert.Equal(t, []int{1, 0, 2}, []int{got[0].Index, got[1].Index, got[2].Index})
	assert.InDelta(t, 1.0/62+1.0/61, got[0].Score, 1e-9)
	assert.InDelta(t, 1.0/61, got[1].Score, 1e-9)
	assert.InDelta(t, 1.0/62, got[2].Score, 1e-9)
	assert.Equal(t, 2, got[0].LexRank)
	assert.Equal(t, 1, got[0].VecRank)
}

func TestFuse_RRFWeightsAndTies(t *testing.T) {
	t.Parallel()
	r := &Ranker{Items: []Item{{ID: "b"}, {ID: "a"}}}
	lex := []lexHit{{0, 1}}
	vec := []vecHit{{1, 1}}

	equal := r.fuse(lex, vec, Config{}.Resolved(), "q", false)
	heavy := r.fuse(lex, vec, Config{Weights: Weights{Lexical: 1, Vector: 3}}.Resolved(), "q", false)

	assert.Equal(t, []int{1, 0}, []int{equal[0].Index, equal[1].Index}, "equal scores order by id")
	assert.Equal(t, 1, heavy[0].Index, "the heavier list wins")
}

func TestFuse_Weighted(t *testing.T) {
	t.Parallel()
	// min-max over each list, then a convex mix with a = 0.5
	r := &Ranker{Items: []Item{{ID: "a"}, {ID: "b"}, {ID: "c"}}}
	lex := []lexHit{{0, 10}, {1, 5}, {2, 0.0}}
	vec := []vecHit{{1, 0.9}, {2, 0.5}, {0, 0.1}}
	cfg := Config{Fusion: FusionWeighted}.Resolved()

	got := r.fuse(lex, vec, cfg, "q", false)

	// a: lex 1.0, vec 0.0 -> 0.5; b: lex 0.5, vec 1.0 -> 0.75; c: lex 0, vec 0.5 -> 0.25
	assert.Equal(t, []int{1, 0, 2}, []int{got[0].Index, got[1].Index, got[2].Index})
	assert.InDelta(t, 0.75, got[0].Score, 1e-9)
	assert.InDelta(t, 0.5, got[1].Score, 1e-9)
	assert.InDelta(t, 0.25, got[2].Score, 1e-9)
}

func TestFuse_CandidatesTruncateBothLists(t *testing.T) {
	t.Parallel()
	r := &Ranker{Items: []Item{{ID: "a"}, {ID: "b"}, {ID: "c"}}}
	lex := []lexHit{{0, 3}, {1, 2}, {2, 1}}
	vec := []vecHit{{2, 3}, {1, 2}, {0, 1}}

	got := r.fuse(lex, vec, Config{Candidates: 1}.Resolved(), "q", false)

	assert.Len(t, got, 2, "only the first candidate of each list takes part")
}

func TestExactPin_Matching(t *testing.T) {
	t.Parallel()
	r := &Ranker{Items: []Item{{ID: "deploy"}, {ID: "deploy-staging"}, {ID: "stage"}, {ID: "test"}}}
	all := map[int]*SearchHit{0: {}, 1: {}, 2: {}, 3: {}}
	tests := []struct {
		query string
		want  int
	}{
		{"use the deploy-staging skill", 1},
		{"Deploy Staging please", 1},
		{"how do I deploy", -1},
		{"use the deploy skill", 0},
		{"the skill deploy please", 0},
		{"staging deployments", -1},
		{"nothing relevant here", -1},
		{"write the failing test first", -1},
		{"use the test skill", 3},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, r.exactPin(tt.query, all), tt.query)
	}
}

func TestRanker_ConcurrentSearch(t *testing.T) {
	t.Parallel()
	emb := refundEmbedder()
	r := builtRanker(t, ModeHybrid, emb)
	done := make(chan struct{})
	for i := range 8 {
		go func() {
			defer func() { done <- struct{}{} }()
			r.Search(context.Background(), fmt.Sprintf("money back %d", i%3))
		}()
	}
	for range 8 {
		<-done
	}
}
