package lint

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// vendored writes n small files below dir, the way an installed dependency tree looks.
func vendored(t *testing.T, root, dir string, n int) {
	t.Helper()
	base := filepath.Join(root, filepath.FromSlash(dir))
	require.NoError(t, os.MkdirAll(base, 0o755))
	for i := range n {
		require.NoError(t, os.WriteFile(filepath.Join(base, fmt.Sprintf("f%05d.md", i)), []byte("# v\n"), 0o644))
	}
}

func resolveDir(t *testing.T, root, path string) (*Targets, error) {
	t.Helper()
	tree, err := LoadTree(root)
	require.NoError(t, err)
	return ResolveTargets(context.Background(), tree, TargetSpec{Paths: []string{path}, Cwd: root})
}

func TestPathDirectoryTakesGitListedFilesOnly(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".gitignore":          "node_modules/\n",
		"docs/guide.md":       "# guide\n",
		"docs/AGENTS.md":      "# agents\n",
		"docs/untracked.md":   "# u\n",
		"app/AGENTS.md":       "# app\n",
		"node_modules/x/a.md": "# dep\n",
	})
	vendored(t, root, "node_modules/big", 21000) // more than maxTargetFiles
	gitAdd(t, root)
	writeFiles(t, root, map[string]string{"docs/late.md": "# late, untracked and not ignored\n"})

	tests := []struct {
		name string
		path string
		want []string
	}{
		{name: "a subdirectory", path: "docs", want: []string{"docs/AGENTS.md", "docs/guide.md", "docs/late.md", "docs/untracked.md"}},
		{name: "the project root", path: ".", want: []string{"app/AGENTS.md", "docs/AGENTS.md", "docs/guide.md", "docs/late.md", "docs/untracked.md"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			targets, err := resolveDir(t, root, tc.path)

			require.NoError(t, err, "an ignored dependency tree must not count against the limit")
			assert.Equal(t, tc.want, targets.Rels())
		})
	}
}

func TestPathDirectoryNeverWalksDotGit(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"AGENTS.md":            "# a\n",
		".git/hooks/AGENTS.md": "# hook\n",
	})
	// Not a repository: the walk runs, and must skip what the project walk skips.
	vendored(t, root, "node_modules/big", 21000)

	targets, err := resolveDir(t, root, ".")

	require.NoError(t, err)
	assert.Equal(t, []string{"AGENTS.md"}, targets.Rels())
}

func TestPathDirectoryInsideAHiddenFolderStillWalks(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".claude/skills/s/SKILL.md":          "---\nname: s\ndescription: d\n---\nbody\n",
		".claude/node_modules/pkg/AGENTS.md": "# dep\n",
	})

	targets, err := resolveDir(t, root, ".claude")

	require.NoError(t, err)
	assert.Equal(t, []string{".claude/skills/s/SKILL.md"}, targets.Rels())
}
