package importer

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

var localRootProject = map[string]string{
	"rulesync.jsonc":            `{"targets":["claudecode"]}`,
	".rulesync/rules/me.md":     "---\nlocalRoot: true\n---\nMy personal notes.\n",
	".rulesync/rules/shared.md": "---\nroot: false\ntargets: ['*']\n---\nShared rule.\n",
}

func TestConvert_LocalRootRuleGoesToTheLocalTree(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	testutil.WriteTree(t, dir, localRootProject)

	// Act
	report, err := Convert(context.Background(), ConvertOptions{Source: dir, Write: true})

	// Assert
	require.NoError(t, err)
	require.True(t, report.Written)
	got := snapshot(t, filepath.Join(dir, ".ai-rulez"))
	assert.Contains(t, got["local/context/me.md"], "My personal notes.")
	assert.Contains(t, got["rules/shared.md"], "Shared rule.")
	assert.NotContains(t, got, "context/me.md", "a personal rule must not land in the committed tree")
	assert.NotContains(t, got, "rules/me.md")
}

func TestConvert_LocalTreeIsOwnerOnlyAndGitignored(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes")
	}
	// Arrange
	dir := t.TempDir()
	testutil.WriteTree(t, dir, localRootProject)

	// Act
	_, err := Convert(context.Background(), ConvertOptions{Source: dir, Write: true})

	// Assert
	require.NoError(t, err)
	file, err := os.Stat(filepath.Join(dir, ".ai-rulez", "local", "context", "me.md"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), file.Mode().Perm())
	tree, err := os.Stat(filepath.Join(dir, ".ai-rulez", "local"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), tree.Mode().Perm())
	shared, err := os.Stat(filepath.Join(dir, ".ai-rulez", "rules", "shared.md"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), shared.Mode().Perm())
	ignore, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	require.NoError(t, err)
	assert.Contains(t, string(ignore), ".ai-rulez/local/")
}

func TestConvert_LocalTreeFollowsTheDomain(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	testutil.WriteTree(t, dir, localRootProject)

	// Act
	_, err := Convert(context.Background(), ConvertOptions{Source: dir, Write: true, Domain: "mine"})

	// Assert
	require.NoError(t, err)
	got := snapshot(t, filepath.Join(dir, ".ai-rulez"))
	assert.Contains(t, got, "local/domains/mine/context/me.md")
	assert.Contains(t, got, "domains/mine/rules/shared.md")
}

func TestConvert_NoLocalContentLeavesGitignoreAlone(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	testutil.WriteTree(t, dir, sampleProject)

	// Act
	_, err := Convert(context.Background(), ConvertOptions{Source: dir, Write: true})

	// Assert
	require.NoError(t, err)
	_, statErr := os.Stat(filepath.Join(dir, ".gitignore"))
	assert.True(t, os.IsNotExist(statErr))
}

func TestConvert_LocalRootSecretIsStillBlocked(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	testutil.WriteTree(t, dir, map[string]string{
		".rulesync/rules/me.md": "---\nlocalRoot: true\n---\nKey: ghp_abcdefghijklmnopqrstuvwxyz0123456789\n",
	})

	// Act
	report, err := Convert(context.Background(), ConvertOptions{Source: dir, Write: true})

	// Assert
	require.NoError(t, err)
	assert.True(t, report.Security.Blocked)
	assert.False(t, report.Written)
	_, statErr := os.Stat(filepath.Join(dir, ".ai-rulez"))
	assert.True(t, os.IsNotExist(statErr))
}
