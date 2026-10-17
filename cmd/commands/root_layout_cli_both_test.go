package commands

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func bothLayoutsProject(t *testing.T) string {
	t.Helper()
	root := rootLayoutProject(t, "version = \"5.0\"\nname = \"file\"\npresets = [\"cursor\"]\n")
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), "version = \"5.0\"\nname = \"dir\"\npresets = [\"claude\"]\ngitignore = false\n")
	writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "r.md"), "# R\n\nbody\n")
	return root
}

func TestRootLayoutBothLayouts_DoctorReportsTheIgnoredFile(t *testing.T) {
	// Arrange
	bothLayoutsProject(t)

	// Act
	res := runCLI(t, "doctor")

	// Assert
	assert.Contains(t, res.stdout+res.stderr, "ai-rulez.toml is ignored: .ai-rulez/ takes precedence")
}

func TestRootLayoutBothLayouts_GenerateUsesTheDirectory(t *testing.T) {
	// Arrange
	root := bothLayoutsProject(t)

	// Act
	res := runCLI(t, "generate", "--yes")

	// Assert
	require.Equal(t, 0, res.code, res.stdout+res.stderr)
	assert.FileExists(t, filepath.Join(root, ".ai-rulez", ".generated-manifest.json"))
	assert.NoFileExists(t, filepath.Join(root, ".ai-rulez-manifest.json"))
	assert.FileExists(t, filepath.Join(root, "CLAUDE.md"), "the claude preset of the directory config ran")
	assert.NoFileExists(t, filepath.Join(root, ".cursor", "rules"), "the cursor preset of the ignored file did not")
}
