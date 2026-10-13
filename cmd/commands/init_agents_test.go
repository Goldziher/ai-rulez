package commands

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAgentsInitConflicts(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		inAgents bool
		want     []string
	}{
		{"empty project", nil, true, nil},
		{
			"config.toml next to the new .agents config",
			map[string]string{".ai-rulez/config.toml": "x"},
			true,
			[]string{".ai-rulez/config.toml is ignored once .agents/ai-rulez.toml exists: it takes precedence"},
		},
		{
			"alternate config directory",
			map[string]string{".config/ai-rulez/config.toml": "x"},
			true,
			[]string{".config/ai-rulez/config.toml is ignored once .agents/ai-rulez.toml exists: it takes precedence"},
		},
		{
			"the new config.toml is shadowed by an existing .agents config",
			map[string]string{".agents/ai-rulez.toml": "x"},
			false,
			[]string{".agents/ai-rulez.toml takes precedence over the config.toml this writes, which will be ignored"},
		},
		{"config.toml requested with no .agents config", map[string]string{".ai-rulez/rules/a.md": "x"}, false, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			for rel, content := range tt.files {
				writeFile(t, filepath.Join(root, filepath.FromSlash(rel)), content)
			}

			// Act
			got := agentsInitConflicts(root, tt.inAgents)

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}
