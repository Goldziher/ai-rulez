package generator

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// TestGenerateMemberFiles_OmitsTheMarketplaceIndex is the #516 regression: a root
// an enclosing config lists as a marketplace member renders the bundle the index
// points at, not a standalone plugin with an index of its own.
func TestGenerateMemberFiles_OmitsTheMarketplaceIndex(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, ".ai-rulez", "config.toml"),
		"version = \"5.0\"\nname = \"member\"\n\n[plugin]\nname = \"member\"\n")
	writeTestFile(t, filepath.Join(dir, ".ai-rulez", "rules", "always.md"), "# Always\n\nBe nice.\n")
	cfg, err := config.LoadConfig(t.Context(), dir)
	require.NoError(t, err)

	// Act
	written, err := NewGenerator(cfg).GenerateMemberFiles("")

	// Assert
	require.NoError(t, err)
	assert.Positive(t, written)
	assert.FileExists(t, filepath.Join(dir, ".claude-plugin", "plugin.json"))
	assert.NoFileExists(t, filepath.Join(dir, ".claude-plugin", "marketplace.json"))
	assert.NoFileExists(t, filepath.Join(dir, ".cursor-plugin", "marketplace.json"))
	assert.NoFileExists(t, filepath.Join(dir, ".github", "plugin", "marketplace.json"))
}
