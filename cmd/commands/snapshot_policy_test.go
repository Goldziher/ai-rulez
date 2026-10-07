package commands

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// dirRecorder is a policy that records which directory each load asks the
// organization policy about.
type dirRecorder struct {
	mu   sync.Mutex
	dirs []string
	base []string
}

func (r *dirRecorder) Enforce(_ context.Context, cfg *config.Config) (*config.PolicyOutcome, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dirs = append(r.dirs, cfg.PolicyProjectDir())
	r.base = append(r.base, cfg.BaseDir)
	return &config.PolicyOutcome{}, nil
}

func (*dirRecorder) Locks(string) bool { return false }

// A snapshot of an earlier revision is loaded from a temporary directory with no
// git remote: the organization policy must still be discovered from the project.
func TestCatalogDiffSnapshotLoadsDiscoverThePolicyFromTheProject(t *testing.T) {
	// Arrange
	root := catalogDiffRepo(t)
	resetCatalogDiffFlags(t)
	rec := &dirRecorder{}
	previous := activePolicy
	activePolicy = rec
	t.Cleanup(func() { activePolicy = previous })
	ctx := cmdContext()
	var out bytes.Buffer

	// Act
	_, err := runCatalogDiff(ctx, &out, []string{"HEAD~1", "HEAD"})

	// Assert
	require.NoError(t, err)
	want, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)
	snapshots := 0
	for i, base := range rec.base {
		if !strings.Contains(base, "ai-rulez-catalog-") {
			continue
		}
		snapshots++
		got, err := filepath.EvalSymlinks(rec.dirs[i])
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}
	assert.Positive(t, snapshots, "the revision was loaded from a snapshot directory")
}
