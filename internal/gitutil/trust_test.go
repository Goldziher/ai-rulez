package gitutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUntrustedLocalFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits")
	}
	tests := []struct {
		name  string
		setup func(t *testing.T, path string)
		want  string
	}{
		{"missing file is trusted", func(*testing.T, string) {}, ""},
		{"private untracked file is trusted", func(t *testing.T, p string) {
			require.NoError(t, os.WriteFile(p, []byte("{}"), 0o600))
		}, ""},
		{"group writable", func(t *testing.T, p string) {
			require.NoError(t, os.WriteFile(p, []byte("{}"), 0o600))
			require.NoError(t, os.Chmod(p, 0o660))
		}, "it is writable by group or others"},
		{"symlink", func(t *testing.T, p string) {
			target := filepath.Join(filepath.Dir(p), "target")
			require.NoError(t, os.WriteFile(target, []byte("{}"), 0o600))
			require.NoError(t, os.Symlink(target, p))
		}, "it is not a regular file"},
		{"tracked by git", func(t *testing.T, p string) {
			require.NoError(t, os.WriteFile(p, []byte("{}"), 0o600))
			cmd := exec.Command("git", "add", "-f", filepath.Base(p))
			cmd.Dir = filepath.Dir(p)
			out, err := cmd.CombinedOutput()
			require.NoError(t, err, string(out))
		}, "git tracks it"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			cmd := exec.Command("git", "init", "-q")
			cmd.Dir = dir
			require.NoError(t, cmd.Run())
			path := filepath.Join(dir, "state.json")
			tt.setup(t, path)

			// Act
			got := UntrustedLocalFile(path)

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}
