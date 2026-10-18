package lint

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	rn "github.com/Goldziher/ai-rulez/v5/internal/runner"
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

// The ignore probe runs under the caller's context: a canceled run cannot ask
// git, so a missing path stays reported instead of being assumed ignored.
func TestPathMissingIgnoreProbeHonoursContext(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, precisionTree("Local overrides live in `.claude/settings.local.json`.", "", map[string]string{
		".gitignore":            ".claude/settings.local.json\n",
		".claude/settings.json": "{}\n",
	}))
	gitAdd(t, root)
	cfg, err := loadWithResolvers(context.Background(), root)
	require.NoError(t, err)
	tree, err := LoadTree(root)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	rep, err := Run(cfg, tree, WithContext(ctx))

	require.NoError(t, err)
	assert.Equal(t, 1, countCode(rep.Findings, CodePathMissing), "%s", dump(rep.Findings))
}

// git is asked once for all the missing paths of a run, not once per path.
func TestPathMissingAsksGitOnce(t *testing.T) {
	root := t.TempDir()
	body := "Local overrides live in `.claude/settings.local.json`. Also `.claude/m1.md`, `.claude/m2.md`, `.claude/m3.md` and `.claude/m4.md`."
	writeFiles(t, root, precisionTree(body, "", map[string]string{
		".gitignore":            ".claude/settings.local.json\n",
		".claude/settings.json": "{}\n",
	}))
	gitAdd(t, root)
	cfg, err := loadWithResolvers(context.Background(), root)
	require.NoError(t, err)
	tree, err := LoadTree(root)
	require.NoError(t, err)
	var mu sync.Mutex
	asked := 0
	counting := rn.Func(func(ctx context.Context, spec rn.Spec) rn.Result {
		if slices.Contains(spec.Argv, "check-ignore") {
			mu.Lock()
			asked++
			mu.Unlock()
		}
		return rn.Exec{}.Run(ctx, spec)
	})

	rep, err := Run(cfg, tree, WithHost(ambient.Host{Runner: counting}))

	require.NoError(t, err)
	assert.Equal(t, 4, countCode(rep.Findings, CodePathMissing), "%s", dump(rep.Findings))
	assert.Equal(t, 1, asked, "check-ignore runs once for the whole run")
}
