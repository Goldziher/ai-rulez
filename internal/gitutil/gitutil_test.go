package gitutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gitAvailable(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)...) //nolint:gosec // test
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

func TestNotARepositoryDegradesGracefully(t *testing.T) {
	dir := t.TempDir()

	tracked, err := TrackedAmong(dir, []string{"a.md"})
	require.NoError(t, err)
	ignored, ignoredErr := IgnoredAmong(dir, []string{"a.md"})
	require.NoError(t, ignoredErr)

	assert.False(t, IsRepo(dir))
	assert.Empty(t, tracked)
	assert.Nil(t, ignored, "nil tells the caller to use its own matcher")
	assert.Empty(t, InfoExcludePath(dir))
	assert.Empty(t, TopLevel(dir))
}

func TestTrackedAmongIsRelativeToTheQueriedDirectory(t *testing.T) {
	gitAvailable(t)
	top := t.TempDir()
	runGit(t, top, "init", "-q")
	sub := filepath.Join(top, "pkg")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(top, "root.md"), []byte("x"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "AGENTS.md"), []byte("x"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "untracked.md"), []byte("x"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "we[ir]d*.md"), []byte("x"), 0o600))
	runGit(t, top, "add", "root.md", "pkg/AGENTS.md", "pkg/we[[]ir]d*.md")

	atTop, err := TrackedAmong(top, []string{"root.md", "pkg/AGENTS.md", "pkg/untracked.md"})
	require.NoError(t, err)
	atSub, err := TrackedAmong(sub, []string{"AGENTS.md", "root.md", "we[ir]d*.md", "weird.md"})
	require.NoError(t, err)

	assert.Equal(t, map[string]bool{"root.md": true, "pkg/AGENTS.md": true}, atTop)
	assert.True(t, atSub["AGENTS.md"])
	assert.False(t, atSub["root.md"])
	assert.True(t, atSub["we[ir]d*.md"], "pathspecs are literal")
	assert.False(t, atSub["weird.md"], "a glob in the query must not match other files")
	assert.True(t, IsRepo(sub))
}

func TestIgnoredAmongHonoursNegationAndDoubleStar(t *testing.T) {
	gitAvailable(t)
	top := t.TempDir()
	runGit(t, top, "init", "-q")
	require.NoError(t, os.WriteFile(filepath.Join(top, ".gitignore"), []byte("**/*.gen.md\n!keep.gen.md\n/build/\n"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(top, ".git", "info"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(top, ".git", "info", "exclude"), []byte("/excluded.md\n"), 0o600))

	ignored, err := IgnoredAmong(top, []string{"a.gen.md", "deep/er/b.gen.md", "keep.gen.md", "build/x.md", "excluded.md", "plain.md"})

	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"a.gen.md": true, "deep/er/b.gen.md": true, "build/x.md": true, "excluded.md": true}, ignored)
}

func TestIgnoredAmongNoneIgnoredIsNotAnError(t *testing.T) {
	gitAvailable(t)
	top := t.TempDir()
	runGit(t, top, "init", "-q")

	ignored, err := IgnoredAmong(top, []string{"a.md"})

	require.NoError(t, err)
	assert.Empty(t, ignored)
}

func TestInfoExcludePathResolvesInLinkedWorktrees(t *testing.T) {
	gitAvailable(t)
	main := t.TempDir()
	runGit(t, main, "init", "-q")
	require.NoError(t, os.WriteFile(filepath.Join(main, "a.txt"), []byte("x"), 0o600))
	runGit(t, main, "add", "a.txt")
	runGit(t, main, "commit", "-q", "-m", "init")
	linked := filepath.Join(t.TempDir(), "wt")
	runGit(t, main, "worktree", "add", "-q", linked, "-b", "feature")

	mainExclude := InfoExcludePath(main)
	linkedExclude := InfoExcludePath(linked)

	require.NotEmpty(t, mainExclude)
	assert.True(t, filepath.IsAbs(linkedExclude))
	assert.Equal(t, Resolve(filepath.Dir(filepath.Dir(mainExclude))), Resolve(filepath.Dir(filepath.Dir(linkedExclude))),
		"linked worktrees share the common info/exclude")
	assert.NotEqual(t, TopLevel(main), TopLevel(linked), "each worktree has its own top level")
	assert.Equal(t, "x/y.md", RepoRelative(TopLevel(linked), filepath.Join(linked, "x", "y.md")))
	assert.Empty(t, RepoRelative(TopLevel(linked), filepath.Join(main, "x.md")))
}
