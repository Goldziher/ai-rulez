package presets

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/generator/jsonmerge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMergedDocumentPaths pins the registry the generator's stale-deletion guard
// reads. A path that drops out of here stops being protected, which is how a
// hand-authored settings file gets deleted (#185).
func TestMergedDocumentPaths(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []string{
		".agents/settings.json",
		".gemini/settings.json",
		".mcp.json",
	}, MergedDocumentPaths())
}

// TestApplyMergedDocumentRejectsUnregisteredPath covers the drift guard. A preset
// that starts merging a new document without registering it would generate
// correctly and then have that document deleted on the first run whose gate drops
// it, so the mismatch has to surface at generate time instead.
func TestApplyMergedDocumentRejectsUnregisteredPath(t *testing.T) {
	t.Parallel()

	t.Run("an unregistered path is an error", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), ".newtool", "settings.json")
		_, err := applyMergedDocument(path, []jsonmerge.OwnedKey{{Name: keyMCPServers, Value: map[string]any{}}})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "merged JSON document path is not registered")
		assert.NoFileExists(t, path, "a rejected path must not be written")
	})

	t.Run("an empty path renders a fresh document", func(t *testing.T) {
		t.Parallel()

		result, err := applyMergedDocument("", []jsonmerge.OwnedKey{{Name: keyMCPServers, Value: map[string]any{}}})

		require.NoError(t, err, "an empty path names no file, so the registry does not apply")
		assert.Contains(t, result.Body, keyMCPServers)
	})

	t.Run("every registered path is accepted", func(t *testing.T) {
		t.Parallel()

		baseDir := t.TempDir()
		for _, relPath := range MergedDocumentPaths() {
			path := filepath.Join(baseDir, filepath.FromSlash(relPath))
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))

			result, err := applyMergedDocument(path,
				[]jsonmerge.OwnedKey{{Name: keyMCPServers, Value: map[string]any{}}})

			require.NoError(t, err, "%s is registered and must be accepted", relPath)
			assert.Contains(t, result.Body, keyMCPServers)
		}
	})
}
