package gitutil

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

func countingGit() (Git, *atomic.Int64) {
	var n atomic.Int64
	return New(runner.Func(func(ctx context.Context, spec runner.Spec) runner.Result {
		n.Add(1)
		return runner.Run(ctx, spec)
	})), &n
}

func TestMemoAnswersRepeatedStructuralQuestionsWithOneCall(t *testing.T) {
	gitAvailable(t)
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	g, n := countingGit()

	ctx := WithMemo(t.Context())
	for range 5 {
		assert.True(t, g.IsRepoContext(ctx, dir))
		assert.NotEmpty(t, g.TopLevelContext(ctx, dir))
		assert.NotEmpty(t, g.InfoExcludePathContext(ctx, dir))
	}
	assert.Equal(t, int64(3), n.Load(), "one call per distinct question")

	n.Store(0)
	for range 5 {
		g.IsRepoContext(t.Context(), dir)
	}
	assert.Equal(t, int64(5), n.Load(), "without a Memo nothing is cached")
}

func TestMemoIsScopedToItsContext(t *testing.T) {
	gitAvailable(t)
	dir := t.TempDir()
	g, _ := countingGit()
	before := WithMemo(t.Context())
	assert.False(t, g.IsRepoContext(before, dir))

	runGit(t, dir, "init", "-q")

	assert.False(t, g.IsRepoContext(before, dir), "the old scope keeps its answer")
	assert.True(t, g.IsRepoContext(WithMemo(t.Context()), dir), "a new scope sees the new repository")
}

func TestRepoTopMatchesTheSeparateQuestions(t *testing.T) {
	gitAvailable(t)
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	sub := filepath.Join(repo, "a", "b")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	for _, dir := range []string{repo, sub, filepath.Join(repo, ".git"), t.TempDir()} {
		g, _ := countingGit()
		isRepo, top := g.repoTop(t.Context(), dir)
		assert.Equal(t, g.IsRepoContext(t.Context(), dir), isRepo, dir)
		assert.Equal(t, g.TopLevelContext(t.Context(), dir), top, dir)
	}

	g, n := countingGit()
	isRepo, top := g.repoTop(t.Context(), sub)
	assert.True(t, isRepo)
	assert.NotEmpty(t, top)
	assert.Equal(t, int64(1), n.Load())
}
