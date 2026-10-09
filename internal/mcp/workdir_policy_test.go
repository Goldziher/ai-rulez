package mcp

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/safefs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The policy must hand the handler the path it checked, symlinks resolved: a
// link that is swapped to point outside the root after the check must not
// redirect the operation.
func TestDirPolicyConfinePassesTheResolvedPathOn(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	// Arrange
	root := t.TempDir()
	project := filepath.Join(root, "project")
	require.NoError(t, os.MkdirAll(filepath.Join(project, ".ai-rulez"), 0o750))
	outside := t.TempDir()
	link := filepath.Join(root, "link")
	require.NoError(t, os.Symlink(project, link))
	resolvedProject, err := filepath.EvalSymlinks(project)
	require.NoError(t, err)
	args := map[string]any{
		argWorkingDirectory: link,
		argConfigFile:       filepath.Join(link, ".ai-rulez", "config.toml"),
		argConfigDir:        "../link/.ai-rulez",
	}

	// Act
	require.NoError(t, dirPolicy{root: root}.confine(args))
	require.NoError(t, os.Remove(link))
	require.NoError(t, os.Symlink(outside, link))

	// Assert
	assert.Equal(t, resolvedProject, args[argWorkingDirectory])
	assert.Equal(t, filepath.Join(resolvedProject, ".ai-rulez", "config.toml"), args[argConfigFile])
	assert.Equal(t, filepath.Join(resolvedProject, ".ai-rulez"), args[argConfigDir])
	resolvedRoot, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)
	for _, name := range []string{argWorkingDirectory, argConfigFile, argConfigDir} {
		assert.True(t, safefs.Within(resolvedRoot, resolveExisting(args[name].(string))), "%s escaped after the swap: %v", name, args[name])
	}
}

func TestDirPolicyConfineLeavesUnsetArgumentsUnset(t *testing.T) {
	// Arrange
	root := t.TempDir()
	args := map[string]any{}

	// Act
	require.NoError(t, dirPolicy{root: root}.confine(args))

	// Assert
	resolvedRoot, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)
	assert.Equal(t, resolvedRoot, args[argWorkingDirectory])
	for _, name := range []string{argConfigFile, argConfigDir, argBundle} {
		assert.NotContains(t, args, name)
	}
}
