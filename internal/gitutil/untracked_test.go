package gitutil

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// writeTree writes files (relative slash path -> content) below dir.
func writeTree(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
	}
}

// TestOthersWalkMatchesGit holds the in-process untracked listing to what
// `git ls-files --others --exclude-standard` prints, over the cases that
// separate the two: ignored files and directories, a re-included file, an
// untracked file in a tracked directory, a wholly untracked directory, a nested
// repository, an empty directory and a link.
func TestOthersWalkMatchesGit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks and file modes differ on Windows")
	}
	gitAvailable(t)
	top := t.TempDir()
	runGit(t, top, "init", "-q")
	writeTree(t, top, map[string]string{
		".gitignore":               "*.log\nbuild/\n/vendor/\n!keep.log\n",
		"src/a.go":                 "x",
		"src/new.go":               "x",
		"src/debug.log":            "x",
		"src/keep.log":             "x",
		"src/gen/skip.tmp":         "x",
		"src/.gitignore":           "*.tmp\n",
		"build/out.bin":            "x",
		"vendor/lib/x.go":          "x",
		"docs/guide.md":            "x",
		"fresh/one.md":             "x",
		"fresh/deep/two.md":        "x",
		"ignored-only/x.log":       "x",
		"we[ir]d/file.md":          "x",
		"we[ir]d/tracked.md":       "x",
		"nested/inner/.keep":       "x",
		"tracked-dir/sub/new.txt":  "x",
		"tracked-dir/sub/old.txt":  "x",
		"tracked-dir/other/o.txt":  "x",
		"tracked-dir/a b/space.md": "x",
	})
	require.NoError(t, os.MkdirAll(filepath.Join(top, "empty", "dir"), 0o755))
	runGit(t, filepath.Join(top, "nested", "inner"), "init", "-q")
	testutil.SymlinkOrSkip(t, "src", filepath.Join(top, "linkdir"))
	testutil.SymlinkOrSkip(t, "a.go", filepath.Join(top, "src", "link.go"))
	runGit(t, top, "add", ".gitignore", "src/a.go", "docs/guide.md", "we[[]ir]d/tracked.md", "tracked-dir/sub/old.txt", "tracked-dir/other/o.txt")

	g := Git{}
	ctx := context.Background()
	for _, dir := range []string{top, filepath.Join(top, "src"), filepath.Join(top, "tracked-dir")} {
		want, err := g.othersSingle(ctx, dir)
		require.NoError(t, err)
		cached, _, err := g.run(ctx, dir, nil, "ls-files", "--cached", "-z")
		require.NoError(t, err)
		got, ok := g.othersWalk(ctx, dir, splitNUL(cached))
		require.True(t, ok)
		assert.Equal(t, want, got, "untracked listing below %s", dir)
	}
	want, err := g.othersSingle(ctx, top)
	require.NoError(t, err)
	assert.Contains(t, want, "src/new.go")
	assert.Contains(t, want, "src/keep.log")
	assert.Contains(t, want, "nested/inner/")
	assert.NotContains(t, want, "src/debug.log")
}

func TestListFilesAgreesWhenTheWalkIsUsed(t *testing.T) {
	gitAvailable(t)
	top := t.TempDir()
	runGit(t, top, "init", "-q")
	writeTree(t, top, map[string]string{"a.md": "x", "sub/b.md": "x", "sub/c.md": "x", ".gitignore": "ignored/\n", "ignored/z.md": "x"})
	runGit(t, top, "add", "a.md", "sub/b.md", ".gitignore")

	want, ok, err := Git{}.ListFilesContext(context.Background(), top)
	require.NoError(t, err)
	require.True(t, ok)

	got, ok, err := Git{bigIndexBytes: 1}.ListFilesContext(context.Background(), top)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, want, got)
	assert.Equal(t, []string{".gitignore", "a.md", "sub/b.md", "sub/c.md"}, got)
}

func TestOthersWalkListsDirectoryReplacingTrackedFile(t *testing.T) {
	gitAvailable(t)
	top := t.TempDir()
	runGit(t, top, "init", "-q")
	writeTree(t, top, map[string]string{"tracked": "old content"})
	runGit(t, top, "add", "tracked")
	require.NoError(t, os.Remove(filepath.Join(top, "tracked")))
	writeTree(t, top, map[string]string{"tracked/AGENTS.md": "new instructions"})

	ctx := t.Context()
	want, err := Git{}.othersSingle(ctx, top)
	require.NoError(t, err)
	require.Equal(t, []string{"tracked/AGENTS.md"}, want)
	got, err := (Git{bigIndexBytes: 1}).othersContext(ctx, top)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}
