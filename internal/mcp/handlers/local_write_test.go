package handlers

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateConfigHandler_LocalWritesOnlyTheOverlay(t *testing.T) {
	tests := []struct {
		name     string
		args     map[string]any
		wantInfo string
	}{
		{"description", map[string]any{"description": "mine"}, "description"},
		{"default effort", map[string]any{"default_effort": "high"}, "effort"},
		{"effort by preset", map[string]any{"default_effort_by_preset": map[string]any{"claude": "max"}}, "claude"},
		{"rules mode", map[string]any{"rules_mode": "inline"}, "inline"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			writeMinimalConfig(t, dir)
			cfgDir := filepath.Join(dir, ".ai-rulez")
			sharedBefore, err := os.ReadFile(filepath.Join(cfgDir, "config.toml"))
			require.NoError(t, err)
			args := map[string]any{"working_directory": dir, "local": true}
			for k, v := range tt.args {
				args[k] = v
			}

			// Act
			res, err := UpdateConfigHandler(context.Background(), newRequestWithArgs(args))

			// Assert
			require.NoError(t, err)
			require.False(t, res.IsError, textOf(t, res))
			sharedAfter, err := os.ReadFile(filepath.Join(cfgDir, "config.toml"))
			require.NoError(t, err)
			assert.Equal(t, string(sharedBefore), string(sharedAfter), "shared config must be untouched")
			local, err := os.ReadFile(filepath.Join(cfgDir, "config.local.toml"))
			require.NoError(t, err)
			assert.Contains(t, string(local), tt.wantInfo)
		})
	}
}

func TestUpdateConfigHandler_LocalClearRemovesTheOverride(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeMinimalConfig(t, dir)
	set := newRequestWithArgs(map[string]any{"working_directory": dir, "local": true, "default_effort": "high"})
	_, err := UpdateConfigHandler(context.Background(), set)
	require.NoError(t, err)

	// Act
	clear := newRequestWithArgs(map[string]any{"working_directory": dir, "local": true, "default_effort": ""})
	res, err := UpdateConfigHandler(context.Background(), clear)

	// Assert
	require.NoError(t, err)
	require.False(t, res.IsError, textOf(t, res))
	local, err := os.ReadFile(filepath.Join(dir, ".ai-rulez", "config.local.toml"))
	require.NoError(t, err)
	assert.NotContains(t, string(local), "high")
}

func TestUpdateConfigHandler_LocalRejectsInvalidValue(t *testing.T) {
	dir := t.TempDir()
	writeMinimalConfig(t, dir)

	res, err := UpdateConfigHandler(context.Background(), newRequestWithArgs(map[string]any{
		"working_directory": dir, "local": true, "default_effort": "turbo",
	}))

	require.NoError(t, err)
	assert.True(t, res.IsError)
	assert.NoFileExists(t, filepath.Join(dir, ".ai-rulez", "config.local.toml"), "a rejected change leaves no overlay behind")
}

func TestAddProfileHandler_Local(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	writeMinimalConfig(t, dir)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".ai-rulez", "domains", "backend", "rules"), 0o755))
	shared, err := os.ReadFile(filepath.Join(dir, ".ai-rulez", "config.toml"))
	require.NoError(t, err)

	// Act
	res, err := AddProfileHandler(context.Background(), newRequestWithArgs(map[string]any{
		"working_directory": dir, "name": "mine", "domains": []any{"backend"}, "local": true,
	}))

	// Assert
	require.NoError(t, err)
	require.False(t, res.IsError, textOf(t, res))
	after, err := os.ReadFile(filepath.Join(dir, ".ai-rulez", "config.toml"))
	require.NoError(t, err)
	assert.Equal(t, string(shared), string(after))
	local, err := os.ReadFile(filepath.Join(dir, ".ai-rulez", "config.local.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(local), "mine")
}
