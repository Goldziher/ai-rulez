package gitutil

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.email=t@example.com", "-c", "user.name=t"}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

func TestIsLinkedWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	main := filepath.Join(root, "main")
	require.NoError(t, os.MkdirAll(main, 0o755))
	gitRun(t, main, "init", "-q")
	require.NoError(t, os.WriteFile(filepath.Join(main, "f"), []byte("x"), 0o644))
	gitRun(t, main, "add", "f")
	gitRun(t, main, "commit", "-q", "-m", "init")
	linked := filepath.Join(root, "linked")
	gitRun(t, main, "worktree", "add", "-q", linked, "-b", "wt")

	tests := []struct {
		name string
		dir  string
		want bool
	}{
		{"main checkout", main, false},
		{"linked worktree", linked, true},
		{"not a repository", root, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsLinkedWorktree(tt.dir))
		})
	}
}

func TestWorktreesListsRegisteredWorktrees(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	g := Git{}
	ctx := context.Background()
	root := t.TempDir()
	main := filepath.Join(root, "main")
	require.NoError(t, os.MkdirAll(main, 0o755))
	gitRun(t, main, "init", "-q")
	require.NoError(t, os.WriteFile(filepath.Join(main, "f"), []byte("x"), 0o644))
	gitRun(t, main, "add", "f")
	gitRun(t, main, "commit", "-q", "-m", "init")
	linked := filepath.Join(root, "linked")
	gitRun(t, main, "worktree", "add", "-q", linked, "-b", "wt")
	detached := filepath.Join(root, "detached")
	gitRun(t, main, "worktree", "add", "-q", detached, "--detach", "HEAD")

	got, err := g.WorktreesContext(ctx, main)
	require.NoError(t, err)

	paths := map[string]WorktreeInfo{}
	for _, w := range got {
		paths[filepath.Base(w.Path)] = w
	}
	require.Len(t, paths, 3)
	assert.Equal(t, "main", paths["main"].Branch, "the main checkout is listed with its branch")
	assert.Equal(t, "wt", paths["linked"].Branch)
	assert.False(t, paths["linked"].Detached)
	assert.True(t, paths["detached"].Detached)
	assert.Equal(t, "", paths["detached"].Branch)
	for name, w := range paths {
		assert.True(t, filepath.IsAbs(w.Path), "%s path is absolute", name)
		assert.NotEmpty(t, w.Head, "%s has a head", name)
	}
}

func TestCommonDirMatchesAcrossWorktrees(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	g := Git{}
	ctx := context.Background()
	root := t.TempDir()
	main := filepath.Join(root, "main")
	require.NoError(t, os.MkdirAll(main, 0o755))
	gitRun(t, main, "init", "-q")
	linked := filepath.Join(root, "linked")
	gitRun(t, main, "worktree", "add", "-q", linked, "-b", "wt")

	mainCommon, err := g.CommonDirContext(ctx, main)
	require.NoError(t, err)
	linkedCommon, err := g.CommonDirContext(ctx, linked)
	require.NoError(t, err)

	assert.Equal(t, mainCommon, linkedCommon, "a linked worktree shares the common dir")
	assert.True(t, strings.HasSuffix(filepath.ToSlash(mainCommon), "/.git"),
		"the main checkout's common dir is its own .git, got %s", mainCommon)
}
