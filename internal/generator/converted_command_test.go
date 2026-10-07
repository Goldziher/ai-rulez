package generator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const importedCommand = "Summarize the day.\n"

// commandProject is a project whose `daily` command convert imported from
// .claude/commands/daily.md; the claude preset renders it as a skill.
func commandProject(t *testing.T, onDisk string) (dir, original string) {
	t.Helper()
	dir = hashesProject(t, "")
	commands := filepath.Join(dir, ".ai-rulez", "commands")
	require.NoError(t, os.MkdirAll(commands, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(commands, "daily.md"),
		[]byte("---\ndescription: Summarize the day\n---\n"+importedCommand), 0o644))
	original = filepath.Join(dir, ".claude", "commands", "daily.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(original), 0o755))
	require.NoError(t, os.WriteFile(original, []byte(onDisk), 0o644))
	require.NoError(t, WriteConvertRecord(filepath.Join(dir, ".ai-rulez"),
		map[string][]byte{".claude/commands/daily.md": []byte(importedCommand)}))
	return dir, original
}

func TestGenerate_RemovesAConvertedCommandItsSkillReplaces(t *testing.T) {
	quietWarnings(t)
	// Arrange
	dir, original := commandProject(t, importedCommand)

	// Act
	err := newProjectGenerator(t, dir).Generate("default")

	// Assert
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(dir, ".claude", "skills", "daily", "SKILL.md"))
	assert.NoFileExists(t, original, "the command would otherwise appear twice, as a command and as a skill")
}

func TestGenerate_KeepsAConvertedCommandEditedSinceConvert(t *testing.T) {
	quietWarnings(t)
	// Arrange
	dir, original := commandProject(t, importedCommand+"my own addition\n")

	// Act
	err := newProjectGenerator(t, dir).Generate("default")

	// Assert
	require.NoError(t, err)
	assert.FileExists(t, original)
}

func TestDryRun_ListsTheConvertedCommandItRemoves(t *testing.T) {
	quietWarnings(t)
	// Arrange
	dir, original := commandProject(t, importedCommand)

	// Act
	lines, err := newProjectGenerator(t, dir).DryRun("default")

	// Assert
	require.NoError(t, err)
	assert.Contains(t, lines, "delete-stale: .claude/commands/daily.md")
	assert.FileExists(t, original, "a dry run writes nothing")
}

func TestClean_KeepsAConvertedCommandOriginal(t *testing.T) {
	quietWarnings(t)
	// Arrange
	dir, original := commandProject(t, importedCommand)
	require.NoError(t, newProjectGenerator(t, dir).Generate("default"))
	require.NoError(t, os.MkdirAll(filepath.Dir(original), 0o755))
	require.NoError(t, os.WriteFile(original, []byte(importedCommand), 0o644))

	// Act
	plan, err := newProjectGenerator(t, dir).Clean("default", CleanOptions{})

	// Assert
	require.NoError(t, err)
	assert.NotContains(t, plan.Files, original)
}
