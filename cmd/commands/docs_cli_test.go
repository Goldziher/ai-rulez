package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/clidocs"
)

// The command reference of the docs is generated from the command tree
// (`task docs:cli`). A command, flag, default, example or description that
// changes without regenerating the docs fails here, so docs/cli-reference.md and
// the command index of docs/cli.md cannot drift from the binary.
func TestGeneratedCLIDocsMatchTheCommandTree(t *testing.T) {
	// Arrange
	root := CommandTree()
	docs := filepath.Join("..", "..", "docs")
	cliPage, err := os.ReadFile(filepath.Join(docs, "cli.md"))
	require.NoError(t, err)
	reference, err := os.ReadFile(filepath.Join(docs, "cli-reference.md"))
	require.NoError(t, err)

	// Act
	wantCLI, applyErr := clidocs.ApplyIndex(string(cliPage), clidocs.Index(root))

	// Assert
	require.NoError(t, applyErr)
	assert.Equal(t, wantCLI, string(cliPage), "the command index of docs/cli.md is stale: run `task docs:cli`")
	assert.Equal(t, clidocs.Reference(root), string(reference), "docs/cli-reference.md is stale: run `task docs:cli`")
}
