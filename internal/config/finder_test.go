package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeConfigFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func TestFindConfigFileInDirName_ConfigConvention(t *testing.T) {
	t.Run("finds .config/ai-rulez when .ai-rulez is absent", func(t *testing.T) {
		root := t.TempDir()
		want := filepath.Join(root, ".config", "ai-rulez", "config.toml")
		writeConfigFile(t, want, "version = \"4.0\"\nname = \"x\"\n")

		got, err := FindConfigFile(root)
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("prefers .ai-rulez over .config/ai-rulez in the same directory", func(t *testing.T) {
		root := t.TempDir()
		want := filepath.Join(root, ".ai-rulez", "config.toml")
		writeConfigFile(t, want, "version = \"4.0\"\nname = \"x\"\n")
		writeConfigFile(t, filepath.Join(root, ".config", "ai-rulez", "config.toml"), "version = \"4.0\"\nname = \"y\"\n")

		got, err := FindConfigFile(root)
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("walks up to a .config/ai-rulez in an ancestor", func(t *testing.T) {
		root := t.TempDir()
		want := filepath.Join(root, ".config", "ai-rulez", "config.toml")
		writeConfigFile(t, want, "version = \"4.0\"\nname = \"x\"\n")
		nested := filepath.Join(root, "a", "b", "c")
		require.NoError(t, os.MkdirAll(nested, 0o755))

		got, err := FindConfigFile(nested)
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("does not expand an explicit config directory name", func(t *testing.T) {
		root := t.TempDir()
		writeConfigFile(t, filepath.Join(root, ".config", "ai-rulez", "config.toml"), "version = \"4.0\"\nname = \"x\"\n")

		_, err := FindConfigFileInDirName(root, "custom-policy")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no configuration file found")
	})
}

func TestResolveConfigDirName(t *testing.T) {
	t.Run("returns .ai-rulez when present", func(t *testing.T) {
		root := t.TempDir()
		writeConfigFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), "version = \"4.0\"\nname = \"x\"\n")

		assert.Equal(t, aiRulezDirName, ResolveConfigDirName(root))
	})

	t.Run("falls back to .config/ai-rulez", func(t *testing.T) {
		root := t.TempDir()
		writeConfigFile(t, filepath.Join(root, ".config", "ai-rulez", "config.toml"), "version = \"4.0\"\nname = \"x\"\n")

		assert.Equal(t, altConfigDirName, ResolveConfigDirName(root))
	})

	t.Run("returns empty when neither exists", func(t *testing.T) {
		assert.Equal(t, "", ResolveConfigDirName(t.TempDir()))
	})
}

func TestIsConfigDirAncestor(t *testing.T) {
	cases := []struct {
		name          string
		path          string
		configDirName string
		want          bool
	}{
		{"top-level wrapper", ".config", ".config/ai-rulez", true},
		{"nested wrapper", "svc/.config", ".config/ai-rulez", true},
		{"the config dir itself is not an ancestor", ".config/ai-rulez", ".config/ai-rulez", false},
		{"unrelated dir", "src", ".config/ai-rulez", false},
		{"single-component target has no ancestors", ".ai-rulez", ".ai-rulez", false},
		{"single-component target on nested path", "svc/.ai-rulez", ".ai-rulez", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, IsConfigDirAncestor(tc.path, tc.configDirName))
		})
	}
}

func TestLoadConfig_ConfigConvention(t *testing.T) {
	t.Run("roots outputs in the project and names the nested config dir", func(t *testing.T) {
		root := t.TempDir()
		writeConfigFile(t, filepath.Join(root, ".config", "ai-rulez", "config.toml"),
			"version = \"4.0\"\nname = \"nested\"\npresets = [\"claude\"]\n")

		cfg, err := LoadConfig(context.Background(), root)
		require.NoError(t, err)

		assert.Equal(t, "nested", cfg.Name)
		assert.Equal(t, root, cfg.BaseDir)
		assert.Equal(t, filepath.Join(root, ".config", "ai-rulez"), cfg.ConfigDir)
		assert.Equal(t, altConfigDirName, cfg.ConfigDirName)
	})

	t.Run("detects the convention as a directory-based config", func(t *testing.T) {
		root := t.TempDir()
		writeConfigFile(t, filepath.Join(root, ".config", "ai-rulez", "config.toml"), "version = \"4.0\"\nname = \"x\"\n")

		version, err := DetectConfigVersion(root)
		require.NoError(t, err)
		assert.Equal(t, VersionDir, version)
	})
}
