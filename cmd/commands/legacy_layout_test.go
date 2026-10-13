package commands

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLegacyLayoutApplies(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		dir   string
		want  bool
	}{
		{
			name:  "pre-OKF .ai-rulez tree",
			files: map[string]string{".ai-rulez/config.toml": "version = \"5.0\"\n", ".ai-rulez/rules/a.md": "A\n"},
			dir:   ".ai-rulez",
			want:  true,
		},
		{
			name:  "source = agents next to unread .ai-rulez content",
			files: map[string]string{".ai-rulez/config.toml": "version = \"5.0\"\nsource = \"agents\"\n", ".ai-rulez/rules/a.md": "A\n"},
			dir:   ".ai-rulez",
			want:  false,
		},
		{
			name:  "config in .agents with content there",
			files: map[string]string{".agents/ai-rulez.toml": "version = \"5.0\"\n", ".agents/rules/a.md": "A\n"},
			dir:   ".agents/.ai-rulez",
			want:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			for rel, content := range tt.files {
				writeFile(t, filepath.Join(root, filepath.FromSlash(rel)), content)
			}

			// Act
			got := legacyLayoutApplies(filepath.Join(root, filepath.FromSlash(tt.dir)))

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMigrateOKFRefusesAnAgentsProject(t *testing.T) {
	// Arrange
	root := okfProject(t)
	writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), "version = \"5.0\"\nname = \"x\"\nsource = \"agents\"\npresets = [\"claude\"]\n")
	writeFile(t, filepath.Join(root, ".agents", "rules", "a.md"), "A\n")

	// Act
	code, _ := migrateOKFRun(t, false, false)

	// Assert
	assert.Equal(t, 1, code)
	assert.NoFileExists(t, filepath.Join(root, ".ai-rulez", "index.md"))
	assert.NoFileExists(t, filepath.Join(root, ".agents", "index.md"))
}
