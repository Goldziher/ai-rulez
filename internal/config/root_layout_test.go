package config

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/ambient"
	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

const rootLayoutConfig = "version = \"5.0\"\nname = \"solo\"\npresets = [\"claude\"]\n"

func TestFindConfigFile_RootLayout(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  string // relative to the project root; "" means an error
	}{
		{"ai-rulez.toml alone", map[string]string{"ai-rulez.toml": rootLayoutConfig}, "ai-rulez.toml"},
		{
			"the .ai-rulez directory wins",
			map[string]string{"ai-rulez.toml": rootLayoutConfig, ".ai-rulez/config.toml": rootLayoutConfig},
			".ai-rulez/config.toml",
		},
		{
			"the .config convention wins",
			map[string]string{"ai-rulez.toml": rootLayoutConfig, ".config/ai-rulez/config.toml": rootLayoutConfig},
			".config/ai-rulez/config.toml",
		},
		{
			".agents/ai-rulez.toml wins",
			map[string]string{"ai-rulez.toml": rootLayoutConfig, ".agents/ai-rulez.toml": rootLayoutConfig},
			".agents/ai-rulez.toml",
		},
		{"a legacy flat file alone is not a config", map[string]string{"ai-rulez.yaml": "name: x\n"}, ""},
		{"ai-rulez.toml beats a legacy flat file", map[string]string{"ai-rulez.toml": rootLayoutConfig, "ai-rulez.yaml": "name: x\n"}, "ai-rulez.toml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			for rel, body := range tt.files {
				writeConfigFile(t, filepath.Join(root, filepath.FromSlash(rel)), body)
			}

			// Act
			got, err := FindConfigFile(root)

			// Assert
			if tt.want == "" {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, filepath.Join(root, filepath.FromSlash(tt.want)), got)
		})
	}
}

func TestFindConfigFile_RootLayoutWalksUp(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeConfigFile(t, filepath.Join(root, "ai-rulez.toml"), rootLayoutConfig)
	nested := filepath.Join(root, "a", "b")
	require.NoError(t, os.MkdirAll(nested, 0o755))

	// Act
	got, err := FindConfigFile(nested)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "ai-rulez.toml"), got)
}

func TestResolveConfigDirName_RootLayout(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"ai-rulez.toml alone is the project root", map[string]string{"ai-rulez.toml": rootLayoutConfig}, RootConfigDirName},
		{"a directory beats the file", map[string]string{"ai-rulez.toml": rootLayoutConfig, ".ai-rulez/config.toml": rootLayoutConfig}, aiRulezDirName},
		{"nothing", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for rel, body := range tt.files {
				writeConfigFile(t, filepath.Join(root, filepath.FromSlash(rel)), body)
			}

			assert.Equal(t, tt.want, ResolveConfigDirName(root))
		})
	}
}

func TestLoadConfig_RootLayoutIsContentless(t *testing.T) {
	// Arrange: directories that look like a content tree must not be read.
	root := t.TempDir()
	writeConfigFile(t, filepath.Join(root, "ai-rulez.toml"), rootLayoutConfig)
	for _, rel := range []string{"rules/a.md", "context/b.md", "skills/s/SKILL.md", "agents/c.md", "commands/d.md", "domains/x/rules/e.md", "local/rules/f.md"} {
		writeConfigFile(t, filepath.Join(root, filepath.FromSlash(rel)), "# not content\n")
	}

	// Act
	cfg, err := LoadConfig(context.Background(), root, WithoutRemote())

	// Assert
	require.NoError(t, err)
	assert.True(t, cfg.Contentless())
	assert.True(t, cfg.SkipsRootFiles())
	assert.Equal(t, root, cfg.ConfigDir)
	assert.Equal(t, root, cfg.BaseDir)
	assert.Equal(t, filepath.Join(root, "ai-rulez.toml"), cfg.ConfigPath())
	assert.Equal(t, "ai-rulez.toml", cfg.ConfigFile)
	assert.True(t, cfg.Content.IsEmpty(), "no content tree is scanned")
	assert.True(t, cfg.LocalContent.IsEmpty(), "no local content tree is scanned")
	assert.Equal(t, ".ai-rulez-manifest.json", cfg.ManifestName())
	assert.Equal(t, ".ai-rulez-manifest.local.json", cfg.LocalManifestName())
	assert.NoError(t, cfg.Validate())
}

func TestLoadConfig_DirectoryLayoutIsNotContentless(t *testing.T) {
	root := t.TempDir()
	writeConfigFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), rootLayoutConfig)

	cfg, err := LoadConfig(context.Background(), root, WithoutRemote())

	require.NoError(t, err)
	assert.False(t, cfg.Contentless())
	assert.False(t, cfg.SkipsRootFiles())
	assert.Equal(t, ".generated-manifest.json", cfg.ManifestName())
	assert.Equal(t, ".generated-manifest.local.json", cfg.LocalManifestName())
}

func TestLoadConfig_BothLayoutsWarnThatTheFileIsIgnored(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeConfigFile(t, filepath.Join(root, "ai-rulez.toml"), rootLayoutConfig)
	writeConfigFile(t, filepath.Join(root, ".ai-rulez", "config.toml"), rootLayoutConfig)
	rec := &testutil.LogRecorder{}

	// Act
	cfg, err := LoadConfig(context.Background(), root, WithoutRemote(), WithHost(ambient.Host{Log: rec}))

	// Assert
	require.NoError(t, err)
	assert.False(t, cfg.Contentless(), "the directory wins")
	warnings := strings.Join(rec.Level("WARN"), "\n")
	assert.Contains(t, warnings, "ai-rulez.toml is ignored")
}

func TestLoadConfig_RootLayoutLegacyFilesStillFail(t *testing.T) {
	tests := []struct {
		name string
		file string
	}{
		{"flat yaml", "ai-rulez.yaml"},
		{"flat yml", "ai-rulez.yml"},
		{"hidden flat yaml", ".ai-rulez.yaml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeConfigFile(t, filepath.Join(root, tt.file), "name: legacy\n")

			_, err := LoadConfig(context.Background(), root, WithoutRemote())

			require.Error(t, err)
			assert.ErrorIs(t, err, ErrLegacyConfig)
			assert.Contains(t, err.Error(), "migrate")
		})
	}
}

func TestLoadConfigFromFile_RootLayout(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeConfigFile(t, filepath.Join(root, "ai-rulez.toml"), rootLayoutConfig)
	plain := t.TempDir()
	writeConfigFile(t, filepath.Join(plain, "go.mod"), "module x\n")
	writeConfigFile(t, filepath.Join(plain, "config.toml"), "version = \"5.0\"\nname = \"other\"\n")

	// Act
	viaFile, fileErr := LoadConfigFromFile(context.Background(), filepath.Join(root, "ai-rulez.toml"), WithoutRemote())
	viaDir, dirErr := LoadConfigFromFile(context.Background(), root, WithoutRemote())
	_, rootToml := LoadConfigFromFile(context.Background(), filepath.Join(plain, "config.toml"), WithoutRemote())

	// Assert
	require.NoError(t, fileErr)
	assert.True(t, viaFile.Contentless())
	assert.Equal(t, root, viaFile.BaseDir)
	require.NoError(t, dirErr)
	assert.True(t, viaDir.Contentless())
	require.Error(t, rootToml, "a root-level config.toml still needs the directory layout")
	assert.Contains(t, rootToml.Error(), "directory layout required")
}

func TestLoadConfig_RootLayoutRejectsLayers(t *testing.T) {
	root := t.TempDir()
	writeConfigFile(t, filepath.Join(root, "ai-rulez.toml"), rootLayoutConfig+"default_layers = [\"ci\"]\n")

	_, err := LoadConfig(context.Background(), root, WithoutRemote())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "default_layers is not supported")
}

func TestLoadConfig_RootLayoutLocalOverlay(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeConfigFile(t, filepath.Join(root, "ai-rulez.toml"), rootLayoutConfig)
	writeConfigFile(t, filepath.Join(root, "ai-rulez.local.toml"), "presets = [\"claude\", \"codex\"]\n")

	// Act
	cfg, err := LoadConfig(context.Background(), root, WithoutRemote())
	shared, sharedErr := LoadConfig(context.Background(), root, WithoutRemote(), WithoutLocal())
	_, asMain := LoadConfigFromFile(context.Background(), filepath.Join(root, "ai-rulez.local.toml"), WithoutRemote())

	// Assert
	require.NoError(t, err)
	assert.True(t, cfg.Contentless(), "the overlay keeps the layout")
	require.NotNil(t, cfg.LocalOverlay)
	assert.Equal(t, filepath.Join(root, "ai-rulez.local.toml"), cfg.LocalOverlay.Path)
	assert.Len(t, cfg.Presets, 2)
	require.NoError(t, sharedErr)
	assert.Len(t, shared.Presets, 1)
	require.Error(t, asMain)
	assert.Contains(t, asMain.Error(), "local overlay")
}

func TestSaveConfig_RootLayoutWritesAiRulezToml(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeConfigFile(t, filepath.Join(root, "ai-rulez.toml"), rootLayoutConfig)
	cfg, err := LoadConfig(context.Background(), root, WithoutRemote())
	require.NoError(t, err)
	cfg.InstalledSkills = append(cfg.InstalledSkills, InstalledSkillConfig{Name: "demo", Source: "./shared"})

	// Act
	require.NoError(t, SaveConfig(cfg, root))

	// Assert
	written, err := os.ReadFile(filepath.Join(root, "ai-rulez.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(written), "[[installed_skills]]")
	_, statErr := os.Stat(filepath.Join(root, "config.toml"))
	assert.True(t, os.IsNotExist(statErr), "no config.toml appears beside the project files")
	reloaded, err := LoadConfig(context.Background(), root, WithoutRemote())
	require.NoError(t, err)
	assert.Len(t, reloaded.InstalledSkills, 1)
}

func TestLocalDoc_RootLayout(t *testing.T) {
	// Arrange
	root := t.TempDir()
	writeConfigFile(t, filepath.Join(root, "ai-rulez.toml"), rootLayoutConfig)

	// Act
	path, created, err := InitLocalOverlay(root)
	require.NoError(t, err)
	doc, err := OpenLocalDocInDir(root)
	require.NoError(t, err)
	defer doc.Close()

	// Assert
	assert.True(t, created)
	assert.Equal(t, filepath.Join(root, "ai-rulez.local.toml"), path)
	assert.Equal(t, path, doc.Path)
	assert.Equal(t, []string{"ai-rulez.local.*", ".ai-rulez-local.*"}, doc.gitignorePatterns())
	assert.Equal(t, []string{"ai-rulez.local.*", ".ai-rulez-local.*"}, LocalGitignorePatterns(root))
	if runtime.GOOS != "windows" { // overlay edits are not serialized across processes on Windows
		_, statErr := os.Stat(filepath.Join(root, ".ai-rulez-local.lock"))
		assert.NoError(t, statErr, "the edit lock sits beside the overlay under its own name")
	}
}

func TestLayoutOf_RootLayout(t *testing.T) {
	root := t.TempDir()
	writeConfigFile(t, filepath.Join(root, "ai-rulez.toml"), rootLayoutConfig)

	l := LayoutOf(root)

	assert.True(t, l.Contentless)
	assert.Equal(t, root, l.ConfigDir)
	assert.Equal(t, filepath.Join(root, "ai-rulez.toml"), l.ConfigFile)
	assert.Equal(t, root, l.StateDir)
	assert.NotEqual(t, root, l.ContentDir, "the project root is never a content directory")
	assert.Equal(t, root, ProjectBaseDir(root))
}

func TestIsRootConfigDir(t *testing.T) {
	root := t.TempDir()
	writeConfigFile(t, filepath.Join(root, "ai-rulez.toml"), rootLayoutConfig)
	writeConfigFile(t, filepath.Join(root, ".agents", "ai-rulez.toml"), rootLayoutConfig)

	assert.True(t, IsRootConfigDir(root))
	assert.False(t, IsRootConfigDir(filepath.Join(root, ".agents")), "the agents layout has its own state directory")
	assert.False(t, IsRootConfigDir(filepath.Join(root, ".ai-rulez")))
	assert.False(t, IsRootConfigDir(t.TempDir()))
}
