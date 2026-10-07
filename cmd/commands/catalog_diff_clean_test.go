package commands

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
)

// A revision is extracted without the rest of the repository, so its lint used
// to skip the repository-path checks the working tree runs (AR401 on a backticked
// src/ path), and `catalog diff HEAD` on an unchanged tree reported a lint change.
func TestCatalogDiffCleanTreeIsIdentical(t *testing.T) {
	tests := []struct {
		name    string
		context string
	}{
		{"no repository paths", "# Notes\n\nNothing to check.\n"},
		{"a repository path that is missing", "# Layout\n\nCaches live in `src/caches.rs`.\n"},
		{"a repository path that exists", "# Layout\n\nThe entry point is `src/main.rs`.\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := rolesCmdProject(t)
			t.Setenv("HOME", t.TempDir())
			writeFile(t, filepath.Join(root, "src", "main.rs"), "fn main() {}\n")
			writeFile(t, filepath.Join(root, ".ai-rulez", "context", "layout.md"), tt.context)
			gitIn(t, root, "init", "-q", "-b", "main")
			gitIn(t, root, "add", "-A")
			gitIn(t, root, "commit", "-q", "-m", "one")
			resetCatalogDiffFlags(t)
			var out bytes.Buffer

			// Act
			identical, err := runCatalogDiff(context.Background(), &out, []string{"HEAD"})

			// Assert
			require.NoError(t, err)
			assert.True(t, identical, out.String())
		})
	}
}

// The plugin version drift check compares with outputs generated on disk, which
// an extracted revision lacks, so catalog diff leaves it out on both sides.
func TestBuildDiffCatalog_SkipsPluginDrift(t *testing.T) {
	tests := []struct {
		name string
		skip bool
		want int
	}{
		{"strict lint checks plugin drift", false, 1},
		{"catalog diff does not", true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := &config.Config{Plugin: &config.PluginAuthoring{}}
			skipPluginDrift = tt.skip
			t.Cleanup(func() { skipPluginDrift = false })

			// Act
			opts := governanceLintOptions(cfg, []string{lint.AnalyzerPlugin})

			// Assert
			assert.Len(t, opts, tt.want)
		})
	}
}
