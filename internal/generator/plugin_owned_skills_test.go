package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ownedProject is pruneProject with [plugin] skills_dir_owned set.
func ownedProject(t *testing.T) string {
	t.Helper()
	dir, _ := pruneProject(t)
	cfgPath := filepath.Join(dir, ".ai-rulez", "config.toml")
	body, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	updated := strings.Replace(string(body), `runtimes = ["claude", "gemini", "cursor"]`,
		"runtimes = [\"claude\", \"gemini\", \"cursor\"]\nskills_dir_owned = true", 1)
	require.NotEqual(t, string(body), updated)
	require.NoError(t, os.WriteFile(cfgPath, []byte(updated), 0o600))
	require.NoError(t, loadDomainsProject(t, dir).GeneratePlugin(""))
	return dir
}

func TestVerifyPlugin_SkillsDirOwned(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "ghost skill directory", path: "skills/ghost-renamed/SKILL.md"},
		{name: "stray file directly under skills", path: "skills/obsolete.txt"},
		{name: "ghost directory in another runtime tree", path: ".cursor-plugin/skills/ghost/SKILL.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name+" is extra when owned", func(t *testing.T) {
			// Arrange
			dir := ownedProject(t)
			writeDomainsFile(t, filepath.Join(dir, filepath.FromSlash(tt.path)), "x")

			// Act
			err := loadDomainsProject(t, dir).VerifyPlugin("")

			// Assert
			require.ErrorIs(t, err, ErrPluginDrift)
			assert.Contains(t, PluginDriftOf(err), PluginDrift{Path: tt.path, Kind: PluginDriftExtra})
		})
		t.Run(tt.name+" is ignored by default", func(t *testing.T) {
			// Arrange
			dir, _ := pruneProject(t)
			writeDomainsFile(t, filepath.Join(dir, filepath.FromSlash(tt.path)), "x")

			// Act
			err := loadDomainsProject(t, dir).VerifyPlugin("")

			// Assert
			require.NoError(t, err)
		})
	}
}

func TestGeneratePlugin_SkillsDirOwnedRemovesUnrecordedEntries(t *testing.T) {
	// Arrange
	dir := ownedProject(t)
	ghost := filepath.Join(dir, "skills", "ghost-renamed")
	writeDomainsFile(t, filepath.Join(ghost, "SKILL.md"), "x")
	writeDomainsFile(t, filepath.Join(ghost, "old", "deep", "n.md"), "x")
	stray := filepath.Join(dir, "skills", "obsolete.txt")
	writeDomainsFile(t, stray, "x")

	// Act
	plan, err := loadDomainsProject(t, dir).DryRunPlugin("")
	require.NoError(t, err)
	assert.Contains(t, plan, "delete-stale: skills/ghost-renamed/SKILL.md")
	assert.Contains(t, plan, "delete-stale: skills/obsolete.txt")
	assert.FileExists(t, stray, "a dry run deletes nothing")
	require.NoError(t, loadDomainsProject(t, dir).GeneratePlugin(""))

	// Assert
	assert.NoFileExists(t, stray)
	assert.NoDirExists(t, ghost, "the emptied ghost directory is removed bottom-up")
	assert.FileExists(t, filepath.Join(dir, "skills", "core-s", "SKILL.md"))
	assert.DirExists(t, filepath.Join(dir, "skills"))
	require.NoError(t, loadDomainsProject(t, dir).VerifyPlugin(""))
}

func TestGeneratePlugin_DefaultNeverTouchesUnrecordedSkillDirs(t *testing.T) {
	// Arrange
	dir, _ := pruneProject(t)
	ghost := filepath.Join(dir, "skills", "ghost-audit", "SKILL.md")
	stray := filepath.Join(dir, "skills", "obsolete.txt")
	writeDomainsFile(t, ghost, "x")
	writeDomainsFile(t, stray, "x")

	// Act
	require.NoError(t, loadDomainsProject(t, dir).GeneratePlugin(""))

	// Assert
	assert.FileExists(t, ghost)
	assert.FileExists(t, stray)
}

func TestGeneratePlugin_RemovesDirectoriesLeftEmpty(t *testing.T) {
	// Arrange
	dir, _ := pruneProject(t)
	skill := filepath.Join(dir, "skills", "core-s")
	writeDomainsFile(t, filepath.Join(skill, "old", "deep", "n.md"), "x")
	writeDomainsFile(t, filepath.Join(skill, "keep", "a.md"), "x")
	writeDomainsFile(t, filepath.Join(skill, "keep", "sub", "b.md"), "x")
	require.NoError(t, os.MkdirAll(filepath.Join(skill, "unrelated-empty"), 0o750))

	// Act
	require.NoError(t, loadDomainsProject(t, dir).GeneratePlugin(""))

	// Assert
	assert.NoDirExists(t, filepath.Join(skill, "old"), "old/deep and old are removed bottom-up")
	assert.NoFileExists(t, filepath.Join(skill, "keep", "a.md"))
	assert.NoDirExists(t, filepath.Join(skill, "keep"))
	assert.DirExists(t, filepath.Join(skill, "unrelated-empty"), "a directory that was already empty is not ours")
	assert.FileExists(t, filepath.Join(skill, "SKILL.md"), "a generated skill directory stays")
}
