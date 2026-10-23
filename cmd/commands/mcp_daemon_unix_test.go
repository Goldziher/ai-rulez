//go:build !windows

package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenDaemonLog_CreatesAPrivateLogAndRestartsAnOversizedOne(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "d")
	socket := filepath.Join(dir, "daemon.sock")

	f := openDaemonLog(socket)
	require.NotNil(t, f)
	_, err := f.WriteString("first\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	info, err := os.Stat(filepath.Join(dir, "daemon.log"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	require.NoError(t, os.WriteFile(filepath.Join(dir, "daemon.log"), make([]byte, maxDaemonLogBytes+1), 0o600))
	f = openDaemonLog(socket)
	require.NotNil(t, f)
	require.NoError(t, f.Close())
	info, err = os.Stat(filepath.Join(dir, "daemon.log"))
	require.NoError(t, err)
	assert.Zero(t, info.Size(), "an oversized log is started afresh")
}
