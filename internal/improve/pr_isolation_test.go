package improve

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/runner"
	"github.com/Goldziher/ai-rulez/v5/internal/sandbox"
)

// recordingSelf answers every command run in the worktree with success and records its argv; commands that
// are not the stand-in ai-rulez (gh) go to a real process.
type recordingSelf struct {
	mu    sync.Mutex
	argvs [][]string
	dirs  []string
}

func (r *recordingSelf) run(self string) runner.Runner {
	return runner.Func(func(ctx context.Context, spec runner.Spec) runner.Result {
		isSelf := false
		for _, a := range spec.Argv {
			isSelf = isSelf || a == self
		}
		if !isSelf {
			return runner.Exec{}.Run(ctx, spec)
		}
		r.mu.Lock()
		r.argvs = append(r.argvs, spec.Argv)
		r.dirs = append(r.dirs, spec.Dir)
		r.mu.Unlock()
		return runner.Result{Status: runner.StatusOK}
	})
}

func fakeBwrap(t *testing.T) *sandbox.Sandbox {
	t.Helper()
	look := func(name string) (string, error) {
		if name == "bwrap" {
			return "/usr/bin/bwrap", nil
		}
		return "", errors.New("not found")
	}
	return sandbox.New("linux", look).WithRunner(runner.Func(func(context.Context, runner.Spec) runner.Result {
		return runner.Result{Status: runner.StatusOK}
	}))
}

func noSandbox() *sandbox.Sandbox {
	return sandbox.New("linux", func(string) (string, error) { return "", errors.New("not found") })
}

func TestPR_ConfinesTheWorktreeCommands(t *testing.T) {
	// Arrange
	w := newPRWorld(t)
	rec := &recordingSelf{}
	self := "/fake/ai-rulez"
	w.opts.Self = []string{self}
	w.opts.Exec = rec.run(self)
	w.opts.Isolation, w.opts.Sandbox = sandbox.ModeAuto, fakeBwrap(t)
	cache := t.TempDir()
	w.opts.Env = append(w.opts.Env, "XDG_CACHE_HOME="+cache)

	// Act
	res, err := PR(context.Background(), &w.opts)

	// Assert: generate ran under the sandbox, writable only in the worktree and the cache
	require.NoError(t, err, w.out.String())
	require.Len(t, rec.argvs, 1, "generate (the project has no lock)")
	for _, argv := range rec.argvs {
		assert.Equal(t, "/usr/bin/bwrap", argv[0], "the command runs under the sandbox tool")
		assert.NotContains(t, argv, "--unshare-net", "generate and lock fetch remote includes, so the network stays on")
		cut := indexOf(argv, "--")
		require.Positive(t, cut)
		assert.Equal(t, []string{self}, argv[cut+1:cut+2])
		binds := bindTargets(argv[:cut])
		assert.Contains(t, binds, realPath(t, filepath.Join(cache, "ai-rulez")), "only ai-rulez's subdirectory of the cache")
		assert.NotContains(t, binds, realPath(t, cache))
		inWorktree := false
		for _, b := range binds {
			inWorktree = inWorktree || strings.Contains(b, "ai-rulez-improve-pr-")
		}
		assert.True(t, inWorktree, "the worktree is writable: %v", binds)
		for _, b := range binds {
			assert.NotEqual(t, realPath(t, w.root), b, "the user's checkout is not writable")
		}
	}
	require.NotNil(t, res.Isolation)
	assert.True(t, res.Isolation.Confined)
	assert.Equal(t, "bwrap", res.Isolation.Backend)
	assert.Contains(t, w.out.String(), "bwrap sandbox: writes only inside the worktree")
	assert.True(t, res.Isolation.NoWrites)
}

func TestPR_IsolationNoneLeavesTheCommandsAlone(t *testing.T) {
	// Arrange
	w := newPRWorld(t)
	rec := &recordingSelf{}
	self := "/fake/ai-rulez"
	w.opts.Self, w.opts.Exec = []string{self}, rec.run(self)
	w.opts.Isolation, w.opts.Sandbox = sandbox.ModeNone, fakeBwrap(t)

	// Act
	res, err := PR(context.Background(), &w.opts)

	// Assert
	require.NoError(t, err, w.out.String())
	require.NotEmpty(t, rec.argvs)
	assert.Equal(t, self, rec.argvs[0][0])
	assert.Nil(t, res.Isolation)
}

func TestPR_IsolationRequireRefusesWithoutABackendBeforeAnythingIsCreated(t *testing.T) {
	// Arrange
	w := newPRWorld(t)
	w.opts.Self = []string{"/fake/ai-rulez"}
	w.opts.Isolation, w.opts.Sandbox = sandbox.ModeRequire, noSandbox()

	// Act
	_, err := PR(context.Background(), &w.opts)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), CodeIsolationUnavailable)
	assert.Contains(t, err.Error(), "--isolation require")
	assert.Empty(t, gitIn(t, w.root, "branch", "--list", "ai-rulez/improve/*"), "no branch was made")
}

func TestPR_IsolationAutoWithoutABackendWarnsAndRunsUnconfined(t *testing.T) {
	// Arrange
	w := newPRWorld(t)
	rec := &recordingSelf{}
	self := "/fake/ai-rulez"
	w.opts.Self, w.opts.Exec = []string{self}, rec.run(self)
	w.opts.Isolation, w.opts.Sandbox = sandbox.ModeAuto, noSandbox()

	// Act
	res, err := PR(context.Background(), &w.opts)

	// Assert
	require.NoError(t, err, w.out.String())
	assert.Equal(t, self, rec.argvs[0][0])
	assert.Contains(t, w.out.String(), "no sandbox backend works")
	require.NotNil(t, res.Isolation)
	assert.False(t, res.Isolation.Confined)
}

func indexOf(list []string, s string) int {
	for i, v := range list {
		if v == s {
			return i
		}
	}
	return -1
}

// bindTargets lists the directories a bwrap argv binds writable (--bind SRC DST).
func bindTargets(argv []string) []string {
	var out []string
	for i, a := range argv {
		if a == "--bind" && i+2 < len(argv) {
			out = append(out, argv[i+2])
		}
	}
	return out
}

func realPath(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	require.NoError(t, err)
	return r
}
