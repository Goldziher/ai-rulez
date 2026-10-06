package includes

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func linkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
}

func TestLocalSource_NeverFollowsSymlinks(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T, inc, outside string)
		wantRules int
	}{
		{
			name: "symlinked .ai-rulez directory is skipped",
			setup: func(t *testing.T, inc, outside string) {
				require.NoError(t, os.MkdirAll(filepath.Join(outside, "rules"), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(outside, "rules", "a.md"), []byte("# a\n"), 0o644))
				linkOrSkip(t, outside, filepath.Join(inc, ".ai-rulez"))
			},
			wantRules: 0,
		},
		{
			name: "symlinked rules directory is skipped",
			setup: func(t *testing.T, inc, outside string) {
				require.NoError(t, os.MkdirAll(filepath.Join(outside, "rules"), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(outside, "rules", "a.md"), []byte("# a\n"), 0o644))
				require.NoError(t, os.MkdirAll(filepath.Join(inc, ".ai-rulez"), 0o755))
				linkOrSkip(t, filepath.Join(outside, "rules"), filepath.Join(inc, ".ai-rulez", "rules"))
			},
			wantRules: 0,
		},
		{
			name: "bare structure is scanned in place",
			setup: func(t *testing.T, inc, _ string) {
				require.NoError(t, os.MkdirAll(filepath.Join(inc, "rules"), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(inc, "rules", "a.md"), []byte("# a\n"), 0o644))
			},
			wantRules: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			inc, outside := t.TempDir(), t.TempDir()
			tt.setup(t, inc, outside)

			// Act
			tree, err := NewLocalSource("x", inc, t.TempDir(), nil).Fetch(context.Background())

			// Assert
			require.NoError(t, err)
			assert.Len(t, tree.Rules, tt.wantRules)
		})
	}
}
