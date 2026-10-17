package commands

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRootLayoutGenerate_SkillsIndexIsManagedLikeAnyOutput(t *testing.T) {
	// Arrange: the config directory is the project root, so the index sits there.
	root := rootLayoutProject(t, rootLayoutInstalled+"\n[usage]\nskills_index = true\n")

	// Act
	generated := runCLI(t, "generate", "--yes")
	index := readFileString(t, filepath.Join(root, "skills-index.json"))
	check := runCLI(t, "generate", "--check")
	cleaned := runCLI(t, "clean", "--yes")

	// Assert
	require.Equal(t, 0, generated.code, generated.stdout+generated.stderr)
	assert.Contains(t, index, `"demo"`)
	assert.Equal(t, 0, check.code, "the index is not drift: %s%s", check.stdout, check.stderr)
	require.Equal(t, 0, cleaned.code, cleaned.stdout+cleaned.stderr)
	assert.NoFileExists(t, filepath.Join(root, "skills-index.json"), "clean takes back what generate wrote")
	assert.FileExists(t, filepath.Join(root, "ai-rulez.toml"))
}
