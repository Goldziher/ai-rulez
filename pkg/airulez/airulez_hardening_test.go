package airulez_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/pkg/airulez"
)

// execRunner is a Runner an outside module could write: it only uses the public
// Spec and Result types, and records what it was asked to start.
type execRunner struct {
	mu    sync.Mutex
	calls [][]string
}

func (r *execRunner) Run(ctx context.Context, spec airulez.Spec) airulez.Result {
	r.mu.Lock()
	r.calls = append(r.calls, append([]string(nil), spec.Argv...))
	r.mu.Unlock()
	cmd := exec.CommandContext(ctx, spec.Argv[0], spec.Argv[1:]...) //nolint:gosec // test runner
	cmd.Dir = spec.Dir
	cmd.Env = append(os.Environ(), spec.Env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	res := airulez.Result{Status: airulez.StatusOK, Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	var exitErr *exec.ExitError
	switch {
	case errors.As(err, &exitErr):
		res.Status, res.ExitCode, res.Err = airulez.StatusExit, exitErr.ExitCode(), err
	case err != nil:
		res.Status, res.ExitCode, res.Err = airulez.StatusUnavailable, -1, err
	}
	return res
}

func (r *execRunner) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

func TestMachineLocalOverlayIsOptIn(t *testing.T) {
	tests := []struct {
		name      string
		withLocal bool
		wantName  string
	}{
		{name: "ignored by default", wantName: "svc"},
		{name: "read with WithLocal", withLocal: true, wantName: "overlaid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			writeSources(t, dir)
			require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", "config.local.toml"), []byte("name = \"overlaid\"\n"), 0o644))
			require.NoError(t, os.MkdirAll(filepath.Join(dir, ".ai-rulez", "local", "rules"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, ".ai-rulez", "local", "rules", "mine.md"), []byte("# Mine\n"), 0o644))
			disk, err := airulez.DirWorkspace(dir)
			require.NoError(t, err)

			// Act
			project, err := airulez.Load(t.Context(), airulez.Options{Workspace: disk, WithLocal: tt.withLocal})
			require.NoError(t, err)
			plan, err := project.Plan(t.Context(), airulez.PlanOptions{})
			require.NoError(t, err)

			// Assert
			assert.Equal(t, tt.wantName, project.Name())
			var localOutputs int
			for _, f := range plan.Files {
				if f.LocalOnly {
					localOutputs++
				}
			}
			assert.Equal(t, tt.withLocal, localOutputs > 0, "machine-local content renders only with WithLocal: %+v", plan.Files)
		})
	}
}

func TestStrictValidationStartsGitThroughTheOptionsRunner(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeSources(t, dir)
	commit(t, dir)
	disk, err := airulez.DirWorkspace(dir)
	require.NoError(t, err)
	run := &execRunner{}
	project, err := airulez.Load(t.Context(), airulez.Options{Workspace: disk, Runner: run})
	require.NoError(t, err)
	before := run.count()

	// Act
	_, err = project.Validate(t.Context(), airulez.ValidateOptions{Strict: true})

	// Assert
	require.NoError(t, err)
	assert.Greater(t, run.count(), before, "strict validation indexes tracked files through the injected runner")
}

func TestStrictValidationNeverStartsGitUnderDenyAll(t *testing.T) {
	// Arrange: a repository, and PATH without git, so a stray real git would fail.
	dir := t.TempDir()
	writeSources(t, dir)
	commit(t, dir)
	t.Setenv("PATH", t.TempDir())
	disk, err := airulez.DirWorkspace(dir)
	require.NoError(t, err)
	project, err := airulez.Load(t.Context(), airulez.Options{Workspace: disk})
	require.NoError(t, err)

	// Act
	report, err := project.Validate(t.Context(), airulez.ValidateOptions{Strict: true})

	// Assert
	require.NoError(t, err)
	require.NotNil(t, report)
}

func TestGenerateFailuresAreErrorValues(t *testing.T) {
	// Arrange: a directory where an output file belongs makes the write fail.
	dir := t.TempDir()
	writeSources(t, dir)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "CLAUDE.md", "child"), 0o755))
	disk, err := airulez.DirWorkspace(dir)
	require.NoError(t, err)
	project, err := airulez.Load(t.Context(), airulez.Options{Workspace: disk})
	require.NoError(t, err)

	// Act
	_, err = project.Generate(t.Context(), airulez.GenerateOptions{Mode: airulez.Write})

	// Assert
	var apiErr *airulez.Error
	require.ErrorAs(t, err, &apiErr, "every failure of Generate is an *Error")
	assert.Equal(t, airulez.CodeApply, apiErr.Code)
}

func TestErrorWithoutCauseDoesNotPanic(t *testing.T) {
	assert.Equal(t, airulez.CodeLoad, (&airulez.Error{Code: airulez.CodeLoad}).Error())
}
