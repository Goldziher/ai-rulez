package generator

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dirsUnder lists every directory below root (excluding root) as slash paths.
func dirsUnder(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	require.NoError(t, filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		require.NoError(t, err)
		if rel, relErr := filepath.Rel(root, path); relErr == nil && rel != "." && d.IsDir() {
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	}))
	sort.Strings(out)
	return out
}

func TestUserClean_RemovesTheDirectoriesGenerateCreated(t *testing.T) {
	quietWarnings(t)
	// Arrange
	home, gen := newUserHome(t, userConfigTOML, userFixture())
	_, err := gen.GenerateUser("")
	require.NoError(t, err)
	require.DirExists(t, filepath.Join(home, ".cursor"))

	// Act
	_, err = loadUserGenerator(t, home).Clean("", CleanOptions{KeepGitignore: true})

	// Assert: only the user config is left in the home directory.
	require.NoError(t, err)
	assert.Equal(t, []string{".config", ".config/ai-rulez", ".config/ai-rulez/agents", ".config/ai-rulez/domains",
		".config/ai-rulez/domains/work", ".config/ai-rulez/domains/work/rules", ".config/ai-rulez/domains/work/skills",
		".config/ai-rulez/domains/work/skills/work-skill", ".config/ai-rulez/rules", ".config/ai-rulez/skills",
		".config/ai-rulez/skills/my-skill"}, dirsUnder(t, home))
}

func TestUserClean_KeepsDirectoriesThatExistedBeforeGenerate(t *testing.T) {
	quietWarnings(t)
	// Arrange: an empty ~/.cursor and ~/.claude/skills the user made earlier.
	home, gen := newUserHome(t, userConfigTOML, userFixture())
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".cursor"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".claude", "skills"), 0o755))
	_, err := gen.GenerateUser("")
	require.NoError(t, err)
	// A second generate must not mistake the now-populated directories for its own.
	_, err = loadUserGenerator(t, home).GenerateUser("")
	require.NoError(t, err)

	// Act
	_, err = loadUserGenerator(t, home).Clean("", CleanOptions{KeepGitignore: true})

	// Assert
	require.NoError(t, err)
	assert.DirExists(t, filepath.Join(home, ".cursor"))
	assert.DirExists(t, filepath.Join(home, ".claude", "skills"))
	assert.NoDirExists(t, filepath.Join(home, ".codex"), "a directory ai-rulez created is removed once empty")
	assert.NoDirExists(t, filepath.Join(home, ".claude", "agents"))
}

func TestUserClean_KeepsACreatedDirectoryTheUserFilled(t *testing.T) {
	quietWarnings(t)
	// Arrange
	home, gen := newUserHome(t, userConfigTOML, userFixture())
	_, err := gen.GenerateUser("")
	require.NoError(t, err)
	mine := filepath.Join(home, ".codex", "mine.md")
	require.NoError(t, os.WriteFile(mine, []byte("mine\n"), 0o644))

	// Act
	_, err = loadUserGenerator(t, home).Clean("", CleanOptions{KeepGitignore: true})

	// Assert
	require.NoError(t, err)
	assert.FileExists(t, mine)
	assert.NoDirExists(t, filepath.Join(home, ".cursor"))
}

func TestUserClean_IgnoresARecordedDirectoryThatClimbsOutOfTheHome(t *testing.T) {
	quietWarnings(t)
	// Arrange: a tampered manifest naming an empty directory elsewhere.
	home, gen := newUserHome(t, userConfigTOML, userFixture())
	_, err := gen.GenerateUser("")
	require.NoError(t, err)
	outside := filepath.Join(filepath.Dir(home), "outside-empty-"+filepath.Base(home))
	require.NoError(t, os.MkdirAll(outside, 0o755))
	t.Cleanup(func() { _ = os.RemoveAll(outside) })
	manifest := filepath.Join(home, ".config", "ai-rulez", generatedManifestName)
	data, err := os.ReadFile(manifest)
	require.NoError(t, err)
	tampered := string(data)[:len(data)-2] + ",\n  \"dirs\": [\"../" + filepath.Base(outside) + "\"]\n}\n"
	require.NoError(t, os.WriteFile(manifest, []byte(tampered), 0o644))

	// Act
	_, err = loadUserGenerator(t, home).Clean("", CleanOptions{KeepGitignore: true})

	// Assert
	require.NoError(t, err)
	assert.DirExists(t, outside)
}
