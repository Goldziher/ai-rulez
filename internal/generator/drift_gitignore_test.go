package generator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func newPlainRepo(t *testing.T, gitignoreOn bool) string {
	t.Helper()
	root := t.TempDir()
	initGitRepo(t, root)
	dir := filepath.Join(root, ".ai-rulez")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "rules"), 0o755))
	cfg := "version = \"5.0\"\nname = \"plain\"\npresets = [\"claude\"]\n"
	if gitignoreOn {
		cfg += "gitignore = true\n"
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.toml"), []byte(cfg), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "rules", "r.md"), []byte("# r\n"), 0o644))
	return root
}

func gitignoreDriftOf(t *testing.T, root string) []Drift {
	t.Helper()
	cfg, err := config.LoadConfig(context.Background(), root)
	require.NoError(t, err)
	drift, err := NewGenerator(cfg).CheckDrift("default")
	require.NoError(t, err)
	return drift
}

func TestCheckDrift_ManagedGitignoreBlock(t *testing.T) {
	tests := []struct {
		name        string
		gitignoreOn bool
		mutate      func(t *testing.T, path string)
		want        []Drift
	}{
		{
			name: "in sync after generate", gitignoreOn: true,
			mutate: func(*testing.T, string) {},
		},
		{
			name: "emptied file", gitignoreOn: true,
			mutate: func(t *testing.T, path string) { require.NoError(t, os.WriteFile(path, nil, 0o644)) },
			want:   []Drift{{Path: ".gitignore", Kind: DriftMissing, Note: "managed block"}},
		},
		{
			name: "file removed", gitignoreOn: true,
			mutate: func(t *testing.T, path string) { require.NoError(t, os.Remove(path)) },
			want:   []Drift{{Path: ".gitignore", Kind: DriftMissing, Note: "managed block"}},
		},
		{
			name: "a line deleted from the block", gitignoreOn: true,
			mutate: func(t *testing.T, path string) {
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(path, []byte(strings.Replace(string(data), "CLAUDE.md\n", "", 1)), 0o644))
			},
			want: []Drift{{Path: ".gitignore", Kind: DriftStale, Note: "managed block"}},
		},
		{
			name: "hand lines outside the block are not drift", gitignoreOn: true,
			mutate: func(t *testing.T, path string) {
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(path, append([]byte("node_modules/\n"), data...), 0o644))
			},
		},
		{
			name: "block left behind with gitignore off", gitignoreOn: false,
			mutate: func(t *testing.T, path string) {
				require.NoError(t, os.WriteFile(path, []byte("# BEGIN ai-rulez (DO NOT EDIT - managed by ai-rulez)\nAGENTS.md\n# END ai-rulez\n"), 0o644))
			},
			want: []Drift{{Path: ".gitignore", Kind: DriftStale, Note: "managed block"}},
		},
		{
			name: "gitignore off keeps only the local manifest, in sync", gitignoreOn: false,
			mutate: func(*testing.T, string) {},
		},
		{
			name: "gitignore off, local manifest entry deleted", gitignoreOn: false,
			mutate: func(t *testing.T, path string) { require.NoError(t, os.Remove(path)) },
			want:   []Drift{{Path: ".gitignore", Kind: DriftMissing, Note: "managed block"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := newPlainRepo(t, tt.gitignoreOn)
			generateRepo(t, root)
			path := filepath.Join(root, ".gitignore")

			// Act
			tt.mutate(t, path)
			got := gitignoreDriftOf(t, root)

			// Assert
			assert.Equal(t, tt.want, got)
		})
	}
}
