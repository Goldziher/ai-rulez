//go:build !windows

package mcp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func TestOpenDaemonLog_CreatesAPrivateLogAndRestartsAnOversizedOne(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "d")
	socket := filepath.Join(dir, daemonSocketName)

	f := OpenDaemonLog(socket)
	require.NotNil(t, f)
	_, err := f.WriteString("first\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	info, err := os.Stat(filepath.Join(dir, "daemon.log"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	require.NoError(t, os.WriteFile(filepath.Join(dir, "daemon.log"), make([]byte, maxDaemonLogBytes+1), 0o600))
	f = OpenDaemonLog(socket)
	require.NotNil(t, f)
	require.NoError(t, f.Close())
	info, err = os.Stat(filepath.Join(dir, "daemon.log"))
	require.NoError(t, err)
	assert.Zero(t, info.Size(), "an oversized log is started afresh")
}

func TestOpenDaemonLog_RefusesUnsafeLocations(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root passes the ownership checks")
	}
	cases := []struct {
		name  string
		setup func(t *testing.T) string // returns the socket path
	}{
		{"a link at the log path", func(t *testing.T) string {
			dir := filepath.Join(t.TempDir(), "d")
			require.NoError(t, os.MkdirAll(dir, 0o700))
			target := filepath.Join(t.TempDir(), "victim")
			require.NoError(t, os.WriteFile(target, nil, 0o600))
			testutil.SymlinkOrSkip(t, target, filepath.Join(dir, "daemon.log"))
			return filepath.Join(dir, daemonSocketName)
		}},
		{"a directory open to others", func(t *testing.T) string {
			dir := filepath.Join(t.TempDir(), "d")
			require.NoError(t, os.MkdirAll(dir, 0o700))
			require.NoError(t, os.Chmod(dir, 0o777))
			return filepath.Join(dir, daemonSocketName)
		}},
		{"a temp-directory socket whose per-user parent is open to others", func(t *testing.T) string {
			parent := filepath.Join(t.TempDir(), "ai-rulez-user")
			require.NoError(t, os.MkdirAll(filepath.Join(parent, "hash"), 0o700))
			require.NoError(t, os.Chmod(parent, 0o777))
			return filepath.Join(parent, "hash", shortSocketName)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			socket := tc.setup(t)
			f := OpenDaemonLog(socket)
			if f != nil {
				_ = f.Close()
			}
			assert.Nil(t, f)
		})
	}
}
