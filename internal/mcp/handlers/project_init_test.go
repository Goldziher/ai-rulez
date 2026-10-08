package handlers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func TestInitProjectHandler_WritesTheV4TOMLLayout(t *testing.T) {
	tests := []struct {
		name        string
		args        map[string]any
		wantPresets []string
	}{
		{"default preset", map[string]any{"project_name": "demo"}, []string{"claude"}},
		{"named providers", map[string]any{"project_name": "demo", "providers": []any{"claude", "cursor"}}, []string{"claude", "cursor"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			tt.args["working_directory"] = dir

			// Act
			res, err := InitProjectHandler(t.Context(), newRequestWithArgs(tt.args))

			// Assert
			require.NoError(t, err)
			require.False(t, res.IsError, textOf(t, res))
			cfgDir := filepath.Join(dir, ".ai-rulez")
			assert.Equal(t, filepath.Join(cfgDir, "config.toml"), resultPayload(t, res)["path"])
			assert.NoFileExists(t, filepath.Join(cfgDir, "config.yaml"))
			for _, sub := range []string{"rules", "context", "skills", "agents", "domains"} {
				assert.DirExists(t, filepath.Join(cfgDir, sub))
			}
			raw, err := os.ReadFile(filepath.Join(cfgDir, "config.toml"))
			require.NoError(t, err)
			assert.NotContains(t, string(raw), "ai-rules-v2")
			cfg, err := config.LoadConfig(t.Context(), dir)
			require.NoError(t, err)
			assert.Equal(t, "demo", cfg.Name)
			var got []string
			for _, p := range cfg.Presets {
				got = append(got, p.BuiltIn)
			}
			assert.Equal(t, tt.wantPresets, got)
		})
	}
}

func TestInitProjectHandler_RefusesToOverwriteAConfig(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".ai-rulez")
	require.NoError(t, os.MkdirAll(cfgDir, 0o755))
	existing := filepath.Join(cfgDir, "config.toml")
	require.NoError(t, os.WriteFile(existing, []byte("version = \"5.0\"\nname = \"keep\"\n"), 0o644))

	// Act
	res, err := InitProjectHandler(t.Context(), newRequestWithArgs(map[string]any{"working_directory": dir}))

	// Assert
	require.NoError(t, err)
	require.True(t, res.IsError)
	raw, err := os.ReadFile(existing)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "keep")
}

func TestInitProjectHandler_RejectsUnknownProviders(t *testing.T) {
	dir := t.TempDir()

	res, err := InitProjectHandler(t.Context(), newRequestWithArgs(map[string]any{
		"working_directory": dir,
		"providers":         []any{"cursor", "bogus"},
	}))

	require.NoError(t, err)
	require.True(t, res.IsError, "an unknown provider must not be dropped silently")
	assert.Contains(t, textOf(t, res), "bogus")
	assert.NoDirExists(t, filepath.Join(dir, ".ai-rulez"), "nothing is written when the request is invalid")
}

func TestInitProjectHandler_RejectsNonStringProviders(t *testing.T) {
	res, err := InitProjectHandler(t.Context(), newRequestWithArgs(map[string]any{
		"working_directory": t.TempDir(),
		"providers":         []any{"cursor", 7},
	}))

	require.NoError(t, err)
	require.True(t, res.IsError)
}

func TestInitProjectHandler_WritesTheRootIndex(t *testing.T) {
	dir := t.TempDir()

	res, err := InitProjectHandler(t.Context(), newRequestWithArgs(map[string]any{"working_directory": dir}))

	require.NoError(t, err)
	require.False(t, res.IsError, textOf(t, res))
	root, err := os.ReadFile(filepath.Join(dir, ".ai-rulez", "index.md"))
	require.NoError(t, err)
	assert.Contains(t, string(root), "okf_version")
}
