package signing

import (
	"os"
	"path/filepath"
	"testing"

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
			cfg := filepath.Join(top, filepath.FromSlash(tt.config))
			require.NoError(t, os.MkdirAll(cfg, 0o755))

			// Act
			rel, abs := stateScope(cfg)

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
