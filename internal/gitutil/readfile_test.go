package gitutil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func TestReadIgnoreFile(t *testing.T) {
	dir := t.TempDir()
	regular := filepath.Join(dir, "regular")
	require.NoError(t, os.WriteFile(regular, []byte("a\nb\n"), 0o644))
	big := filepath.Join(dir, "big")
	require.NoError(t, os.WriteFile(big, []byte("0123456789"), 0o644))
	link := filepath.Join(dir, "link")
	testutil.SymlinkOrSkip(t, regular, link)
	old := maxIgnoreFileSize
	t.Cleanup(func() { maxIgnoreFileSize = old })

	tests := []struct {
		name    string
		path    string
		limit   int64
		want    string
		wantErr error
		missing bool
	}{
		{"regular file", regular, 100, "a\nb\n", nil, false},
		{"symlink to regular file", link, 100, "a\nb\n", nil, false},
		{"over the size limit", big, 5, "", ErrTooLarge, false},
		{"missing", filepath.Join(dir, "nope"), 100, "", nil, true},
	}
	if runtime.GOOS != "windows" { // Windows has no device files to link to
		devLink := filepath.Join(dir, "dev")
		testutil.SymlinkOrSkip(t, os.DevNull, devLink)
		tests = append(tests, struct {
			name    string
			path    string
			limit   int64
			want    string
			wantErr error
			missing bool
		}{"symlink to a device", devLink, 100, "", ErrNotRegular, false})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			maxIgnoreFileSize = tt.limit

			// Act
			data, err := ReadIgnoreFile(tt.path)

			// Assert
			switch {
			case tt.missing:
				assert.True(t, os.IsNotExist(err))
			case tt.wantErr != nil:
				assert.ErrorIs(t, err, tt.wantErr)
			default:
				require.NoError(t, err)
				assert.Equal(t, tt.want, string(data))
			}
			lenient, lenientErr := ReadIgnoreFileOrEmpty(tt.path)
			if tt.wantErr != nil {
				assert.NoError(t, lenientErr)
				assert.Empty(t, lenient)
			}
		})
	}
}
