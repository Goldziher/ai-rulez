package lint

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
)

func writeTreeFiles(t *testing.T, dir string, rels ...string) {
	t.Helper()
	for _, rel := range rels {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o600))
	}
}

func TestLoadTreeLimited(t *testing.T) {
	tests := []struct {
		name      string
		files     []string
		limits    WalkLimits
		wantErr   string
		wantFiles []string
	}{
		{"within limits", []string{"AGENTS.md", "a/b.md"}, WalkLimits{MaxEntries: 10, MaxDepth: 3}, "", []string{"AGENTS.md", "a/b.md"}},
		{"too many entries", []string{"1.md", "2.md", "3.md", "4.md", "5.md"}, WalkLimits{MaxEntries: 3}, "more than 3 entries", nil},
		{"deeper directories are not walked", []string{"AGENTS.md", "a/b/c/d/deep.md", "a/ok.md"}, WalkLimits{MaxDepth: 2}, "", []string{"AGENTS.md", "a/ok.md"}},
		{"zero is unbounded", []string{"1.md", "2.md", "a/b/c/d/e.md"}, WalkLimits{}, "", []string{"1.md", "2.md", "a/b/c/d/e.md"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			writeTreeFiles(t, dir, tc.files...)

			// Act
			tree, err := LoadTreeLimited(context.Background(), gitutil.Git{}, dir, tc.limits)

			// Assert
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.ElementsMatch(t, tc.wantFiles, tree.Paths())
		})
	}
}
