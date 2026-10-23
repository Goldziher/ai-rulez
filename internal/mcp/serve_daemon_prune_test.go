//go:build !windows

package mcp

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPruneStaleDaemonDirs(t *testing.T) {
	parent := t.TempDir()
	mk := func(name string, files ...string) string {
		dir := filepath.Join(parent, name)
		require.NoError(t, os.MkdirAll(dir, 0o700))
		for _, f := range files {
			require.NoError(t, os.WriteFile(filepath.Join(dir, f), nil, 0o600))
		}
		return dir
	}
	keep := mk("keep", "daemon.log")
	logOnly := mk("log-only", "daemon.log")
	empty := mk("empty")
	foreign := mk("foreign", "daemon.log", "notes.txt")
	fresh := mk("fresh", "daemon.log")
	old := time.Now().Add(-48 * time.Hour)
	for _, d := range []string{keep, logOnly, empty, foreign} {
		require.NoError(t, os.Chtimes(d, old, old))
	}

	pruneStaleDaemonDirs(parent, keep, 24*time.Hour, time.Now())

	assert.DirExists(t, keep, "the daemon's own directory stays")
	assert.NoDirExists(t, logOnly)
	assert.NoDirExists(t, empty)
	assert.DirExists(t, foreign, "a directory with other files is not ours to remove")
	assert.DirExists(t, fresh, "a recently used directory stays")
}
