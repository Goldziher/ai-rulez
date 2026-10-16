package gitutil

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The changes since a revision are the same whether git lists the untracked files
// or the parallel walk of a large repository does.
func TestChangesSinceAgreeWhenTheWalkIsUsed(t *testing.T) {
	gitAvailable(t)
	dir := diffFixture(t)
	writeIn(t, dir, ".gitignore", "ignored/\n*.log\n")
	gitIn(t, dir, "add", ".gitignore")
	gitIn(t, dir, "commit", "-q", "-m", "ignore")
	writeIn(t, dir, "a.txt", "one\ntwo\nthree\nfour\n")
	writeIn(t, dir, "pkg/new.go", "package pkg\n")
	writeIn(t, dir, "pkg/deep/more.go", "package deep\n")
	writeIn(t, dir, "ignored/z.txt", "z\n")
	writeIn(t, dir, "pkg/debug.log", "log\n")
	ctx := context.Background()
	walk := Git{bigIndexBytes: 1}
	require.True(t, walk.bigRepository(dir), "the walk is forced on")

	for _, sub := range []string{dir, filepath.Join(dir, "pkg")} {
		want, err := Git{}.ChangesSinceContext(ctx, sub, "main")
		require.NoError(t, err)
		got, err := walk.ChangesSinceContext(ctx, sub, "main")
		require.NoError(t, err)
		assert.Equal(t, want, got, "ChangesSinceContext from %s", sub)
	}

	wantFiles, err := Git{}.ChangedSinceContext(ctx, dir, "main")
	require.NoError(t, err)
	gotFiles, err := walk.ChangedSinceContext(ctx, dir, "main")
	require.NoError(t, err)
	assert.Equal(t, wantFiles, gotFiles)
	assert.Contains(t, gotFiles, "pkg/deep/more.go")
	assert.NotContains(t, gotFiles, "ignored/z.txt")
	assert.NotContains(t, gotFiles, "pkg/debug.log")
}
