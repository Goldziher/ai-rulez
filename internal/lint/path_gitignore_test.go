package lint

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AR401 skips a documented path that the repository's own gitignore or
// .git/info/exclude hides, since the file is local by design (#459).
func TestPathMissingSkipsGitIgnoredPaths(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{"gitignored file", "Local overrides live in `.claude/settings.local.json`.", 0},
		{"info exclude file", "Keep notes in `.claude/personal-preferences.md`.", 0},
		{"not ignored stays reported", "See `.claude/other.md` for details.", 1},
		{"ignored and missing together", "`.claude/settings.local.json` and `.claude/other.md`", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeFiles(t, root, precisionTree(tt.body, "", map[string]string{
				".gitignore":            ".claude/settings.local.json\n",
				".claude/settings.json": "{}\n",
			}))
			gitAdd(t, root)
			exclude := filepath.Join(root, ".git", "info", "exclude")
			require.NoError(t, os.MkdirAll(filepath.Dir(exclude), 0o755))
			require.NoError(t, os.WriteFile(exclude, []byte(".claude/personal-preferences.md\n"), 0o644))

			fs := lintDir(t, root)

			assert.Equal(t, tt.want, countCode(fs, CodePathMissing), "%s", dump(fs))
		})
	}
}
