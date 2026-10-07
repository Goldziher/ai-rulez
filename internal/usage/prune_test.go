package usage

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var pruneNow = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

func line(day int, id string) string {
	ts := pruneNow.AddDate(0, 0, -day).Format(time.RFC3339)
	return fmt.Sprintf(`{"v":3,"ts":%q,"event":"skill_invoked","skill":%q,"id":%q}`, ts, id, id)
}

func writeLog(t *testing.T, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "usage.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600))
	return path
}

func TestPruneLog(t *testing.T) {
	cutoff := pruneNow.AddDate(0, 0, -30)
	old1, old2, fresh := line(60, "old1"), line(45, "old2"), line(2, "fresh")
	tests := []struct {
		name     string
		lines    []string
		opts     PruneOptions
		want     []string
		wantRes  PruneResult
		rewrites bool
	}{
		{
			name:  "removes only lines older than the cutoff",
			lines: []string{old1, fresh, old2},
			opts:  PruneOptions{Cutoff: cutoff}, want: []string{fresh},
			wantRes:  PruneResult{Scanned: 3, Removed: 2, Kept: 1, RemovedBytes: int64(len(old1) + len(old2) + 2), Rewritten: true},
			rewrites: true,
		},
		{
			name:  "a protected line is kept however old",
			lines: []string{old1, old2, fresh},
			opts:  PruneOptions{Cutoff: cutoff, Protect: true, ProtectFrom: int64(len(old1) + 1)}, want: []string{old2, fresh},
			wantRes:  PruneResult{Scanned: 3, Removed: 1, Kept: 2, Protected: 1, RemovedBytes: int64(len(old1) + 1), Rewritten: true},
			rewrites: true,
		},
		{
			name:  "unreadable and timeless lines are kept",
			lines: []string{old1, "not json", `{"event":"skill_invoked"}`, `{"ts":"yesterday"}`, fresh},
			opts:  PruneOptions{Cutoff: cutoff}, want: []string{"not json", `{"event":"skill_invoked"}`, `{"ts":"yesterday"}`, fresh},
			wantRes:  PruneResult{Scanned: 5, Removed: 1, Kept: 4, Unreadable: 3, RemovedBytes: int64(len(old1) + 1), Rewritten: true},
			rewrites: true,
		},
		{
			name:  "nothing old leaves the file untouched",
			lines: []string{fresh}, opts: PruneOptions{Cutoff: cutoff}, want: []string{fresh},
			wantRes: PruneResult{Scanned: 1, Kept: 1},
		},
		{
			name:  "a dry run reports without rewriting",
			lines: []string{old1, fresh}, opts: PruneOptions{Cutoff: cutoff, DryRun: true}, want: []string{old1, fresh},
			wantRes: PruneResult{Scanned: 2, Removed: 1, Kept: 1, RemovedBytes: int64(len(old1) + 1)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			path := writeLog(t, tt.lines...)

			// Act
			res, err := PruneLog(path, tt.opts)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.wantRes, res)
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, strings.Join(tt.want, "\n")+"\n", string(data))
			if tt.rewrites && runtime.GOOS != "windows" { // Windows reports 0o666 for every file
				info, err := os.Stat(path)
				require.NoError(t, err)
				assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
			}
		})
	}
}

func TestPruneLog_NeverTouchesAPartialLastLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.jsonl")
	partial := `{"v":3,"ts":"2020-01-01T00:00:00Z","event":"skill_inv`
	require.NoError(t, os.WriteFile(path, []byte(line(60, "old")+"\n"+partial), 0o600))

	res, err := PruneLog(path, PruneOptions{Cutoff: pruneNow.AddDate(0, 0, -30)})

	require.NoError(t, err)
	assert.Equal(t, 1, res.Removed)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, partial, string(data), "a line a hook is still writing is kept verbatim")
}

func TestPruneLog_MissingLogIsAnError(t *testing.T) {
	_, err := PruneLog(filepath.Join(t.TempDir(), "nope.jsonl"), PruneOptions{Cutoff: pruneNow})
	require.Error(t, err)
}
