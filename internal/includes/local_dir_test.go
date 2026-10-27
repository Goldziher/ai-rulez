package includes

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func TestCheckLocalDir(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "shared", "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "shared", "file.txt"), []byte("x"), 0o644))
	testutil.SymlinkOrSkip(t, outside, filepath.Join(root, "shared", "escape"))
	tests := []struct {
		name    string
		source  string
		path    string
		wantErr string
	}{
		{"source only", "shared", "", ""},
		{"existing sub-path", "shared", "sub", ""},
		{"missing sub-path", "shared", "nope", "does not exist"},
		{"missing source", "gone", "", "does not exist"},
		{"file sub-path", "shared", "file.txt", "not a directory"},
		{"climbing sub-path", "shared", "../x", "leaves the include source"},
		{"symlink out of the project", "shared", "escape", "outside the project"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inc := &config.IncludeConfig{Name: "inc", Source: tt.source, Path: tt.path}

			err := CheckLocalDir(nil, root, inc)

			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
