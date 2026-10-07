package policy

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// A configuration loaded with a relative directory carries relative content
// paths; the content still came from an include and is still bounded.
func TestApplyContent_RelativePathsAreStillImported(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		includes []config.IncludeConfig
		want     int
	}{
		{"a local include", "shared/.ai-rulez/agents/shared.md", []config.IncludeConfig{{Name: "shared", Source: "./shared"}}, 1},
		{"a path outside the config directory", "../cache/agents/shared.md", nil, 1},
		{"the project's own content", ".ai-rulez/agents/own.md", nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			t.Chdir(t.TempDir())
			agent := contentFile(t, "shared", tt.path, hookFrontmatter)
			cfg := &config.Config{
				BaseDir: ".", ConfigDir: ".ai-rulez", Includes: tt.includes,
				Content: &config.ContentTree{Agents: []config.ContentFile{agent}},
			}
			res := Resolve([]Layer{layer("managed", Policy{Hooks: Hooks{Forbidden: true}})})

			// Act
			got := res.ApplyContent(cfg)

			// Assert
			assert.Len(t, got, tt.want)
		})
	}
}
