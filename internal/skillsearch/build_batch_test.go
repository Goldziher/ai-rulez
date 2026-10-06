package skillsearch

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type cappedEmbedder struct {
	*conceptEmbedder
	max   int
	sizes []int
}

func (c *cappedEmbedder) MaxBatch() int { return c.max }
func (c *cappedEmbedder) Embed(ctx context.Context, texts []string) (Embedding, error) {
	c.sizes = append(c.sizes, len(texts))
	return c.conceptEmbedder.Embed(ctx, texts)
}

func TestBuild_EmbedderBatchLimit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		cfg  Config
		opt  int
	}{
		{"an embedder that cannot batch gets one text per call", Config{}, 0},
		{"a larger configured size is lowered to the limit", Config{BatchSize: 64}, 0},
		{"an explicit option is also bounded", Config{}, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			emb := &cappedEmbedder{conceptEmbedder: refundEmbedder(), max: 1}

			// Act
			res, err := Build(t.Context(), refundCatalog(), &BuildOptions{Config: tt.cfg, Embedder: emb, BatchSize: tt.opt})

			// Assert
			require.NoError(t, err)
			assert.Equal(t, 4, res.Embedded)
			assert.Equal(t, []int{1, 1, 1, 1}, emb.sizes)
		})
	}
}
