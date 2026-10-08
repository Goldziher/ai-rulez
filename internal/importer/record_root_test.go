package importer

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCollectImported_StaysInsideTheProject pins the gosec G122 fix: the convert
// record reads imported files through an os.Root on the project, so no symlink,
// however it got there, makes it read a file outside the project.
func TestCollectImported_StaysInsideTheProject(t *testing.T) {
	tests := []struct {
		name string
		link string // project-relative symlink to the outside secret
		rels []string
		want []string
	}{
		{"regular files and directories", "", []string{"CLAUDE.md", ".claude/rules"}, []string{".claude/rules/a.md", "CLAUDE.md"}},
		{"a symlinked file below a directory", ".claude/rules/leak.md", []string{".claude/rules"}, []string{".claude/rules/a.md"}},
		{"a symlinked top-level source", "AGENTS.md", []string{"AGENTS.md", "CLAUDE.md"}, []string{"CLAUDE.md"}},
		{"a symlinked directory", ".claude/skills", []string{".claude/skills"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			outside := t.TempDir()
			secret := filepath.Join(outside, "secret.md")
			require.NoError(t, os.WriteFile(secret, []byte("secret\n"), 0o600))
			project := t.TempDir()
			testutil.WriteTree(t, project, map[string]string{"CLAUDE.md": "root\n", ".claude/rules/a.md": "rule\n"})
			if tt.link != "" {
				target := secret
				if tt.link == ".claude/skills" {
					target = outside
				}
				testutil.SymlinkOrSkip(t, target, filepath.Join(project, filepath.FromSlash(tt.link)))
			}

			// Act
			files, err := collectImported(project, tt.rels)

			// Assert
			require.NoError(t, err)
			got := make([]string, 0, len(files))
			for rel, data := range files {
				got = append(got, rel)
				assert.NotEqual(t, "secret\n", string(data), rel)
			}
			sort.Strings(got)
			if tt.want == nil {
				tt.want = []string{}
			}
			assert.Equal(t, tt.want, got)
		})
	}
}
