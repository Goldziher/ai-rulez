package generator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerate_LocalManifestIgnoredOnFirstRun(t *testing.T) {
	// Arrange
	root := newSecretMCPRepo(t, `["vibe", "claude"]`)

	// Act
	generateRepo(t, root)
	first, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	require.NoError(t, err)
	generateRepo(t, root)
	second, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	require.NoError(t, err)

	// Assert
	assert.Contains(t, string(first), ".ai-rulez/.generated-manifest.local.json")
	assert.Equal(t, string(first), string(second), "run 1 and run 2 write the same .gitignore")
}
