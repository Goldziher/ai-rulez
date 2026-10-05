package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pruneProject generates the root plugin bundle of a project with runtimes
// claude, gemini and cursor, then returns the project dir and the bundle files of the
// skill core-s that a later removal makes obsolete.
func pruneProject(t *testing.T) (dir string, obsolete []string) {
	t.Helper()
	dir = newDomainsProject(t, "")
	cfgPath := filepath.Join(dir, ".ai-rulez", "config.toml")
	body, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	updated := strings.Replace(string(body), `runtimes = ["claude"]`, `runtimes = ["claude", "gemini", "cursor"]`, 1)
	require.NoError(t, os.WriteFile(cfgPath, []byte(updated), 0o600))
	require.NoError(t, loadDomainsProject(t, dir).GeneratePlugin(""))

	for _, rel := range []string{"skills/core-s/SKILL.md"} {
		require.FileExists(t, filepath.Join(dir, filepath.FromSlash(rel)))
		obsolete = append(obsolete, rel)
	}
	return dir, obsolete
}

func removeCoreSkill(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.RemoveAll(filepath.Join(dir, ".ai-rulez", "skills", "core-s")))
}

func TestGeneratePlugin_PrunesRemovedSkill(t *testing.T) {
	dir, obsolete := pruneProject(t)
	removeCoreSkill(t, dir)

	require.NoError(t, loadDomainsProject(t, dir).GeneratePlugin(""))

	for _, rel := range obsolete {
		assert.NoFileExists(t, filepath.Join(dir, filepath.FromSlash(rel)))
	}
	assert.NoDirExists(t, filepath.Join(dir, "skills", "core-s"), "the emptied directory is pruned")
	assert.DirExists(t, filepath.Join(dir, "skills"), "a directory that still holds files stays")
	assert.FileExists(t, filepath.Join(dir, "skills", "niche-s", "SKILL.md"))
	require.NoError(t, loadDomainsProject(t, dir).VerifyPlugin(""))
}

func TestGeneratePlugin_PrunesEveryRuntime(t *testing.T) {
	dir, _ := pruneProject(t)
	// Every generated file that belongs to core-s, whatever runtime wrote it.
	var owned []string
	require.NoError(t, filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.Contains(filepath.ToSlash(path), "core-s") &&
			!strings.Contains(filepath.ToSlash(path), "/.ai-rulez/") {
			owned = append(owned, path)
		}
		return err
	}))
	require.NotEmpty(t, owned)
	removeCoreSkill(t, dir)

	require.NoError(t, loadDomainsProject(t, dir).GeneratePlugin(""))
	for _, path := range owned {
		assert.NoFileExists(t, path)
	}
}

func TestGeneratePlugin_KeepsEditedObsoleteFileAndWarns(t *testing.T) {
	dir, _ := pruneProject(t)
	edited := filepath.Join(dir, "skills", "core-s", "SKILL.md")
	require.NoError(t, os.WriteFile(edited, []byte("hand edited\n"), 0o600))
	removeCoreSkill(t, dir)
	gen := loadDomainsProject(t, dir)

	report, err := gen.PluginPrunePlan("")
	require.NoError(t, err)
	assert.Equal(t, []string{"skills/core-s/SKILL.md"}, report.Kept)
	assert.Equal(t, []string{".cursor-plugin/skills/core-s/SKILL.md"}, report.Pruned, "an untouched copy is still pruned")

	require.NoError(t, gen.GeneratePlugin(""))
	assert.FileExists(t, edited, "an edited generated file is never deleted")
	got, err := os.ReadFile(edited)
	require.NoError(t, err)
	assert.Equal(t, "hand edited\n", string(got))
}

func TestGeneratePlugin_KeepsUnknownFiles(t *testing.T) {
	dir, _ := pruneProject(t)
	mine := filepath.Join(dir, "skills", "core-s", "notes.md")
	writeDomainsFile(t, mine, "mine")
	handSkill := filepath.Join(dir, "skills", "hand-made", "SKILL.md")
	writeDomainsFile(t, handSkill, "mine")
	removeCoreSkill(t, dir)

	require.NoError(t, loadDomainsProject(t, dir).GeneratePlugin(""))
	assert.FileExists(t, mine, "a file the sidecar never listed is kept")
	assert.FileExists(t, handSkill)
	assert.NoFileExists(t, filepath.Join(dir, "skills", "core-s", "SKILL.md"))
	assert.DirExists(t, filepath.Join(dir, "skills", "core-s"), "a directory holding a kept file stays")
}

func TestDryRunPlugin_ListsObsoleteFilesWithoutDeleting(t *testing.T) {
	dir, _ := pruneProject(t)
	removeCoreSkill(t, dir)
	gen := loadDomainsProject(t, dir)

	lines, err := gen.DryRunPlugin("")
	require.NoError(t, err)
	assert.Contains(t, lines, "delete-stale: skills/core-s/SKILL.md")
	assert.FileExists(t, filepath.Join(dir, "skills", "core-s", "SKILL.md"))
}

func TestVerifyPlugin_FailsOnObsoleteGeneratedFile(t *testing.T) {
	dir, _ := pruneProject(t)
	removeCoreSkill(t, dir)

	err := loadDomainsProject(t, dir).VerifyPlugin("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "obsolete generated plugin file")
	assert.Contains(t, err.Error(), "skills/core-s/SKILL.md")
}

func TestGeneratePlugin_DoesNotPruneThroughSymlinkOutOfProject(t *testing.T) {
	dir, _ := pruneProject(t)
	outside := t.TempDir()
	// Replace the skill directory with a link leaving the project.
	require.NoError(t, os.Rename(filepath.Join(dir, "skills", "core-s", "SKILL.md"), filepath.Join(outside, "SKILL.md")))
	require.NoError(t, os.RemoveAll(filepath.Join(dir, "skills", "core-s")))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "skills", "core-s")))
	removeCoreSkill(t, dir)

	require.NoError(t, loadDomainsProject(t, dir).GeneratePlugin(""))
	assert.FileExists(t, filepath.Join(outside, "SKILL.md"), "nothing outside the project is deleted")
}
