package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// linkedCursorProject generates a claude+cursor project with rules a and b, then
// moves .cursor out of the project and leaves a symlink to it in its place.
func linkedCursorProject(t *testing.T) (dir string) {
	t.Helper()
	dir = filepath.Join(t.TempDir(), "p")
	rules := filepath.Join(dir, ".ai-rulez", "rules")
	require.NoError(t, os.MkdirAll(rules, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", "config.toml"),
		[]byte("version = \"5.0\"\nname = \"x\"\npresets = [\"claude\", \"cursor\"]\ngitignore = false\nagents_md = false\n"), 0o644))
	for _, name := range []string{"a", "b"} {
		writeRule(t, dir, name)
	}
	require.NoError(t, newProjectGenerator(t, dir).Generate("default"))
	outside := filepath.Join(filepath.Dir(dir), "outside-cursor")
	require.NoError(t, os.Rename(filepath.Join(dir, ".cursor"), outside))
	testutil.SymlinkOrSkip(t, outside, filepath.Join(dir, ".cursor"))
	return dir
}

func writeRule(t *testing.T, dir, name string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", "rules", name+".md"),
		[]byte("---\npriority: high\n---\n# "+name+"\n\nRule "+name+".\n"), 0o644))
}

func TestOutputDirSymlinkedOutOfTheProject(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, gen *Generator, dir string)
	}{
		{
			name: "check reports the outputs behind the link as blocked",
			run: func(t *testing.T, gen *Generator, _ string) {
				t.Helper()
				drift, err := gen.CheckDrift("default")
				require.NoError(t, err)
				var blocked []string
				for _, d := range drift {
					if d.Kind == DriftBlocked {
						blocked = append(blocked, d.Path)
					}
				}
				assert.Contains(t, blocked, ".cursor")
			},
		},
		{
			name: "dry run marks them blocked",
			run: func(t *testing.T, gen *Generator, _ string) {
				t.Helper()
				lines, err := gen.DryRun("default")
				require.NoError(t, err)
				assert.True(t, anyLinePrefix(lines, "blocked: .cursor"), lines)
			},
		},
		{
			name: "generate refuses before removing or writing anything",
			run: func(t *testing.T, gen *Generator, dir string) {
				t.Helper()
				err := gen.Generate("default")
				require.Error(t, err)
				assert.Contains(t, err.Error(), ".cursor")
				assert.FileExists(t, filepath.Join(dir, ".claude", "rules", "b.md"), "the stale rule is not removed")
				assert.NoFileExists(t, filepath.Join(dir, ".claude", "rules", "c.md"), "the new rule is not written")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quietWarnings(t)
			// Arrange
			dir := linkedCursorProject(t)
			require.NoError(t, os.Remove(filepath.Join(dir, ".ai-rulez", "rules", "b.md")))
			writeRule(t, dir, "c")
			gen := newProjectGenerator(t, dir)

			// Act + Assert
			tt.run(t, gen, dir)
		})
	}
}

func anyLinePrefix(lines []string, prefix string) bool {
	for _, l := range lines {
		if strings.HasPrefix(l, prefix) {
			return true
		}
	}
	return false
}
