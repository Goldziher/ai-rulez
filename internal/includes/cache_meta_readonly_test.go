package includes

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A load that finds the include cache current must not rewrite its metadata: a
// read-only command (an MCP list or read) would otherwise write under HOME on
// every call. Only a fetch that refreshed the cache records a new fetched_at.
func TestGitSourceFetch_CacheHitLeavesTheMetaUntouched(t *testing.T) {
	// Arrange
	repoDir := initLocalRepo(t)
	commitFile(t, repoDir, ".ai-rulez/rules/rule.md", "---\nname: rule1\npriority: high\n---\n# Rule")
	cacheDir := t.TempDir()
	source := &GitSource{
		name:        "meta-readonly",
		repoURL:     normalizeGitURL("file://" + repoDir),
		originalURL: "file://" + repoDir,
		cacheDir:    cacheDir,
	}
	ctx := context.Background()
	_, err := source.Fetch(ctx)
	require.NoError(t, err)
	metaPath := filepath.Join(cacheDir, cacheMetaFile)
	before, err := os.ReadFile(metaPath)
	require.NoError(t, err)
	infoBefore, err := os.Stat(metaPath)
	require.NoError(t, err)

	// Act
	_, err = source.Fetch(ctx)

	// Assert
	require.NoError(t, err)
	after, err := os.ReadFile(metaPath)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "a cache hit rewrote the metadata")
	infoAfter, err := os.Stat(metaPath)
	require.NoError(t, err)
	assert.Equal(t, infoBefore.ModTime(), infoAfter.ModTime())
}
