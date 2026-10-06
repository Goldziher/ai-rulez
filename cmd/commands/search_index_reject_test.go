package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchIndex_SkipsASkillTheProviderRejects(t *testing.T) {
	// Arrange: the provider refuses any batch holding the deploy skill
	searchProject(t)
	resetSearch(t)
	srv := newEmbedServer(t)
	srv.reject = "staging"
	useEmbeddings(t, srv, true)

	// Act
	code, out, errOut := runIndex(t)

	// Assert: the other skills are indexed, the rejected one is named, the run reports findings
	assert.Equal(t, exitSearchGateFailed, code)
	assert.Contains(t, out, "embedded 2, reused 0")
	assert.Contains(t, errOut, `rejected skill "deploy-staging"`)
	assert.Contains(t, errOut, "ranks lexically only")
	_, statErr := os.Stat(filepath.Join(".ai-rulez", "local", "search", "manifest.json"))
	require.NoError(t, statErr, "the finished vectors are written")
}
