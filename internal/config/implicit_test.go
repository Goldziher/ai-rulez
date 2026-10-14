package config

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadImplicitInAnEmptyDirectory(t *testing.T) {
	root := t.TempDir()

	cfg, err := LoadImplicit(context.Background(), root)

	require.NoError(t, err)
	assert.True(t, cfg.Implicit)
	assert.Empty(t, cfg.Presets, "an implicit configuration generates nothing")
	assert.Empty(t, cfg.Includes)
	assert.Empty(t, cfg.ConfigPath(), "there is no config file to name")
	assert.Equal(t, cfg.BaseDir, cfg.AuthoredRoot(), "everything under the root is hand-written")
	require.NotNil(t, cfg.Content)
	assert.Empty(t, cfg.Content.Rules)
	assert.NoError(t, cfg.Validate())
}

func TestLoadImplicitReadsAContentTreeWithoutConfigToml(t *testing.T) {
	root := t.TempDir()
	writeConfigFile(t, filepath.Join(root, ".ai-rulez", "rules", "style.md"), "# Style\n")
	writeConfigFile(t, filepath.Join(root, ".ai-rulez", "skills", "deploy", "SKILL.md"), "---\nname: deploy\ndescription: Deploy it.\n---\nbody\n")

	cfg, err := LoadImplicit(context.Background(), root)

	require.NoError(t, err)
	require.Len(t, cfg.Content.Rules, 1)
	assert.Equal(t, "style", cfg.Content.Rules[0].Name)
	require.Len(t, cfg.Content.Skills, 1)
	assert.Equal(t, "deploy", cfg.Content.Skills[0].Name)
}

func TestLoadConfigWithoutAConfigIsErrNoConfig(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
	}{
		{name: "empty directory"},
		{name: "config directory with no config file", files: map[string]string{".ai-rulez/rules/a.md": "# a\n"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for name, body := range tt.files {
				writeConfigFile(t, filepath.Join(root, filepath.FromSlash(name)), body)
			}

			_, err := LoadConfig(context.Background(), root)

			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrNoConfig), "got %v", err)
		})
	}
}

func TestLoadConfigKeepsOtherFailuresDistinctFromErrNoConfig(t *testing.T) {
	root := t.TempDir()
	writeConfigFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), "version = \"5.0\"\nname = [\n")

	_, err := LoadConfig(context.Background(), root)

	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrNoConfig), "a config that fails to parse is not a missing one")
}
