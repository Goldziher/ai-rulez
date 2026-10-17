package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
	"github.com/Goldziher/ai-rulez/v5/internal/workspace"
)

func TestLoadConfig_RootLayoutErrorPaths(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T, root string)
		wantErr string
	}{
		{
			"invalid toml",
			func(t *testing.T, root string) {
				writeConfigFile(t, filepath.Join(root, "ai-rulez.toml"), "version = \"5.0\"\nname = \n")
			},
			"parse TOML config",
		},
		{
			"default layers are not supported",
			func(t *testing.T, root string) {
				writeConfigFile(t, filepath.Join(root, "ai-rulez.toml"), rootLayoutConfig+"default_layers = [\"ci\"]\n")
			},
			"default_layers is not supported in ai-rulez.toml",
		},
		{
			"malformed local overlay",
			func(t *testing.T, root string) {
				writeConfigFile(t, filepath.Join(root, "ai-rulez.toml"), rootLayoutConfig)
				writeConfigFile(t, filepath.Join(root, "ai-rulez.local.toml"), "presets = [\n")
			},
			"ai-rulez.local.toml",
		},
		{
			"ai-rulez.toml is a directory",
			func(t *testing.T, root string) {
				require.NoError(t, os.MkdirAll(filepath.Join(root, "ai-rulez.toml"), 0o755))
			},
			".ai-rulez directory not found",
		},
		{
			"ai-rulez.toml is a symlink",
			func(t *testing.T, root string) {
				target := filepath.Join(t.TempDir(), "elsewhere.toml")
				writeConfigFile(t, target, rootLayoutConfig)
				testutil.SymlinkOrSkip(t, target, filepath.Join(root, "ai-rulez.toml"))
			},
			"refusing symlinked config ai-rulez.toml",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			tt.setup(t, root)

			// Act
			_, err := LoadConfig(context.Background(), root, WithoutRemote())

			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestLoadRootLayout_MissingFileIsNoConfig(t *testing.T) {
	// Arrange
	root := t.TempDir()

	// Act
	_, err := loadRootLayout(context.Background(), workspace.OSView(root), root, loadOptions{})

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no config file found (tried ai-rulez.toml)")
	assert.ErrorAs(t, err, new(noConfig))
}

func TestLoadConfig_RootLayoutBesideALegacyFlatFileLoads(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeConfigFile(t, filepath.Join(root, "ai-rulez.toml"), rootLayoutConfig)
	writeConfigFile(t, filepath.Join(root, "ai-rulez.yaml"), "name: legacy\n")

	// Act
	cfg, err := LoadConfig(context.Background(), root, WithoutRemote())

	// Assert
	require.NoError(t, err)
	assert.True(t, cfg.Contentless())
	assert.Equal(t, "solo", cfg.Name)
}

func TestConfig_RootLayoutNames(t *testing.T) {
	tests := []struct {
		name           string
		files          map[string]string
		wantContent    bool
		wantManifest   string
		wantLocal      string
		wantLocalDir   string
		wantSkipsRoots bool
	}{
		{
			"root layout", map[string]string{"ai-rulez.toml": rootLayoutConfig}, true,
			".ai-rulez-manifest.json", ".ai-rulez-manifest.local.json", ".ai-rulez-local.d", true,
		},
		{
			"directory layout", map[string]string{".ai-rulez/config.toml": rootLayoutConfig}, false,
			".generated-manifest.json", ".generated-manifest.local.json", "local", false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			for rel, body := range tt.files {
				writeConfigFile(t, filepath.Join(root, filepath.FromSlash(rel)), body)
			}

			// Act
			cfg, err := LoadConfig(context.Background(), root, WithoutRemote())

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.wantContent, cfg.Contentless())
			assert.Equal(t, tt.wantSkipsRoots, cfg.SkipsRootFiles())
			assert.Equal(t, tt.wantManifest, cfg.ManifestName())
			assert.Equal(t, tt.wantLocal, cfg.LocalManifestName())
			assert.Equal(t, filepath.Join(cfg.ConfigDir, tt.wantLocalDir), cfg.LocalDir())
		})
	}
}

func TestRootManifestNames(t *testing.T) {
	assert.Equal(t, []string{".ai-rulez-manifest.json", ".ai-rulez-manifest.local.json"}, RootManifestNames())
}

func TestContentless_NilConfigIsNotContentless(t *testing.T) {
	var cfg *Config

	assert.False(t, cfg.Contentless())
	assert.False(t, cfg.SkipsRootFiles())
}

func TestIsRootConfigDir_RejectsADirectoryNamedLikeAConfigDirectory(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeConfigFile(t, filepath.Join(root, ".ai-rulez", "ai-rulez.toml"), rootLayoutConfig)
	writeConfigFile(t, filepath.Join(root, ".agents", "ai-rulez.toml"), rootLayoutConfig)

	// Act and Assert
	assert.False(t, IsRootConfigDir(filepath.Join(root, ".ai-rulez")))
	assert.False(t, IsRootConfigDir(filepath.Join(root, ".agents")))
	assert.False(t, isRootLayoutMain(filepath.Join(root, ".ai-rulez", "ai-rulez.toml"), filepath.Join(root, ".ai-rulez")))
	assert.False(t, isRootLayoutMain(filepath.Join(root, ".agents", "ai-rulez.toml"), filepath.Join(root, ".agents")))
	assert.False(t, isRootLayoutMain(filepath.Join(root, "config.toml"), root))
	assert.True(t, isRootLayoutMain(filepath.Join(root, "ai-rulez.toml"), root))
}
