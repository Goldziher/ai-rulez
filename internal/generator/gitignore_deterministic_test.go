package generator

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

func sortedPatterns(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// The managed block must not depend on which machine-local files exist or on
// whether the run loaded them (#499).
func TestCollectGitignorePaths_IsIndependentOfLocalState(t *testing.T) {
	tests := []struct {
		name       string
		withLocal  bool
		loaded     bool
		skipped    bool
		manifestOn bool
	}{
		{name: "no local files"},
		{name: "overlay on disk, loaded", withLocal: true, loaded: true},
		{name: "overlay on disk, skipped (--no-local)", withLocal: true, skipped: true},
		{name: "local manifest on disk", manifestOn: true},
	}
	var reference []string
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := localOnDisk(t, tt.withLocal)
			if tt.loaded {
				cfg.LocalOverlay = &config.LocalOverlay{}
			}
			if tt.manifestOn {
				require.NoError(t, os.WriteFile(filepath.Join(cfg.ConfigDir, ".generated-manifest.local.json"), []byte("{}"), 0o600))
			}
			gen := NewGenerator(cfg)
			gen.localSkipped = tt.skipped

			// Act
			got := sortedPatterns(gen.collectGitignorePaths(nil))

			// Assert
			assert.Contains(t, got, ".ai-rulez/config.local.*")
			assert.Contains(t, got, ".ai-rulez/.config.local.*")
			assert.Contains(t, got, ".ai-rulez/local/")
			assert.Contains(t, got, ".ai-rulez/.generated-manifest.local.json")
			if i == 0 {
				reference = got
			}
			assert.Equal(t, reference, got)
		})
	}
}

func TestGitignoreSinks_OneSinkPerToplevel(t *testing.T) {
	// Arrange
	sinks := NewGitignoreSinks()
	a, b := t.TempDir(), t.TempDir()

	// Act
	first, again, other := sinks.For(a), sinks.For(a+"/"), sinks.For(b)

	// Assert
	assert.Same(t, first, again)
	assert.NotSame(t, first, other)
	assert.Equal(t, filepath.Clean(a), first.TopLevel())
	assert.Nil(t, sinks.For(""))
	assert.Nil(t, (*GitignoreSinks)(nil).For(a))
}

func TestGitignoreSinks_BlocksStayInTheirOwnRepository(t *testing.T) {
	// Arrange
	parent, child := t.TempDir(), t.TempDir()
	sinks := NewGitignoreSinks()
	log := logger.Discard()

	// Act
	require.NoError(t, sinks.For(child).Add(log, "", []string{"CLAUDE.md"}))
	sinks.For(parent) // a parent with gitignore = false adds nothing

	// Assert
	assert.FileExists(t, filepath.Join(child, ".gitignore"))
	assert.NoFileExists(t, filepath.Join(parent, ".gitignore"))
}
