package generator

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestGitignore_LocalOverlayIgnoredUnconditionally(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *config.Config
		pattern string
		want    bool
	}{
		{"overlay present", &config.Config{BaseDir: t.TempDir(), LocalOverlay: &config.LocalOverlay{}}, ".ai-rulez/config.local.*", true},
		{"overlay in .config/ai-rulez layout", &config.Config{
			BaseDir: t.TempDir(), ConfigDirName: ".config/ai-rulez", LocalOverlay: &config.LocalOverlay{},
		}, ".config/ai-rulez/config.local.*", true},
		{"no local inputs", &config.Config{BaseDir: t.TempDir()}, ".ai-rulez/config.local.*", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			gen := NewGenerator(tt.cfg)

			// Act
			patterns := gen.collectGitignorePaths(nil)

			// Assert
			assert.Equal(t, tt.want, patterns[tt.pattern])
		})
	}
}
