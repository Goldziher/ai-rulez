package generator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// After `convert --write` the user's own CLAUDE.md, agents and skills sit at the
// paths a generate would write, but ai-rulez has written nothing yet: clean must
// not treat them as generated.
func TestGenerator_Clean_KeepsHandWrittenFilesBeforeFirstGenerate(t *testing.T) {
	// Arrange
	tempDir := t.TempDir()
	copyFixture(t, filepath.Join("..", "..", "tests", "fixtures", "config", "generator", "basic"), tempDir)
	cfg, err := config.LoadConfig(context.Background(), tempDir)
	require.NoError(t, err)
	gen := NewGenerator(cfg)

	// Learn which paths a generate writes, then replace them with hand-written files.
	require.NoError(t, gen.Generate("default"))
	var written []string
	err = filepath.WalkDir(tempDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, _ := filepath.Rel(tempDir, path)
		if d.IsDir() && (rel == ".ai-rulez" || rel == ".git") {
			return filepath.SkipDir
		}
		if !d.IsDir() {
			written = append(written, path)
		}
		return nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, written)
	for _, path := range written {
		require.NoError(t, os.WriteFile(path, []byte("my own hand-written notes\n"), 0o644))
	}
	require.NoError(t, os.Remove(filepath.Join(tempDir, ".ai-rulez", generatedManifestName)))
	if local := gen.localManifestPath(); local != "" {
		_ = os.Remove(local)
	}
	cfg2, err := config.LoadConfig(context.Background(), tempDir)
	require.NoError(t, err)

	// Act
	plan, err := NewGenerator(cfg2).Clean("default", CleanOptions{RemoveEdited: true})
	require.NoError(t, err)

	// Assert
	assert.Empty(t, plan.Files, "nothing ai-rulez wrote may be removed")
	for _, path := range written {
		assert.FileExists(t, path)
	}
}
