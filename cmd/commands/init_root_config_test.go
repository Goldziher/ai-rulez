package commands

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInit_RefusesAProjectConfiguredByAiRulezToml(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantFolder bool
		wantBackup bool
	}{
		{"refuses without --force", []string{"init", "demo"}, 1, false, false},
		{"--minimal refuses without --force", []string{"init", "--minimal", "demo"}, 1, false, false},
		{"--force moves the file aside", []string{"init", "--force", "demo"}, 0, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			const original = "version = \"5.0\"\nname = \"mine\"\n"
			writeFile(t, filepath.Join(root, "ai-rulez.toml"), original)
			chdir(t, root)

			// Act
			res := runCLI(t, tt.args...)

			// Assert
			assert.Equal(t, tt.wantCode, res.code, res.stdout+res.stderr)
			if tt.wantCode != 0 {
				assert.Contains(t, res.stdout+res.stderr, "ai-rulez.toml")
				assert.Equal(t, original, readFileString(t, filepath.Join(root, "ai-rulez.toml")))
			}
			if tt.wantFolder {
				assert.DirExists(t, filepath.Join(root, ".ai-rulez"))
			} else {
				assert.NoDirExists(t, filepath.Join(root, ".ai-rulez"))
			}
			backups, err := filepath.Glob(filepath.Join(root, "ai-rulez.toml.bak-*"))
			require.NoError(t, err)
			assert.Equal(t, tt.wantBackup, len(backups) == 1)
		})
	}
}
