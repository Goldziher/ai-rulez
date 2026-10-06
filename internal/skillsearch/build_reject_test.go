package skillsearch

import (
	"context"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rejectingEmbedder fails any call whose texts contain the poison word, as a provider rejecting a batch with one bad input.
type rejectingEmbedder struct {
	*conceptEmbedder
	poison string
	err    error
	sizes  []int
}

func (r *rejectingEmbedder) Embed(ctx context.Context, texts []string) (Embedding, error) {
	r.sizes = append(r.sizes, len(texts))
	for _, t := range texts {
		if strings.Contains(t, r.poison) {
			return Embedding{}, r.err
		}
	}
	return r.conceptEmbedder.Embed(ctx, texts)
}

func TestBuild_ContinuesPastARejectedBatch(t *testing.T) {
	t.Parallel()
	rejection := &llm.Error{Kind: llm.KindProvider, Status: 400, Message: "input too long"}
	tests := []struct {
		name        string
		err         error
		wantStopped bool
	}{
		{"a 400 from the provider is retried smaller, then skipped", rejection, false},
		{"a spent budget still stops the build", llm.ErrBudget, true},
		{"a disabled network still stops the build", llm.ErrNetworkDisabled, true},
		{"a bad key still stops the build", llm.ErrAuth, true},
		{"a transient provider error still stops the build", &llm.Error{Kind: llm.KindProvider, Status: 503}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			emb := &rejectingEmbedder{conceptEmbedder: refundEmbedder(), poison: "staging", err: tt.err}

			// Act
			res, err := Build(t.Context(), refundCatalog(), &BuildOptions{Embedder: emb, BatchSize: 4})

			// Assert
			require.NoError(t, err)
			if tt.wantStopped {
				require.Error(t, res.Err)
				assert.Empty(t, res.Rejected)
				return
			}
			require.NoError(t, res.Err)
			assert.Equal(t, 3, res.Embedded)
			require.Len(t, res.Rejected, 1)
			assert.Equal(t, "deploy-staging", res.Rejected[0].ID)
			assert.Equal(t, []string{"deploy-staging"}, res.Missing)
			assert.Equal(t, []int{4, 2, 1, 1, 2}, emb.sizes, "the batch is halved until the bad text stands alone")
		})
	}
}

func TestBuild_StopsAfterManyConsecutiveRejections(t *testing.T) {
	t.Parallel()
	// Arrange: a provider that rejects everything is a broken provider, not six bad skills
	emb := &rejectingEmbedder{conceptEmbedder: refundEmbedder(), poison: "", err: &llm.Error{Kind: llm.KindProvider, Status: 400, Message: "bad"}}
	items := refundCatalog()
	for i := 0; i < 6; i++ {
		items = append(items, Item{ID: "extra-" + string(rune('a'+i)), Doc: Doc{Name: "extra", Description: "extra skill"}})
	}

	// Act
	res, err := Build(t.Context(), items, &BuildOptions{Embedder: emb, BatchSize: 1})

	// Assert
	require.NoError(t, err)
	require.Error(t, res.Err)
	assert.Len(t, res.Rejected, maxConsecutiveRejections)
}
