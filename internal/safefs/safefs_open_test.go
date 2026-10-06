package safefs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenRegular(t *testing.T) {
	dir := t.TempDir()
	regular := filepath.Join(dir, "regular")
	require.NoError(t, os.WriteFile(regular, []byte("x"), 0o600))
	link := filepath.Join(dir, "link")
	require.NoError(t, os.Symlink(regular, link))

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
