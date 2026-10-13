package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// git names a repository by its real path while the working directory may be
// spelled through a symlink (macOS /var, a Windows 8.3 short name): the drift
// path must not climb out and back in.
func TestDisplayDriftAbs_SameDirectoryReachedThroughASymlink(t *testing.T) {
	// Arrange
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	testutil.SymlinkOrSkip(t, real, link)
	require.NoError(t, os.WriteFile(filepath.Join(real, ".gitignore"), nil, 0o644))
	chdir(t, link)
	t.Setenv("PWD", link) // os.Getwd reports the spelling the shell used

	tests := []struct{ name, abs, want string }{
		{name: "an existing file", abs: filepath.Join(real, ".gitignore"), want: ".gitignore"},
		{name: "a file not written yet", abs: filepath.Join(real, "sub", "new.txt"), want: "sub/new.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			got := displayDriftAbs(tt.abs, "fallback")

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}
