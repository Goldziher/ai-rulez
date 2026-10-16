package lint

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
)

func TestDetectedHarnessFilesExcludeIndependentGitWorktrees(t *testing.T) {
	for _, linked := range []bool{false, true} {
		t.Run(map[bool]string{false: "repository", true: "linked worktree"}[linked], func(t *testing.T) {
			root := t.TempDir()
			writeFiles(t, root, map[string]string{
				".gitignore":                    ".claude/\n",
				".claude/skills/local/SKILL.md": "# Local instructions\n",
			})
			gitAdd(t, root)
			nested := filepath.Join(root, ".claude", "worktrees", "other")
			require.NoError(t, os.MkdirAll(nested, 0o755))
			require.NoError(t, gitutil.CommandNoContext(nested, "init", "-q").Run())
			if linked {
				metadata := filepath.Join(t.TempDir(), "git-data")
				require.NoError(t, os.Rename(filepath.Join(nested, ".git"), metadata))
				require.NoError(t, os.WriteFile(filepath.Join(nested, ".git"), []byte("gitdir: "+filepath.ToSlash(metadata)+"\n"), 0o644))
			}
			writeFiles(t, nested, map[string]string{"AGENTS.md": "# Independent instructions\n"})
			tree, err := LoadTree(root)
			require.NoError(t, err)

			got := newCandidates(t.Context(), tree, true).list()

			assert.Contains(t, got, ".claude/skills/local/SKILL.md")
			assert.NotContains(t, got, ".claude/worktrees/other/AGENTS.md")
		})
	}
}
