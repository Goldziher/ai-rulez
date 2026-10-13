package commands

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// recordingLogger keeps the warnings a flush replays.
type recordingLogger struct {
	mu    sync.Mutex
	lines []string
}

func (r *recordingLogger) record(level, msg string, args []any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines = append(r.lines, fmt.Sprint(level, " ", msg, " ", args))
}
func (r *recordingLogger) Debug(msg string, args ...any) { r.record("debug", msg, args) }
func (r *recordingLogger) Info(msg string, args ...any)  { r.record("info", msg, args) }
func (r *recordingLogger) Warn(msg string, args ...any)  { r.record("warn", msg, args) }
func (r *recordingLogger) Error(msg string, args ...any) { r.record("error", msg, args) }

func TestRootNotes_FlushKeepsRecordedOrderAndMixesLines(t *testing.T) {
	// Arrange
	ctx, notes := withRootNotes(context.Background())
	var out bytes.Buffer
	rec := &recordingLogger{}

	// Act
	notes.Warn("first", "k", "v")
	noteLine(ctx, &out, "raw line\n")
	notes.Info("third")
	notes.flush(rec, &out)
	notes.flush(rec, &out) // a second flush prints nothing

	// Assert
	assert.Equal(t, []string{"warn first [k v]", "info third []"}, rec.lines)
	assert.Equal(t, "raw line\n", out.String())
}

func TestNoteLine_WritesImmediatelyOutsideARoot(t *testing.T) {
	var out bytes.Buffer
	noteLine(context.Background(), &out, "now\n")
	assert.Equal(t, "now\n", out.String())
}

// Warnings raised by the workers of a recursive drift check come out in root
// order, run after run, however the workers finish.
func TestRecursiveDrift_WarningsAreReportedInRootOrder(t *testing.T) {
	// Arrange: every root says gitignore = false, so --gitignore warns for each.
	base := t.TempDir()
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "secret.md"), "# secret\n")
	const n = 12
	var paths []string
	for i := range n {
		dir := filepath.Join(base, fmt.Sprintf("r%02d", i))
		cfg := filepath.Join(dir, ".ai-rulez", "config.toml")
		writeFile(t, cfg, "version = \"5.0\"\nname = \"r\"\npresets = [\"claude\"]\ngitignore = false\nagents_md = false\n")
		if i%3 == 0 { // some roots also refuse a symlinked rule, written as a raw stderr line
			require.NoError(t, os.MkdirAll(filepath.Join(dir, ".ai-rulez", "rules"), 0o755))
			testutil.SymlinkOrSkip(t, filepath.Join(outside, "secret.md"), filepath.Join(dir, ".ai-rulez", "rules", "leak.md"))
		}
		paths = append(paths, cfg)
	}
	t.Cleanup(func() { updateGitignore, recursive = false, false })
	updateGitignore, recursive = true, true
	var wantRoots []string // the roots that load, in order; a refused symlink fails the load
	for i := range n {
		if i%3 != 0 {
			wantRoots = append(wantRoots, filepath.Join(base, fmt.Sprintf("r%02d", i)))
		}
	}
	want := ""

	for run := range 5 {
		// Act
		rec := &recordingLogger{}
		var out bytes.Buffer
		ctx := context.Background()
		forEachRoot(paths, func(p string) driftRoot { return checkDriftRoot(ctx, p, nil, nil) },
			func(_ int, _ string, r driftRoot, _ time.Duration) {
				require.NotNil(t, r.notes)
				r.notes.flush(rec, &out)
			})

		// Assert
		require.Len(t, rec.lines, len(wantRoots), "one --gitignore warning per loaded root")
		for i, line := range rec.lines {
			assert.Contains(t, line, wantRoots[i], "run %d: warning %d is its root's", run, i)
		}
		got := out.String()
		if run == 0 {
			want = got
			assert.Contains(t, want, "refusing symlinked content")
		}
		assert.Equal(t, want, got, "raw lines are stable across runs")
	}
}
