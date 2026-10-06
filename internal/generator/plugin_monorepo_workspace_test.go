package generator

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
)

// Marketplace members are read through the workspace the root was loaded from: a
// monorepo held in memory has no directory on the disk of this process.
func TestCollectPluginOutputs_MonorepoMembersReadThroughTheWorkspace(t *testing.T) {
	// Arrange
	const root = "/virtual/mono"
	mem := workspace.NewMem(root)
	fixture := filepath.Join("..", "..", "tests", "fixtures", "plugin", "monorepo")
	require.NoError(t, filepath.WalkDir(fixture, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		rel, relErr := filepath.Rel(fixture, path)
		require.NoError(t, relErr)
		mem.Set(filepath.ToSlash(rel), string(data), 0o644)
		return nil
	}))
	cfg, err := config.LoadConfig(t.Context(), root, config.WithWorkspace(mem), config.WithoutRemote())
	require.NoError(t, err)

	// Act
	outputs, err := NewGenerator(cfg).collectPluginOutputs("")

	// Assert
	require.NoError(t, err)
	var paths []string
	for _, o := range outputs {
		paths = append(paths, filepath.ToSlash(o.Path))
	}
	assert.Contains(t, paths, root+"/plugins/alpha/.claude-plugin/plugin.json")
	assert.Contains(t, paths, root+"/plugins/beta/.claude-plugin/plugin.json")
}
