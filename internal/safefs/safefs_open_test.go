package safefs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenLockFile_HandleCanReadAndWrite(t *testing.T) {
	// Windows LockFileEx refuses an append-only handle (Access is denied); a
	// handle that can read is one it accepts.
	tests := []struct {
		name     string
		existing bool
	}{
		{"created", false},
		{"existing", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			path := filepath.Join(t.TempDir(), "log.lock")
			if tt.existing {
				require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))
			}
			// Act
			f, err := OpenLockFile(path)
			require.NoError(t, err)
			defer f.Close() //nolint:errcheck // test
			_, writeErr := f.WriteAt([]byte("y"), 0)
			buf := make([]byte, 1)
			_, readErr := f.ReadAt(buf, 0)
			// Assert
			require.NoError(t, writeErr)
			require.NoError(t, readErr)
			assert.Equal(t, "y", string(buf))
		})
	}
}

func TestOpenRegular(t *testing.T) {
	dir := t.TempDir()
	regular := filepath.Join(dir, "regular")
	require.NoError(t, os.WriteFile(regular, []byte("x"), 0o600))
	link := filepath.Join(dir, "link")
	testutil.SymlinkOrSkip(t, regular, link)

	tests := []struct {
		name    string
		path    string
		wantErr string
		// wantIs is checked instead of a message that differs between platforms.
		wantIs error
	}{
		{"regular file", regular, "", nil},
		{"symlink", link, "symlink", nil},
		{"directory", dir, "regular file", nil},
		{"missing", filepath.Join(dir, "none"), "", os.ErrNotExist},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := OpenRegular(tt.path)
			if tt.wantIs != nil {
				require.ErrorIs(t, err, tt.wantIs)
				return
			}
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.NoError(t, f.Close())
		})
	}
}
