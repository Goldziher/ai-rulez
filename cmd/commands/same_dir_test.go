package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

func TestSameDir(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "bundle")
	other := filepath.Join(root, "other")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.MkdirAll(other, 0o755))
	link := filepath.Join(root, "link")
	testutil.SymlinkOrSkip(t, dir, link)
	t.Chdir(root)

	tests := []struct {
		name string
		a, b string
		want bool
	}{
		{"identical", dir, dir, true},
		{"trailing separator and dots", dir + string(filepath.Separator), filepath.Join(root, "x", "..", "bundle"), true},
		{"relative against absolute", "bundle", dir, true},
		{"through a symlink", link, dir, true},
		{"different directories", dir, other, false},
		{"empty", "", dir, false},
		{"missing directories compare by cleaned path", filepath.Join(root, "nope"), filepath.Join(root, "nope", "."), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, sameDir(tt.a, tt.b))
		})
	}
}
