package govview

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/runner"
)

// recordingRunner runs for real and remembers the git subcommands.
type recordingRunner struct {
	mu   sync.Mutex
	cmds []string
}

func (r *recordingRunner) Run(ctx context.Context, spec runner.Spec) runner.Result {
	r.mu.Lock()
	r.cmds = append(r.cmds, strings.Join(spec.Argv, " "))
	r.mu.Unlock()
	return runner.Exec{}.Run(ctx, spec)
}

func TestExtractRevision_ReadsThroughTheContextRunnerInBatches(t *testing.T) {
	// Arrange
	dir, _, second := snapRepo(t)
	rec := &recordingRunner{}
	ctx := runner.WithContext(context.Background(), rec)

	// Act
	snap, err := ExtractRevision(ctx, dir, "HEAD", "svc/.ai-rulez", t.TempDir())

	// Assert
	require.NoError(t, err)
	assert.Equal(t, second, snap.Commit)
	assert.Equal(t, 3, snap.Files)
	var batches, blobs int
	for _, c := range rec.cmds {
		batches += strings.Count(c, "cat-file --batch")
		blobs += strings.Count(c, "cat-file blob")
	}
	assert.Equal(t, 1, batches, "every file arrives in one batch run: %v", rec.cmds)
	assert.Zero(t, blobs, "no per-file process: %v", rec.cmds)
}

func TestExtractRevision_LimitsApplyBeforeAnyContentIsWritten(t *testing.T) {
	// Arrange: svc/.ai-rulez holds three files of 4 bytes each at HEAD.
	dir, _, _ := snapRepo(t)
	tests := []struct {
		name string
		lim  snapshotLimits
		want string
	}{
		{"file count", snapshotLimits{files: 2, fileSize: 1 << 20, total: 1 << 20}, "too large"},
		{"file size", snapshotLimits{files: 10, fileSize: 3, total: 1 << 20}, "too large"},
		{"total size", snapshotLimits{files: 10, fileSize: 1 << 20, total: 10}, "too large"},
		{"within the limits", snapshotLimits{files: 3, fileSize: 1 << 20, total: 1 << 20}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dest := t.TempDir()

			// Act
			_, err := extractRevision(context.Background(), tt.lim, dir, "HEAD", "svc/.ai-rulez", dest)

			// Assert
			if tt.want == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
			assert.NoDirExists(t, dest+"/svc", "nothing is written when a limit is hit")
		})
	}
}
