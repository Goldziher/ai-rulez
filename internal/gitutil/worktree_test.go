package gitutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.email=t@example.com", "-c", "user.name=t"}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

func TestIsLinkedWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	main := filepath.Join(root, "main")
	require.NoError(t, os.MkdirAll(main, 0o755))
	gitRun(t, main, "init", "-q")
	require.NoError(t, os.WriteFile(filepath.Join(main, "f"), []byte("x"), 0o644))
	gitRun(t, main, "add", "f")
	gitRun(t, main, "commit", "-q", "-m", "init")
	linked := filepath.Join(root, "linked")
	gitRun(t, main, "worktree", "add", "-q", linked, "-b", "wt")

	tests := []struct {
		name string
		dir  string
		want bool
	}{
		{"main checkout", main, false},
		{"linked worktree", linked, true},
		{"not a repository", root, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsLinkedWorktree(tt.dir))
		})
	}
}
