package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/includes"
)

// lockedProject is a project with a lock and generated outputs, whose rule is
// then edited and regenerated: the outputs match the sources, the lock does not.
func lockedProject(t *testing.T) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	includes.Mode, includes.SkipFetch = includes.LockAuto, false
	t.Cleanup(func() {
		includes.Mode, includes.SkipFetch, includes.RequireWhenEnforced = includes.LockAuto, false, false
	})
	root := lockProject(t, "")
	require.Equal(t, 0, writeLockAt("", "", nil), "lock")
	require.Equal(t, 0, runRecursiveGenerate(), "generate")
	writeFile(t, filepath.Join(root, ".ai-rulez", "rules", "style.md"), "# Style\nUse spaces.\n")
	require.Equal(t, 0, runRecursiveGenerate(), "regenerate after the edit")
	return root
}

func TestGenerateCheck_LockedFlagsRequireTheLockToMatch(t *testing.T) {
	tests := []struct {
		name      string
		locked    bool
		frozen    bool
		recursive bool
		want      int
	}{
		{"plain check verifies an enforced lock", false, false, false, exitDrift},
		{"locked", true, false, false, exitDrift},
		{"frozen", false, true, false, exitDrift},
		{"locked recursive", true, false, true, exitDrift},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			lockedProject(t)
			oldRecursive := recursive
			t.Cleanup(func() { recursive, generateLocked, generateFrozen = oldRecursive, false, false })
			recursive, generateLocked, generateFrozen = tt.recursive, tt.locked, tt.frozen

			// Act
			var got int
			_, _ = capture(t, func() { got = generateCheckCode(nil) })

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGenerateCheck_LockedPassesWhenTheLockMatches(t *testing.T) {
	root := lockedProject(t)
	require.Equal(t, 0, writeLockAt("", "", nil), "re-lock accepts the edit")
	require.NoError(t, os.Chdir(root))
	t.Cleanup(func() { generateLocked = false })
	generateLocked = true

	var got int
	_, _ = capture(t, func() { got = generateCheckCode(nil) })

	assert.Equal(t, 0, got)
}

func TestRecursiveGenerate_LockDriftExitsWithTheDriftCode(t *testing.T) {
	tests := []struct {
		name   string
		locked bool
		want   int
	}{
		{"unlocked generate ignores the lock", false, 0},
		{"locked generate reports drift", true, exitDrift},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lockedProject(t)
			t.Cleanup(func() { generateLocked = false })
			generateLocked = tt.locked

			var got int
			_, _ = capture(t, func() { got = runRecursiveGenerate() })

			assert.Equal(t, tt.want, got)
		})
	}
}
