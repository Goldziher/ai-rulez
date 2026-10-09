package improve

import (
	"context"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
	"github.com/Goldziher/ai-rulez/v5/internal/sandbox"
)

// recordingGit records every git process spec and runs it for real.
type recordingGit struct {
	mu    sync.Mutex
	specs []runner.Spec
}

func (r *recordingGit) runner() runner.Runner {
	return runner.Func(func(ctx context.Context, spec runner.Spec) runner.Result {
		r.mu.Lock()
		r.specs = append(r.specs, spec)
		r.mu.Unlock()
		return runner.Exec{}.Run(ctx, spec)
	})
}

func hasEnv(env []string, name string) bool {
	return slices.ContainsFunc(env, func(kv string) bool { return strings.HasPrefix(kv, name+"=") })
}

func isPush(spec runner.Spec) bool { return slices.Contains(spec.Argv, "push") }

func TestPR_GitGetsNoCredentialsExceptForThePush(t *testing.T) {
	// Arrange
	w := newPRWorld(t)
	t.Setenv("ANTHROPIC_API_KEY", "sk-leak-canary")
	t.Setenv("GH_TOKEN", "ghp_leak_canary")
	t.Setenv("SSH_AUTH_SOCK", "/tmp/agent.sock")
	rec := &recordingGit{}
	w.opts.Git = gitutil.New(rec.runner())

	// Act
	res, err := PR(context.Background(), &w.opts)

	// Assert
	require.NoError(t, err, w.out.String())
	require.True(t, res.Pushed)
	pushes := 0
	for _, spec := range rec.specs {
		assert.False(t, hasEnv(spec.Env, "ANTHROPIC_API_KEY"), "%v", spec.Argv)
		assert.False(t, hasEnv(spec.Env, "GH_TOKEN"), "%v", spec.Argv)
		assert.True(t, hasEnv(spec.Env, "PATH"), "%v", spec.Argv)
		assert.Contains(t, spec.Argv, "core.hooksPath="+os.DevNull, "no hook of the repository runs: %v", spec.Argv)
		if isPush(spec) {
			pushes++
			assert.True(t, hasEnv(spec.Env, "SSH_AUTH_SOCK"), "the push keeps the transport credentials git needs")
			continue
		}
		assert.False(t, hasEnv(spec.Env, "SSH_AUTH_SOCK"), "a local git command needs no credentials: %v", spec.Argv)
	}
	assert.Equal(t, 1, pushes)
}

func TestPR_WorktreeCommandsHaveNoNetworkUnlessAsked(t *testing.T) {
	cases := []struct {
		name        string
		allow       bool
		evals       bool
		wantNoNetOn []string // the steps that must run with the network cut
	}{
		{"default", false, false, []string{"generate"}},
		{"default with evals", false, true, []string{"generate"}},
		{"allowed", true, false, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			w := newPRWorld(t)
			rec := &recordingSelf{}
			self := "/fake/ai-rulez"
			w.opts.Self, w.opts.Exec = []string{self}, rec.run(self)
			w.opts.Isolation, w.opts.Sandbox = sandbox.ModeAuto, fakeBwrap(t)
			w.opts.AllowNetwork, w.opts.RunEvals = tc.allow, tc.evals

			// Act
			_, err := PR(context.Background(), &w.opts)

			// Assert
			require.NoError(t, err, w.out.String())
			for _, argv := range rec.argvs {
				step := argv[indexOf(argv, self)+1]
				cut := slices.Contains(argv, "--unshare-net")
				assert.Equal(t, slices.Contains(tc.wantNoNetOn, step), cut, "step %s: %v", step, argv)
			}
			if tc.evals {
				assert.Len(t, rec.argvs, 2, "generate and eval run")
			}
		})
	}
}
