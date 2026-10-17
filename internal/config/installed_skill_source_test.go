package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeInstalledSkillSources(t *testing.T) {
	// Arrange: vendor/skills exists as a directory under the project.
	base := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(base, "vendor", "skills"), 0o755))

	tests := []struct {
		name   string
		source string
		want   string
	}{
		{"github prefix", "github:anthropics/skills", "https://github.com/anthropics/skills"},
		{"bare shorthand", "anthropics/skills", "https://github.com/anthropics/skills"},
		{"bare shorthand naming an existing directory stays local", "vendor/skills", "vendor/skills"},
		{"full https url", "https://github.com/anthropics/skills", "https://github.com/anthropics/skills"},
		{"scp url", "git@github.com:anthropics/skills.git", "git@github.com:anthropics/skills.git"},
		{"dot-relative path", "./vendor/skills", "./vendor/skills"},
		{"absolute path", "/srv/skills", "/srv/skills"},
		{"home path", "~/skills", "~/skills"},
		{"file url", "file:///srv/skills", "file:///srv/skills"},
		{"malformed github prefix is left for validation", "github:nope", "github:nope"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			skills := []InstalledSkillConfig{{Name: "s", Source: tt.source}}

			// Act
			NormalizeInstalledSkillSources(base, skills)

			// Assert
			assert.Equal(t, tt.want, skills[0].Source)
		})
	}
}
