package lint

import (
	"context"
	"os/exec"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
	procrunner "github.com/Goldziher/ai-rulez/v5/internal/runner"
)

// lsFilesRunner counts the `git ls-files` runs that reach it.
type lsFilesRunner struct{ calls *atomic.Int32 }

func (r lsFilesRunner) Run(ctx context.Context, spec procrunner.Spec) procrunner.Result {
	if slices.Contains(spec.Argv, "ls-files") {
		r.calls.Add(1)
	}
	return procrunner.Exec{}.Run(ctx, spec)
}

// The candidate listing of a run goes through the runner of the context, as the
// other git calls of a lint run do, so a caller that supplies one controls it.
func TestCandidatesListFilesThroughTheContextRunner(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	require.NoError(t, gitutil.CommandNoContext(root, "init", "-q").Run())
	writeFiles(t, root, map[string]string{"AGENTS.md": "# a\n"})
	tree, err := LoadTree(root)
	require.NoError(t, err)
	require.True(t, tree.Git)
	calls := &atomic.Int32{}
	ctx := procrunner.WithContext(context.Background(), lsFilesRunner{calls: calls})

	got := Candidates(ctx, tree)

	assert.Contains(t, got, "AGENTS.md")
	assert.Positive(t, calls.Load(), "git ls-files ran through the context runner")
}
