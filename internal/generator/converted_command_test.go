package generator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const importedCommand = "---\ndescription: Summarize the day\n---\nSummarize the day.\n"

// commandProject is a project whose `daily` command convert imported from
// .claude/commands/daily.md, copying it verbatim to .ai-rulez/commands/daily.md;
// the claude preset renders it as a skill.
func commandProject(t *testing.T, onDisk string) (dir, original string) {
	t.Helper()
	dir = hashesProject(t, "")
	commands := filepath.Join(dir, ".ai-rulez", "commands")
	require.NoError(t, os.MkdirAll(commands, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(commands, "daily.md"), []byte(importedCommand), 0o644))
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

func TestGenerateThenClean_LeavesAConvertedCommandAsTheOriginalOrTheSkill(t *testing.T) {
	quietWarnings(t)
	// Arrange
	dir, original := commandProject(t, importedCommand)
	skill := filepath.Join(dir, ".claude", "skills", "daily", "SKILL.md")
	require.NoError(t, newProjectGenerator(t, dir).Generate("default"))
	require.NoFileExists(t, original, "generate retired the command")

	// Act
	plan, err := newProjectGenerator(t, dir).Clean("default", CleanOptions{})

	// Assert
	require.NoError(t, err)
	assert.Contains(t, plan.Restored, original)
	assert.NoFileExists(t, skill)
	data, rerr := os.ReadFile(original)
	require.NoError(t, rerr, "clean puts the retired command back")
	assert.Equal(t, importedCommand, string(data))
}

func TestCleanDryRun_ListsTheRetiredCommandItRestores(t *testing.T) {
	quietWarnings(t)
	// Arrange
	dir, original := commandProject(t, importedCommand)
	require.NoError(t, newProjectGenerator(t, dir).Generate("default"))

	// Act
	plan, err := newProjectGenerator(t, dir).Clean("default", CleanOptions{DryRun: true})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{original}, plan.Restored)
	assert.NotContains(t, plan.Dirs, filepath.Join(dir, ".claude"), "the restored command keeps its folder")
	assert.NoFileExists(t, original, "a dry run writes nothing")
}

func TestGenerate_KeepsAConvertedCommandCleanCouldNotRestore(t *testing.T) {
	quietWarnings(t)
	// Arrange
	dir, original := commandProject(t, importedCommand)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", "commands", "daily.md"),
		[]byte("---\ndescription: Summarize the day\n---\nSummarize the whole day.\n"), 0o644))

	// Act
	err := newProjectGenerator(t, dir).Generate("default")

	// Assert
	require.NoError(t, err)
	data, rerr := os.ReadFile(original)
	require.NoError(t, rerr, "the imported copy changed, so only the original holds the imported bytes")
	assert.Equal(t, importedCommand, string(data))
}

func TestClean_KeepsTheSkillOfARetiredCommandItCannotRestore(t *testing.T) {
	quietWarnings(t)
	// Arrange
	dir, original := commandProject(t, importedCommand)
	skill := filepath.Join(dir, ".claude", "skills", "daily", "SKILL.md")
	require.NoError(t, newProjectGenerator(t, dir).Generate("default"))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", "commands", "daily.md"),
		[]byte("---\ndescription: Summarize the day\n---\nSummarize the whole day.\n"), 0o644))

	// Act
	plan, err := newProjectGenerator(t, dir).Clean("default", CleanOptions{})

	// Assert
	require.NoError(t, err)
	assert.Empty(t, plan.Restored)
	assert.FileExists(t, skill, "removing it would leave neither the command nor the skill")
	assert.NoFileExists(t, original)
}
