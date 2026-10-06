package skillsearch

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func autoRanker(t *testing.T, cfg Config) *Ranker {
	t.Helper()
	items := refundCatalog()
	emb := refundEmbedder()
	res, err := Build(t.Context(), items, &BuildOptions{Config: cfg, Embedder: emb})
	require.NoError(t, err)
	return &Ranker{Items: items, Cfg: cfg, Index: res.Index, Embedder: emb}
}

func TestRanker_HybridDefaultsToVectorWhenTheIndexIsFresh(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		cfg         Config
		stale       bool
		wantRanking string
	}{
		{"default fusion, fresh index ranks by vector", Config{Mode: ModeHybrid}, false, ModeVector},
		{"explicit rrf keeps the hybrid fusion", Config{Mode: ModeHybrid, Fusion: FusionRRF}, false, ModeHybrid},
		{"explicit weighted keeps the hybrid fusion", Config{Mode: ModeHybrid, Fusion: FusionWeighted}, false, ModeHybrid},
		{"default fusion with a stale skill fuses", Config{Mode: ModeHybrid}, true, ModeHybrid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			r := autoRanker(t, tt.cfg)
			if tt.stale {
				r.Items[1].Doc.Description = "Something else entirely"
			}

			// Act
			got := r.Search(t.Context(), "customer wants money back")

			// Assert
			assert.Empty(t, got.Degraded)
			assert.Equal(t, tt.wantRanking, got.Ranking)
			assert.Equal(t, "issue-refund", names(r, got.Hits)[0])
		})
	}
}

func TestRanker_HybridVectorDefaultKeepsTheExactIDPin(t *testing.T) {
	t.Parallel()
	// Arrange: the cosine alone would put the refund skill first
	r := autoRanker(t, Config{Mode: ModeHybrid})
	query := "customer wants money back, use deploy-staging"

	// Act
	got := r.Search(t.Context(), query)
	vec := r.SearchMode(t.Context(), ModeVector, query, nil)

	// Assert
	assert.Equal(t, ModeVector, got.Ranking)
	assert.Equal(t, "issue-refund", names(r, vec.Hits)[0])
	assert.Equal(t, "deploy-staging", names(r, got.Hits)[0], "a skill named in the query is still pinned")
}
