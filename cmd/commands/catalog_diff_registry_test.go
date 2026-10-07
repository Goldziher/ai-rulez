package commands

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A revision's configuration is validated like the working tree's, provider
// specs included: a committed spec that does not match its preset is an error,
// not a silently skipped check.
func TestCatalogDiffRevisionValidatesProviderSpecs(t *testing.T) {
	// Arrange
	root := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"),
		"version = \"5.0\"\nname = \"p\"\npresets = [\"claude\", { name = \"demo\", provider = \".ai-rulez/providers/demo.toml\" }]\n")
	writeFile(t, filepath.Join(root, ".ai-rulez", "providers", "demo.toml"), "name = \"other\"\n\n[root]\nfile = \"DEMO.md\"\nsections = [\"title\"]\n")
	writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "r.md"), "# R\nBody.\n")
	gitIn(t, root, "init", "-q", "-b", "main")
	gitIn(t, root, "add", "-A")
	gitIn(t, root, "commit", "-q", "-m", "one")
	t.Chdir(root)
	resetCatalogDiffFlags(t)

	// Act
	_, err := revisionSide(context.Background(), &catalogDiffProject{}, "HEAD")

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match preset name")
}
