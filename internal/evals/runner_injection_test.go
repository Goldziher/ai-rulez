package evals

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

func TestCommandRunnerThroughAnInjectedRunner(t *testing.T) {
	tests := []struct {
		name    string
		answer  runner.Result
		wantErr string
	}{
		{
			name: "a valid answer is parsed",
			answer: runner.Result{Status: runner.StatusOK, Stderr: []byte("note\n"),
				Stdout: []byte(`{"version":1,"results":[{"case":"fires","arm":"with","triggered":true,"output":"ok"}]}`)},
		},
		{
			name:    "a failed command is reported",
			answer:  runner.Result{Status: runner.StatusExit, ExitCode: 4, Err: assert.AnError},
			wantErr: "runner command failed",
		},
		{
			name:    "a timeout names the limit",
			answer:  runner.Result{Status: runner.StatusTimeout, Err: assert.AnError},
			wantErr: "timed out after",
		},
		{
			name:    "truncated output is refused",
			answer:  runner.Result{Status: runner.StatusOK, Stdout: []byte(`{"version":1`), StdoutTruncated: true},
			wantErr: "output exceeds",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			req := sampleRequest(t)
			fake := &runner.Fake{Handle: func(runner.Spec) runner.Result { return tt.answer }}
			var stderr bytes.Buffer
			cr := &CommandRunner{Command: "my-harness --flag", Runner: fake, Stderr: &stderr}

			// Act
			resp, err := cr.Run(context.Background(), req)

			// Assert
			calls := fake.Calls()
			require.Len(t, calls, 1)
			assert.Contains(t, calls[0].Argv, "my-harness --flag")
			assert.Contains(t, string(calls[0].Stdin), `"version":1`)
			assert.Contains(t, calls[0].Env, "AI_RULEZ_EVAL_SKILL="+req.Skill.ID)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Len(t, resp.Results, 1)
			assert.Equal(t, "note\n", stderr.String())
		})
	}
}

func TestDeniedRunnerStopsACommandAssertion(t *testing.T) {
	// Arrange
	a := &Assertion{Type: "command_exit", Command: "true"}

	// Act
	msg := runCommandAssertion(a, t.TempDir(), GradeOptions{AllowExec: true, Runner: runner.Deny{}})

	// Assert
	assert.Contains(t, msg, "command did not run")
	assert.Contains(t, msg, "not allowed")
}

func TestChildProcessesGetAScrubbedEnvironment(t *testing.T) {
	parent := []string{"PATH=/bin", "HOME=/home/u", "CI=true", "GITHUB_TOKEN=gh", "AWS_SECRET_ACCESS_KEY=aws", "ANTHROPIC_API_KEY=k", "PROJECT_FLAG=1"}
	tests := []struct {
		name    string
		run     func(t *testing.T, fake *runner.Fake)
		want    []string
		dropped []string
	}{
		{
			name: "a command_exit assertion keeps the base, CI and --env-pass names",
			run: func(t *testing.T, fake *runner.Fake) {
				t.Helper()
				opts := GradeOptions{AllowExec: true, Runner: fake, Environ: parent, EnvPass: []string{"PROJECT_FLAG"}}
				assert.Empty(t, runCommandAssertion(&Assertion{Type: AssertCommandExit, Command: "true"}, t.TempDir(), opts))
			},
			want:    []string{"PATH=/bin", "HOME=/home/u", "CI=true", "PROJECT_FLAG=1"},
			dropped: []string{"GITHUB_TOKEN", "AWS_SECRET_ACCESS_KEY", "ANTHROPIC_API_KEY"},
		},
		{
			name: "claude plugin eval keeps only what claude needs",
			run: func(t *testing.T, fake *runner.Fake) {
				t.Helper()
				env := ambient.MapEnv{Vars: map[string]string{}}
				for _, kv := range parent {
					k, v, _ := strings.Cut(kv, "=")
					env.Vars[k] = v
				}
				exec := execThrough(fake, claudeChildEnv(env))
				_ = exec(context.Background(), "claude", []string{"plugin", "eval"}, nil, nil)
			},
			want:    []string{"PATH=/bin", "HOME=/home/u", "ANTHROPIC_API_KEY=k"},
			dropped: []string{"GITHUB_TOKEN", "AWS_SECRET_ACCESS_KEY", "PROJECT_FLAG", "CI="},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			fake := &runner.Fake{}

			// Act
			tt.run(t, fake)

			// Assert
			require.Len(t, fake.Calls(), 1)
			spec := fake.Calls()[0]
			assert.False(t, spec.InheritEnv, "the parent environment is never inherited whole")
			env := strings.Join(spec.Env, "\n")
			for _, w := range tt.want {
				assert.Contains(t, env, w)
			}
			for _, d := range tt.dropped {
				assert.NotContains(t, env, d)
			}
		})
	}
}

func TestCommandAssertionExitStatus(t *testing.T) {
	two := 2
	tests := []struct {
		name string
		a    Assertion
		want string
	}{
		{"zero exit passes", Assertion{Command: "exit 0"}, ""},
		{"expected non-zero passes", Assertion{Command: "exit 2", ExitCode: &two}, ""},
		{"unexpected exit fails", Assertion{Command: "exit 3"}, "exit status 3, want 0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange and Act
			got := runCommandAssertion(&tt.a, t.TempDir(), GradeOptions{AllowExec: true})
			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewGitFuncUsesTheRunner(t *testing.T) {
	// Arrange
	fake := &runner.Fake{Handle: func(runner.Spec) runner.Result {
		return runner.Result{Status: runner.StatusOK, Stdout: []byte("/top\n")}
	}}

	// Act
	out, err := NewGitFunc(fake)("/repo", "rev-parse", "--show-toplevel")

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "/top\n", out)
	assert.Equal(t, []string{"git", "-C", "/repo", "rev-parse", "--show-toplevel"}, fake.Calls()[0].Argv)
}
