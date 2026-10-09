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

// countingRunner counts the calls it forwards to a real process. A pointer is
// comparable, so the Memo can tell it from any other runner.
type countingRunner struct{ n atomic.Int64 }

func (c *countingRunner) Run(ctx context.Context, spec runner.Spec) runner.Result {
	c.n.Add(1)
	return runner.Run(ctx, spec)
}

func countingGit() (Git, *atomic.Int64) {
	c := &countingRunner{}
	return New(c), &c.n
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

func TestMemoDoesNotShareAnswersBetweenRunners(t *testing.T) {
	answering := func(out string) Git {
		return New(&runner.Fake{Handle: func(runner.Spec) runner.Result {
			return runner.Result{Status: runner.StatusOK, Stdout: []byte(out)}
		}})
	}
	yes, no := answering("true\n"), answering("")
	ctx := WithMemo(t.Context())

	assert.True(t, yes.IsRepoContext(ctx, "/repo"))
	assert.False(t, no.IsRepoContext(ctx, "/repo"), "another runner is another repository as far as the memo knows")
	assert.True(t, yes.IsRepoContext(ctx, "/repo"), "and the first one still has its own answer")
}

// opaqueRunner is comparable by type but holds an interface whose dynamic value
// (a slice) is not: using it as a map key panics at run time.
type opaqueRunner struct {
	state any
	calls *atomic.Int64
}

func (o opaqueRunner) Run(context.Context, runner.Spec) runner.Result {
	o.calls.Add(1)
	return runner.Result{Status: runner.StatusOK, Stdout: []byte("true\n")}
}

func TestMemoDoesNotPanicOnAnUncomparableRunner(t *testing.T) {
	var calls atomic.Int64
	g := New(opaqueRunner{state: []string{"x"}, calls: &calls})
	ctx := WithMemo(t.Context())

	assert.NotPanics(t, func() {
		assert.True(t, g.IsRepoContext(ctx, "/repo"))
		assert.True(t, g.IsRepoContext(ctx, "/repo"))
	})
	assert.Equal(t, int64(2), calls.Load(), "a runner without an identity is never memoised")
}

func TestMemoDoesNotShareAnswersBetweenFuncRunners(t *testing.T) {
	answering := func(out string) Git {
		return New(runner.Func(func(context.Context, runner.Spec) runner.Result {
			return runner.Result{Status: runner.StatusOK, Stdout: []byte(out)}
		}))
	}
	yes, no := answering("true\n"), answering("")
	ctx := WithMemo(t.Context())

	assert.True(t, yes.IsRepoContext(ctx, "/repo"))
	assert.False(t, no.IsRepoContext(ctx, "/repo"), "two func adapters are two runners")
	assert.True(t, yes.IsRepoContext(ctx, "/repo"))
}

func TestMemoDoesNotRememberACancelledQuestion(t *testing.T) {
	g := New(&ctxRunner{})
	memoCtx := WithMemo(t.Context())

	canceled, cancel := context.WithCancel(memoCtx)
	cancel()
	assert.False(t, g.IsRepoContext(canceled, "/repo"), "a canceled run cannot answer")

	assert.True(t, g.IsRepoContext(memoCtx, "/repo"), "the cancellation was not remembered for the live context")
}

// ctxRunner fails like a runner does when its context ends, and answers otherwise.
type ctxRunner struct{}

func (*ctxRunner) Run(ctx context.Context, _ runner.Spec) runner.Result {
	if err := ctx.Err(); err != nil {
		return runner.Result{Status: runner.StatusError, Err: err}
	}
	return runner.Result{Status: runner.StatusOK, Stdout: []byte("true\n")}
}
