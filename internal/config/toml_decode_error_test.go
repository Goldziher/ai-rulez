package config

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig_StructuralTOMLErrorNamesKeyAndLine(t *testing.T) {
	tests := []struct {
		name     string
		toml     string
		contains []string
	}{
		{
			name:     "skills array of tables",
			toml:     "version = \"5.0\"\nname = \"x\"\n\n[[skills]]\nname = \"a\"\npath = \"p\"\n",
			contains: []string{"[[skills]]", "line 4", "skill_sources", "installed_skills", ".ai-rulez/skills"},
		},
		{
			name:     "string where a table belongs",
			toml:     "version = \"5.0\"\nname = \"x\"\nheader = \"oops\"\n",
			contains: []string{"header", "line 3"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			base := t.TempDir()
			writeProjectFile(t, filepath.Join(base, ".ai-rulez"), "config.toml", tt.toml)

			// Act
			_, err := LoadConfig(context.Background(), base)

			// Assert
			require.Error(t, err)
			for _, want := range tt.contains {
				assert.Contains(t, err.Error(), want)
			}
		})
	}
}
