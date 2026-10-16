package gitutil

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

// lsFilesArgs records the argv of every `git ls-files` run it forwards.
type lsFilesArgs struct {
	mu    sync.Mutex
	calls [][]string
}

func (c *lsFilesArgs) Run(ctx context.Context, spec runner.Spec) runner.Result {
	for _, a := range spec.Argv {
		if a == "ls-files" {
			c.mu.Lock()
			c.calls = append(c.calls, append([]string(nil), spec.Argv...))
			c.mu.Unlock()
			break
		}
	}
	return runner.Run(ctx, spec)
}

// pathspecCalls counts the ls-files runs that carried a literal pathspec, the
// per-question fallback, as opposed to the whole-repository listing.
func (c *lsFilesArgs) pathspecCalls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, argv := range c.calls {
		for _, a := range argv {
			if strings.HasPrefix(a, ":(literal)") {
				n++
				break
			}
		}
	}
	return n
}

// #376: git's literal pathspec compares names exactly even under
// core.ignorecase (it folds case only with the icase magic), so the shared
// listing's exact-string lookup answers the same. This pins that equivalence:
// a git that folded case would fail it and the lookup would need to defer.
func TestSharedIndexMatchesThePathspecWhenGitIgnoresCase(t *testing.T) {
	repo := sharedIndexRepo(t)
	runGit(t, repo, "config", "core.ignorecase", "true")
	dir := filepath.Join(repo, "a")
	paths := []string{".ai-rulez/config.toml", ".AI-RULEZ/config.toml", ".ai-rulez/CONFIG.toml"}

	plain, err := New(&lsFilesArgs{}).TrackedAmongContext(t.Context(), dir, paths)
	require.NoError(t, err)
	shared, err := New(&lsFilesArgs{}).TrackedAmongContext(WithSharedIndex(t.Context()), dir, paths)
	require.NoError(t, err)

	assert.Equal(t, plain, shared)
	assert.True(t, shared[".ai-rulez/config.toml"])
}

// A path below a gitlink (a submodule) is for the pathspec call: the index
// lists the submodule as one entry, so the listing cannot speak for what is in it.
func TestSharedIndexDefersToThePathspecBelowAGitlink(t *testing.T) {
	repo := sharedIndexRepo(t)
	runGit(t, repo, "update-index", "--add", "--cacheinfo", "160000,e69de29bb2d1d6434b8b29ae775ad8c2e48c5391,sub")
	paths := []string{"sub/inner/config.toml", "b/other.txt"}

	plain, err := New(&lsFilesArgs{}).TrackedAmongContext(t.Context(), repo, paths)
	rec := &lsFilesArgs{}
	shared, sharedErr := New(rec).TrackedAmongContext(WithSharedIndex(t.Context()), repo, paths)

	assert.Equal(t, err == nil, sharedErr == nil, "fails closed exactly when the pathspec call does")
	assert.Equal(t, plain, shared)
	assert.Equal(t, 1, rec.pathspecCalls(), "the path below the gitlink went to the pathspec call")
	assert.True(t, shared["b/other.txt"] || err != nil)
}

func TestSharedIndexListsTheRepositoryFilesOnce(t *testing.T) {
	tests := []struct {
		name      string
		ctx       func(context.Context) context.Context
		wantCalls int64
	}{
		{"with a shared index", WithSharedIndex, 1},
		{"without one", func(ctx context.Context) context.Context { return ctx }, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			repo := sharedIndexRepo(t)
			c := &lsFilesRunner{}
			g := New(c)
			ctx := tt.ctx(t.Context())

			// Act
			var last []string
			for range 3 {
				files, ok, err := g.ListFilesContext(ctx, repo)
				require.NoError(t, err)
				require.True(t, ok)
				last = files
			}

			// Assert
			assert.Equal(t, tt.wantCalls, c.lsFiles.Load())
			assert.Contains(t, last, "b/other.txt")
			assert.Contains(t, last, "b/.ai-rulez/config.toml", "an untracked file that git does not ignore is listed")
		})
	}
}
