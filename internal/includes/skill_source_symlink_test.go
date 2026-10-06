package includes

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestScanInstalledSkillDir_RefusesSymlinkedSkillMd(t *testing.T) {
	// Arrange
	secret := filepath.Join(t.TempDir(), "secret.txt")
	require.NoError(t, os.WriteFile(secret, []byte("TOP-SECRET"), 0o600))
	dir := t.TempDir()
	linkOrSkip(t, secret, filepath.Join(dir, skillMarkerFile))

	// Act
	file, err := ScanInstalledSkillDir(dir, "evil")

	// Assert
	require.Error(t, err)
	assert.NotContains(t, file.Content, "TOP-SECRET")
	assert.False(t, hasSkillMarker(dir))
}

func TestSkillGitSource_FindSkillDir_RefusesSymlinkedComponents(t *testing.T) {
	// Arrange
	cache := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, skillMarkerFile), []byte("# s\n"), 0o644))
	linkOrSkip(t, outside, filepath.Join(cache, "skills"))
	src := &SkillGitSource{cacheDir: cache, path: "skills"}

	// Act
	dir := src.findSkillDir()

	// Assert
	assert.Empty(t, dir)
}

func TestGetSkillCacheDir_KeyedByNameAndURL(t *testing.T) {
	// Arrange
	t.Setenv("HOME", t.TempDir())

	// Act
	a, errA := getSkillCacheDir("kreuzberg", "https://github.com/a/skills.git")
	b, errB := getSkillCacheDir("kreuzberg", "https://github.com/b/skills.git")
	a2, _ := getSkillCacheDir("kreuzberg", "https://github.com/a/skills.git/")

	// Assert
	require.NoError(t, errA)
	require.NoError(t, errB)
	assert.NotEqual(t, a, b)
	assert.Equal(t, a, a2)
}

func TestSwapDir_ReplacesWithoutLeavingOldTree(t *testing.T) {
	// Arrange
	root := t.TempDir()
	final := filepath.Join(root, "final")
	tmp := filepath.Join(root, "tmp")
	require.NoError(t, os.MkdirAll(final, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(final, "old"), nil, 0o644))
	require.NoError(t, os.MkdirAll(tmp, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "new"), nil, 0o644))

	// Act
	err := swapDir(tmp, final)

	// Assert
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(final, "new"))
	assert.NoFileExists(t, filepath.Join(final, "old"))
	entries, _ := os.ReadDir(root)
	assert.Len(t, entries, 1)
}

func TestSkillSources_NameASymlinkedSkillMdInTheError(t *testing.T) {
	// Arrange: SKILL.md is a symlink to a file that exists.
	root := t.TempDir()
	dir := filepath.Join(root, "skill")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	target := filepath.Join(root, "real.md")
	require.NoError(t, os.WriteFile(target, []byte("x"), 0o644))
	testutil.SymlinkOrSkip(t, target, filepath.Join(dir, "SKILL.md"))

	// Act
	link := symlinkedMarker(dir)

	// Assert
	assert.Equal(t, filepath.Join(dir, "SKILL.md"), link)
	assert.Empty(t, symlinkedMarker(root), "a missing SKILL.md is not a symlink")
}

func TestResolveInstalledSkill_LocalSymlinkedSkillMdErrorNamesTheLink(t *testing.T) {
	// Arrange
	base := t.TempDir()
	dir := filepath.Join(base, "skills", "s")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	target := filepath.Join(base, "real.md")
	require.NoError(t, os.WriteFile(target, []byte("x"), 0o644))
	testutil.SymlinkOrSkip(t, target, filepath.Join(dir, "SKILL.md"))
	cfg := &config.Config{BaseDir: base}
	conf := &config.InstalledSkillConfig{Name: "s", Source: ".", Path: "skills/s"}

	// Act
	_, err := resolveInstalledSkill(context.Background(), cfg, nil, conf, "")

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), filepath.Join(dir, "SKILL.md"))
	assert.Contains(t, err.Error(), "symlink")
	assert.NotContains(t, err.Error(), "no SKILL.md found")
}
