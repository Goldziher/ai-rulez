package generator

import (
	"context"
	"os/exec"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

// lsTreeRecorder runs real commands and records the arguments of each ls-tree.
type lsTreeRecorder struct {
	mu    sync.Mutex
	trees [][]string
}

func (r *lsTreeRecorder) Run(ctx context.Context, spec runner.Spec) runner.Result {
	for i, a := range spec.Argv {
		if a == "ls-tree" {
			r.mu.Lock()
			r.trees = append(r.trees, append([]string(nil), spec.Argv[i:]...))
			r.mu.Unlock()
		}
	}
	return runner.Run(ctx, spec)
}

func TestPluginVersionDriftListsOnlyTheFilesItCompares(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	// Arrange
	dir := newDomainsProject(t, driftTail("1.0.0"))
	require.NoError(t, loadDomainsProject(t, dir).GeneratePlugin(""))
	gitCmd(t, dir, "init", "-q")
	gitCmd(t, dir, "add", "-A")
	gitCmd(t, dir, "commit", "-q", "-m", "baseline")
	rec := &lsTreeRecorder{}
	g := loadDomainsProject(t, dir)
	g.SetHost(ambient.Host{Runner: rec})

	// Act
	_, err := g.PluginVersionDrift("")

	// Assert
	require.NoError(t, err)
	require.NotEmpty(t, rec.trees, "the baseline snapshot lists a tree")
	for _, args := range rec.trees {
		assert.Contains(t, args, "--", "the listing is restricted to paths, not the whole tree: %v", args)
	}
}
