package gitutil

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

func TestGitRunsThroughTheInjectedRunner(t *testing.T) {
	tests := []struct {
		name     string
		answer   runner.Result
		call     func(Git) any
		wantArgv []string
		want     any
	}{
		{
			name:     "IsRepo is true when git prints true",
			answer:   runner.Result{Status: runner.StatusOK, Stdout: []byte("true\n")},
			call:     func(g Git) any { return g.IsRepo("/work") },
			wantArgv: []string{"git", "-C", "/work", "rev-parse", "--is-inside-work-tree"},
			want:     true,
		},
		{
			name:     "IsRepo is false when git is not installed",
			answer:   runner.Result{Status: runner.StatusUnavailable, ExitCode: -1, Err: errors.New("not found")},
			call:     func(g Git) any { return g.IsRepo("/work") },
			wantArgv: []string{"git", "-C", "/work", "rev-parse", "--is-inside-work-tree"},
			want:     false,
		},
		{
			name:     "TopLevel returns the cleaned path",
			answer:   runner.Result{Status: runner.StatusOK, Stdout: []byte("/repo/\n")},
			call:     func(g Git) any { return g.TopLevel("/repo/sub") },
			wantArgv: []string{"git", "-C", "/repo/sub", "rev-parse", "--show-toplevel"},
			want:     "/repo",
		},
		{
			name:     "TopLevel is empty outside a repository",
			answer:   runner.Result{Status: runner.StatusExit, ExitCode: 128, Err: errors.New("exit status 128"), Stderr: []byte("fatal: not a git repository")},
			call:     func(g Git) any { return g.TopLevel("/tmp") },
			wantArgv: []string{"git", "-C", "/tmp", "rev-parse", "--show-toplevel"},
			want:     "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			fake := &runner.Fake{Handle: func(runner.Spec) runner.Result { return tt.answer }}
			// Act
			got := tt.call(New(fake))
			// Assert
			if got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			calls := fake.Calls()
			if len(calls) != 1 || strings.Join(calls[0].Argv, " ") != strings.Join(tt.wantArgv, " ") {
				t.Fatalf("argv = %v, want %v", calls, tt.wantArgv)
			}
			for _, kv := range calls[0].Env {
				if strings.HasPrefix(kv, "GIT_DIR=") {
					t.Fatalf("the environment selects a repository: %s", kv)
				}
			}
		})
	}
}

func TestDenyRunnerStopsEveryGitQuestion(t *testing.T) {
	// Arrange
	g := New(runner.Deny{})
	// Act and Assert
	if g.IsRepo(t.TempDir()) {
		t.Fatal("IsRepo must be false when commands are denied")
	}
	if _, err := g.TrackedAmong(t.TempDir(), []string{"a"}); err != nil {
		t.Fatalf("TrackedAmong outside a repository is not an error: %v", err)
	}
	res := g.Exec(context.Background(), "", nil, "version")
	if err := ResultErr(res); err == nil {
		t.Fatal("Exec must report the denial")
	}
}
