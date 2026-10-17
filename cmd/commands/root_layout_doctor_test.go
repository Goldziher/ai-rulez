package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// doctor reads the project offline, so it never resolves an installed skill. A
// local skill that the lock pins must not look like a source that was removed.
func TestDoctor_LockedLocalInstalledSkillIsNotDrift(t *testing.T) {
	const installed = "\n[[installed_skills]]\nname = \"demo\"\nsource = \"./shared\"\n"
	tests := []struct {
		name  string
		setup func(t *testing.T, root string)
	}{
		{"root layout", func(t *testing.T, root string) {
			writeFile(t, filepath.Join(root, "ai-rulez.toml"), rootLayoutConfig+installed)
		}},
		{"directory layout", func(t *testing.T, root string) {
			writeFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), rootLayoutConfig+"gitignore = false\n"+installed)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := rootLayoutProject(t, rootLayoutConfig)
			require.NoError(t, os.Remove(filepath.Join(root, "ai-rulez.toml")))
			tt.setup(t, root)
			require.Equal(t, 0, runCLI(t, "generate", "--yes").code)
			require.Equal(t, 0, runCLI(t, "lock").code)

			// Act
			res := runCLI(t, "doctor")

			// Assert
			assert.Equal(t, 0, res.code, res.stdout+res.stderr)
			assert.NotContains(t, res.stdout, "authored content differs")
			assert.Equal(t, 0, runCLI(t, "lock", "--check").code, "lock --check still agrees")
		})
	}
}
