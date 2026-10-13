package gitutil

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

// lsFilesRunner counts the `git ls-files` runs it forwards.
type lsFilesRunner struct{ lsFiles atomic.Int64 }

func (c *lsFilesRunner) Run(ctx context.Context, spec runner.Spec) runner.Result {
	for _, a := range spec.Argv {
		if a == "ls-files" {
			c.lsFiles.Add(1)
			break
		}
	}
	return runner.Run(ctx, spec)
}

func sharedIndexRepo(t *testing.T) string {
	t.Helper()
	gitAvailable(t)
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	for _, rel := range []string{"a/.ai-rulez/config.toml", "b/.ai-rulez/config.toml", "b/other.txt"} {
		require.NoError(t, os.MkdirAll(filepath.Join(repo, filepath.Dir(rel)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(repo, rel), []byte("x"), 0o644))
	}
	runGit(t, repo, "add", "a/.ai-rulez/config.toml", "b/other.txt")
	return repo
}

// #376: with a shared index every root of a repository asks "is this file
// tracked" of one listing instead of paying a full index read per question.
func TestSharedIndexAnswersTrackedQuestionsFromOneListing(t *testing.T) {
	repo := sharedIndexRepo(t)
	c := &lsFilesRunner{}
	g := New(c)
	ctx := WithSharedIndex(t.Context())

	aTracked, err := g.TrackedAmongContext(ctx, filepath.Join(repo, "a"), []string{".ai-rulez/config.toml", "missing"})
	require.NoError(t, err)
	bTracked, err := g.TrackedAmongContext(ctx, filepath.Join(repo, "b"), []string{".ai-rulez/config.toml", "other.txt"})
	require.NoError(t, err)
	again, err := g.TrackedAmongContext(ctx, filepath.Join(repo, "a", ".ai-rulez"), []string{"config.toml"})
	require.NoError(t, err)

	assert.Equal(t, map[string]bool{".ai-rulez/config.toml": true}, aTracked)
	assert.Equal(t, map[string]bool{"other.txt": true}, bTracked, "an untracked file is not tracked")
	assert.Equal(t, map[string]bool{"config.toml": true}, again)
	assert.Equal(t, int64(1), c.lsFiles.Load(), "one listing for the whole repository")
}

func TestSharedIndexMatchesThePathspecAnswers(t *testing.T) {
	repo := sharedIndexRepo(t)
	paths := []string{".ai-rulez/config.toml", "config.toml", "../b/other.txt", "nope/x"}
	dir := filepath.Join(repo, "a")

	plain, err := New(&lsFilesRunner{}).TrackedAmongContext(t.Context(), dir, paths)
	require.NoError(t, err)
	shared, err := New(&lsFilesRunner{}).TrackedAmongContext(WithSharedIndex(t.Context()), dir, paths)
	require.NoError(t, err)

	assert.Equal(t, plain, shared)
}

func TestSharedIndexOutsideARepositoryTracksNothing(t *testing.T) {
	gitAvailable(t)
	tracked, err := New(&lsFilesRunner{}).TrackedAmongContext(WithSharedIndex(t.Context()), t.TempDir(), []string{"a"})
	require.NoError(t, err)
	assert.Empty(t, tracked)
}

func TestSharedIndexIsSafeForConcurrentRoots(t *testing.T) {
	repo := sharedIndexRepo(t)
	c := &lsFilesRunner{}
	g := New(c)
	ctx := WithSharedIndex(t.Context())

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := g.TrackedAmongContext(ctx, filepath.Join(repo, "a"), []string{".ai-rulez/config.toml"})
			assert.NoError(t, err)
			assert.True(t, got[".ai-rulez/config.toml"])
		}()
	}
	wg.Wait()

	assert.Equal(t, int64(1), c.lsFiles.Load(), "concurrent askers share one listing")
}
