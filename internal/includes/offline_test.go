package includes

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// An offline context must resolve from the cache only: with nothing cached the
// fetch fails immediately instead of reaching for the network.
func TestGitSourceFetch_OfflineContextNeverFetches(t *testing.T) {
	// Arrange
	src, err := NewGitSource("offline-ctx-test-"+t.Name(), "https://example.invalid/none/repo.git", "", "", t.TempDir(), nil, "")
	require.NoError(t, err)
	ctx := config.WithOfflineIncludes(context.Background())

	// Act
	_, err = src.Fetch(ctx)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no cached content")
}

func TestSkillGitSourceFetch_OfflineContextNeverFetches(t *testing.T) {
	// Arrange
	src, err := NewSkillGitSource("offline-skill-test-"+t.Name(), "https://example.invalid/none/repo.git", "", "", "")
	require.NoError(t, err)
	ctx := config.WithOfflineIncludes(context.Background())

	// Act
	_, err = src.Fetch(ctx)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no cached skill")
}
