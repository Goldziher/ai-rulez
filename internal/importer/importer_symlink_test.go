package importer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const symlinkSecret = "TOP-SECRET-OUTSIDE-CONTENT"

func writeSymlinkTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	}
}

func symlinkOrSkip(t *testing.T, target, name string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(name), 0o755))
	if err := os.Symlink(target, name); err != nil {
		t.Skip("symlinks unavailable")
	}
}

func TestImportDoesNotFollowSymlinks(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, project, outside string)
	}{
		{"symlinked CLAUDE.md", func(t *testing.T, p, o string) {
			symlinkOrSkip(t, filepath.Join(o, "secret.md"), filepath.Join(p, "CLAUDE.md"))
		}},
		{"symlinked skills directory", func(t *testing.T, p, o string) {
			symlinkOrSkip(t, filepath.Join(o, "skills"), filepath.Join(p, ".claude", "skills"))
		}},
		{"symlinked SKILL.md", func(t *testing.T, p, o string) {
			symlinkOrSkip(t, filepath.Join(o, "secret.md"), filepath.Join(p, ".claude", "skills", "x", "SKILL.md"))
		}},
		{"symlinked agent file", func(t *testing.T, p, o string) {
			symlinkOrSkip(t, filepath.Join(o, "secret.md"), filepath.Join(p, ".claude", "agents", "a.md"))
		}},
		{"symlinked cursor rule", func(t *testing.T, p, o string) {
			symlinkOrSkip(t, filepath.Join(o, "secret.md"), filepath.Join(p, ".cursor", "rules", "r.md"))
		}},
		{"symlinked cursor rules directory", func(t *testing.T, p, o string) {
			symlinkOrSkip(t, o, filepath.Join(p, ".cursor", "rules"))
		}},
	}
	for _, tt := range tests {
		for _, mode := range []string{"auto", "explicit"} {
			t.Run(tt.name+"/"+mode, func(t *testing.T) {
				// Arrange
				root := t.TempDir()
				project, outside := filepath.Join(root, "project"), filepath.Join(root, "outside")
				require.NoError(t, os.MkdirAll(project, 0o755))
				writeSymlinkTree(t, outside, map[string]string{
					"secret.md":         "# Secret\n" + symlinkSecret + "\n",
					"skills/x/SKILL.md": "---\nname: x\n---\n" + symlinkSecret + "\n",
				})
				writeSymlinkTree(t, project, map[string]string{"README.md": "x"})
				tt.setup(t, project, outside)
				out := filepath.Join(project, ".ai-rulez")
				sources := "auto"
				if mode == "explicit" {
					sources = "CLAUDE.md,.claude/skills,.claude/agents,.cursor/rules"
				}

				// Act
				_ = NewImporter(project, out).Import(sources) //nolint:errcheck // "nothing imported" is the expected outcome

				// Assert: the outside content is nowhere in the written tree.
				_ = filepath.WalkDir(out, func(p string, d os.DirEntry, err error) error { //nolint:errcheck // absent tree is fine
					if err != nil || d.IsDir() {
						return nil
					}
					data, _ := os.ReadFile(p) //nolint:errcheck // test
					assert.False(t, strings.Contains(string(data), symlinkSecret), "%s carries outside content", p)
					return nil
				})
			})
		}
	}
}
