package generator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func TestVerifyPlugin_ReportsUnrecordedFilesAndSymlinks(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, dir string)
		want   PluginDrift
	}{
		{
			name: "stray file in a generated skill directory is extra",
			mutate: func(t *testing.T, dir string) {
				writeDomainsFile(t, filepath.Join(dir, "skills", "core-s", "stray.md"), "x")
			},
			want: PluginDrift{Path: "skills/core-s/stray.md", Kind: PluginDriftExtra},
		},
		{
			name: "stray file in a nested skill directory is extra",
			mutate: func(t *testing.T, dir string) {
				writeDomainsFile(t, filepath.Join(dir, ".cursor-plugin", "skills", "core-s", "refs", "x.md"), "x")
			},
			want: PluginDrift{Path: ".cursor-plugin/skills/core-s/refs/x.md", Kind: PluginDriftExtra},
		},
		{
			name: "symlink with the recorded content is a symlink",
			mutate: func(t *testing.T, dir string) {
				skill := filepath.Join(dir, "skills", "core-s", "SKILL.md")
				body, err := os.ReadFile(skill)
				require.NoError(t, err)
				target := filepath.Join(t.TempDir(), "same.md")
				require.NoError(t, os.WriteFile(target, body, 0o600))
				require.NoError(t, os.Remove(skill))
				testutil.SymlinkOrSkip(t, target, skill)
			},
			want: PluginDrift{Path: "skills/core-s/SKILL.md", Kind: PluginDriftSymlink},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir, _ := pruneProject(t)
			require.NoError(t, loadDomainsProject(t, dir).VerifyPlugin(""))
			tt.mutate(t, dir)

			// Act
			err := loadDomainsProject(t, dir).VerifyPlugin("")

			// Assert
			require.ErrorIs(t, err, ErrPluginDrift)
			assert.Contains(t, PluginDriftOf(err), tt.want)
		})
	}
}

func TestGeneratePlugin_RemovesUnrecordedFilesInGeneratedSkillDirs(t *testing.T) {
	// Arrange
	dir, _ := pruneProject(t)
	stray := filepath.Join(dir, "skills", "core-s", "stray.md")
	writeDomainsFile(t, stray, "x")
	handMade := filepath.Join(dir, "skills", "hand-made", "SKILL.md")
	writeDomainsFile(t, handMade, "mine")

	// Act
	plan, err := loadDomainsProject(t, dir).DryRunPlugin("")
	require.NoError(t, err)
	assert.Contains(t, plan, "delete-stale: skills/core-s/stray.md")
	assert.FileExists(t, stray, "a dry run deletes nothing")
	require.NoError(t, loadDomainsProject(t, dir).GeneratePlugin(""))

	// Assert
	assert.NoFileExists(t, stray)
	assert.FileExists(t, filepath.Join(dir, "skills", "core-s", "SKILL.md"))
	assert.FileExists(t, handMade, "a directory the bundle does not generate is never touched")
	require.NoError(t, loadDomainsProject(t, dir).VerifyPlugin(""))
}

func TestVerifyPlugin_NamesEachDriftedPath(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, dir string)
		want   []PluginDrift
	}{
		{
			name: "edited file is changed",
			mutate: func(t *testing.T, dir string) {
				appendTo(t, filepath.Join(dir, "skills", "core-s", "SKILL.md"), "x\n")
			},
			want: []PluginDrift{{Path: "skills/core-s/SKILL.md", Kind: PluginDriftChanged}},
		},
		{
			name: "deleted file is missing",
			mutate: func(t *testing.T, dir string) {
				require.NoError(t, os.Remove(filepath.Join(dir, "skills", "core-s", "SKILL.md")))
			},
			want: []PluginDrift{{Path: "skills/core-s/SKILL.md", Kind: PluginDriftMissing}},
		},
		{
			name: "removed skill leaves an obsolete file",
			mutate: func(t *testing.T, dir string) {
				removeCoreSkill(t, dir)
			},
			want: []PluginDrift{
				{Path: ".cursor-plugin/skills/core-s/SKILL.md", Kind: PluginDriftObsolete},
				{Path: "skills/core-s/SKILL.md", Kind: PluginDriftObsolete},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir, _ := pruneProject(t)
			tt.mutate(t, dir)

			// Act
			err := loadDomainsProject(t, dir).VerifyPlugin("")

			// Assert
			require.ErrorIs(t, err, ErrPluginDrift)
			assert.Subset(t, PluginDriftOf(err), tt.want)
		})
	}
}

func TestPluginDriftOf_NilForOtherErrors(t *testing.T) {
	assert.Nil(t, PluginDriftOf(nil))
	assert.Nil(t, PluginDriftOf(os.ErrNotExist))
}
