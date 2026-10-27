//go:build windows

package gitutil

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestWriteFileAtomic_WaitsForAReaderThatHoldsTheTarget(t *testing.T) {
	// Arrange: another process (here, a handle without delete sharing, as git
	// holds .git/info/exclude while it reads it) keeps the target open briefly.
	path := filepath.Join(t.TempDir(), "exclude")
	require.NoError(t, os.WriteFile(path, []byte("old\n"), 0o600))
	name, err := windows.UTF16PtrFromString(path)
	require.NoError(t, err)
	h, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil,
		windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	require.NoError(t, err)
	released := make(chan struct{})
	go func() {
		time.Sleep(100 * time.Millisecond)
		_ = windows.CloseHandle(h) //nolint:errcheck // test cleanup
		close(released)
	}()

	// Act
	err = WriteFileAtomic(path, []byte("new\n"), 0o600)

	// Assert
	<-released
	require.NoError(t, err)
	got, err := os.ReadFile(path) //nolint:gosec // test temp path
	require.NoError(t, err)
	assert.Equal(t, "new\n", string(got))
}
