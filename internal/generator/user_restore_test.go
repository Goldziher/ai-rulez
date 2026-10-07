package generator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const userRestoreConfig = `version = "5.0"
name = "user"
presets = ["claude"]

[permissions]
deny = ["Bash(rm -rf *)"]
`

// TestUserClean_RestoresTheOriginalLayoutAfterRepeatedGenerates pins that what
// the first user-scope run learned about a settings file (no final newline, one
// line) survives later runs, so clean gives back the bytes the user wrote.
func TestUserClean_RestoresTheOriginalLayoutAfterRepeatedGenerates(t *testing.T) {
	quietWarnings(t)
	tests := []struct {
		name     string
		original string
		runs     int
	}{
		{"no final newline, two runs", `{"model":"opus"}`, 2},
		{"no final newline, four runs", `{"model":"opus"}`, 4},
		{"final newline, three runs", "{\"model\":\"opus\"}\n", 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			home, gen := newUserHome(t, userRestoreConfig, map[string]string{"rules/personal.md": "# P\nBe brief.\n"})
			settings := filepath.Join(home, ".claude", "settings.json")
			require.NoError(t, os.MkdirAll(filepath.Dir(settings), 0o755))
			require.NoError(t, os.WriteFile(settings, []byte(tt.original), 0o644))

			// Act
			_, err := gen.GenerateUser("")
			require.NoError(t, err)
			for i := 1; i < tt.runs; i++ {
				_, err = loadUserGenerator(t, home).GenerateUser("")
				require.NoError(t, err)
			}
			require.Contains(t, readFileString(t, settings), "deny")
			_, err = loadUserGenerator(t, home).Clean("", CleanOptions{KeepGitignore: true})
			require.NoError(t, err)

			// Assert
			assert.Equal(t, tt.original, readFileString(t, settings))
		})
	}
}
