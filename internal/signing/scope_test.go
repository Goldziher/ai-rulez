package signing_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/Goldziher/ai-rulez/v5/internal/signing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStateScope(t *testing.T) {
	tests := []struct {
		name    string
		markers []string
		config  string
		wantRel string
	}{
		{"root of a checkout", []string{".git"}, ".ai-rulez", ".ai-rulez"},
		{"monorepo service", []string{".git"}, "services/a/.ai-rulez", "services/a/.ai-rulez"},
		{"no checkout", nil, "proj/.ai-rulez", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			top := t.TempDir()
			for _, m := range tt.markers {
				require.NoError(t, os.MkdirAll(filepath.Join(top, m), 0o755))
			}
			if len(tt.markers) == 0 {
				// The case is the absence of a checkout: a repository enclosing the
				// temporary directory (a TMPDIR inside a checkout) would decide it.
				if rel, _ := StateScope(top); rel != "" {
					t.Skip("the temporary directory is inside a git checkout")
				}
			}
			cfg := filepath.Join(top, filepath.FromSlash(tt.config))
			require.NoError(t, os.MkdirAll(cfg, 0o755))

			// Act
			rel, abs := StateScope(cfg)

			// Assert
			assert.Equal(t, cfg, abs)
			if tt.wantRel == "" {
				assert.Empty(t, rel)
				return
			}
			assert.Equal(t, tt.wantRel, rel)
		})
	}
}
