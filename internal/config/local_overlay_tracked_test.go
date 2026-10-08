package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalOverlayTracked(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	tests := []struct {
		name string
		add  bool
		want bool
	}{
		{"untracked overlay", false, false},
		{"git-tracked overlay", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			cfgDir := filepath.Join(dir, ".ai-rulez")
			require.NoError(t, os.MkdirAll(cfgDir, 0o755))
			local := filepath.Join(cfgDir, "config.local.toml")
			require.NoError(t, os.WriteFile(local, []byte("[[includes]]\nname = \"x\"\n"), 0o600))
			git := func(args ...string) {
				cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
				out, err := cmd.CombinedOutput()
				require.NoError(t, err, string(out))
			}
			git("init", "-q")
			if tt.add {
				git("add", "-f", ".ai-rulez/config.local.toml")
			}

			// Act
			got := isGitTracked(dir, local)

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}
