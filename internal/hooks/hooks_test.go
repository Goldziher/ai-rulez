package hooks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func chdirTemp(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	t.Cleanup(func() { _ = os.Chdir(origDir) })
	return tmpDir
}

func TestDetectGitHooks(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T, dir string)
		expected string
	}{
		{
			name:     "no hook system detected",
			setup:    func(t *testing.T, dir string) {},
			expected: "",
		},
		{
			name: "detects lefthook.yml",
			setup: func(t *testing.T, dir string) {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "lefthook.yml"), []byte(""), 0o644))
			},
			expected: "lefthook",
		},
		{
			name: "detects lefthook.yaml",
			setup: func(t *testing.T, dir string) {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "lefthook.yaml"), []byte(""), 0o644))
			},
			expected: "lefthook",
		},
		{
			name: "detects .lefthook.yml",
			setup: func(t *testing.T, dir string) {
				require.NoError(t, os.WriteFile(filepath.Join(dir, ".lefthook.yml"), []byte(""), 0o644))
			},
			expected: "lefthook",
		},
		{
			name: "detects .pre-commit-config.yaml",
			setup: func(t *testing.T, dir string) {
				require.NoError(t, os.WriteFile(filepath.Join(dir, ".pre-commit-config.yaml"), []byte(""), 0o644))
			},
			expected: "pre-commit",
		},
		{
			name: "detects pre-commit-config.yaml",
			setup: func(t *testing.T, dir string) {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "pre-commit-config.yaml"), []byte(""), 0o644))
			},
			expected: "pre-commit",
		},
		{
			name: "detects .husky directory",
			setup: func(t *testing.T, dir string) {
				require.NoError(t, os.MkdirAll(filepath.Join(dir, ".husky"), 0o755))
			},
			expected: "husky",
		},
		{
			name: "lefthook takes priority over pre-commit",
			setup: func(t *testing.T, dir string) {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "lefthook.yml"), []byte(""), 0o644))
				require.NoError(t, os.WriteFile(filepath.Join(dir, ".pre-commit-config.yaml"), []byte(""), 0o644))
			},
			expected: "lefthook",
		},
		{
			name: "pre-commit takes priority over husky",
			setup: func(t *testing.T, dir string) {
				require.NoError(t, os.WriteFile(filepath.Join(dir, ".pre-commit-config.yaml"), []byte(""), 0o644))
				require.NoError(t, os.MkdirAll(filepath.Join(dir, ".husky"), 0o755))
			},
			expected: "pre-commit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := chdirTemp(t)
			tt.setup(t, dir)
			assert.Equal(t, tt.expected, DetectGitHooks())
		})
	}
}

func TestGetHookSystemName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"lefthook", "Lefthook"},
		{"pre-commit", "Pre-commit"},
		{"husky", "Husky"},
		{"unknown", "Unknown"},
		{"", "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, GetHookSystemName(tt.input))
		})
	}
}
