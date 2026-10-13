package commands

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/progress"
)

func TestRunDriftCheck_RecursiveManagedGitignoreBlock(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	tests := []struct {
		name   string
		mutate func(t *testing.T, path string)
		want   int
	}{
		{name: "in sync after generate", mutate: func(*testing.T, string) {}, want: 0},
		{
			name:   "block emptied",
			mutate: func(t *testing.T, path string) { require.NoError(t, os.WriteFile(path, nil, 0o644)) },
			want:   exitDrift,
		},
		{
			name: "a nested root's line deleted from the block",
			mutate: func(t *testing.T, path string) {
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				require.True(t, strings.Contains(string(data), "b/CLAUDE.md\n"), string(data))
				require.NoError(t, os.WriteFile(path, []byte(strings.Replace(string(data), "b/CLAUDE.md\n", "", 1)), 0o644))
			},
			want: exitDrift,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			require.NoError(t, gitutil.CommandNoContext(root, "init", "-q").Run())
			for _, dir := range []string{"a", "b"} {
				writeFile(t, filepath.Join(root, dir, ".ai-rulez", "config.toml"),
					"version = \"5.0\"\nname = \""+dir+"\"\npresets = [\"claude\"]\ngitignore = true\nagents_md = false\n")
			}
			chdir(t, root)
			progress.SetQuiet(true)
			t.Cleanup(func() {
				progress.SetQuiet(false)
				pluginMode, dryRun, profile, recursive, updateGitignore = false, false, "", false, false
			})
			pluginMode, dryRun, profile, recursive, updateGitignore = false, false, "", true, false
			require.Equal(t, 0, runRecursiveGenerate())
			tt.mutate(t, filepath.Join(root, ".gitignore"))

			// Act
			got := runDriftCheck(true)

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}
