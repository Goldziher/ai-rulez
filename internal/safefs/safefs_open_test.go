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
	}{
		{"regular file", regular, ""},
		{"symlink", link, "symlink"},
		{"directory", dir, "regular file"},
		{"missing", filepath.Join(dir, "none"), "no such file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := OpenRegular(tt.path)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.NoError(t, f.Close())
		})
	}
}
