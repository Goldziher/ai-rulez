package config

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

// captureStderr runs fn and returns what it wrote to os.Stderr.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	saved := os.Stderr
	os.Stderr = w
	fn()
	os.Stderr = saved
	require.NoError(t, w.Close())
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	return string(out)
}

func TestScanner_WithoutAHostWritesNothingToStderr(t *testing.T) {
	tests := []struct {
		name string
		// scan builds the scanner and the tree it scans, with a refused symlink in it.
		scan         func(t *testing.T) *contentScanner
		wantProblems int
	}{
		{"project content", func(t *testing.T) *contentScanner {
			project, outside := t.TempDir(), t.TempDir()
			write(t, filepath.Join(outside, "secret.md"), "# secret\n")
			require.NoError(t, os.MkdirAll(filepath.Join(project, ".ai-rulez", "rules"), 0o755))
			symlinkOrSkip(t, filepath.Join(outside, "secret.md"), filepath.Join(project, ".ai-rulez", "rules", "leak.md"))
			s := newProjectScanner(t.Context(), osView(project))
			_, err := scanContentTree(s, filepath.Join(project, ".ai-rulez"), nil)
			require.NoError(t, err)
			return s
		}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			var s *contentScanner
			stderr := captureStderr(t, func() { s = tt.scan(t) })

			// Assert
			assert.Empty(t, stderr, "a library user without a host chose no stream to write to")
			assert.Len(t, s.problems, tt.wantProblems, "the refusal is still recorded for the caller")
		})
	}
}

func TestIncludeScanner_RefusalsAreReportedAtErrorLevel(t *testing.T) {
	// Arrange: an include whose rules directory is a symlink
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "rules")
	write(t, filepath.Join(target, "a.md"), "# a\n")
	symlinkOrSkip(t, target, filepath.Join(root, "rules"))
	rec := &testutil.LogRecorder{}
	s := newIncludeScanner(t.Context(), osView(root))
	s.log = rec

	// Act
	_, err := scanContentTree(s, root, nil)

	// Assert
	require.NoError(t, err)
	assert.Len(t, rec.Level("ERROR"), 1, "%v", rec.String())
	assert.Contains(t, rec.Level("ERROR")[0], "included content")
	assert.Empty(t, rec.Level("WARN"), "--quiet hides warnings, so the refusal is not one")
}
