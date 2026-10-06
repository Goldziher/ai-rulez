package config

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeProjectFile(t *testing.T, dir, name, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600))
}

const (
	overlayMainTOML = `version = "4.0"
name = "shared"
presets = ["claude", "cursor"]
default = "base"

[profiles]
base = ["a"]

[[mcp_servers]]
name = "srv"
command = "x"
args = ["1"]
`
	overlayLocalTOML = `name = "mine"
presets = ["!cursor", "codex"]

[profiles]
dev = ["b"]

[[mcp_servers]]
name = "srv"
args = ["2"]
`
)

func presetNames(cfg *Config) []string {
	names := make([]string, 0, len(cfg.Presets))
	for _, p := range cfg.Presets {
		names = append(names, p.BuiltIn)
	}
	return names
}

func TestLoadConfig_AppliesLocalOverlay(t *testing.T) {
	tests := []struct {
		name      string
		mainFile  string
		mainBody  string
		localFile string
		localBody string
	}{
		{"toml main and local", "config.toml", overlayMainTOML, "config.local.toml", overlayLocalTOML},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			base := t.TempDir()
			dir := filepath.Join(base, ".ai-rulez")
			writeProjectFile(t, dir, tt.mainFile, tt.mainBody)
			writeProjectFile(t, dir, tt.localFile, tt.localBody)

			// Act
			cfg, err := LoadConfig(context.Background(), base)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, "mine", cfg.Name)
			assert.Equal(t, "base", cfg.Default)
			assert.Equal(t, []string{"claude", "codex"}, presetNames(cfg))
			assert.Equal(t, []string{"a"}, cfg.Profiles["base"])
			assert.Equal(t, []string{"b"}, cfg.Profiles["dev"])
			require.Len(t, cfg.MCPServersRaw, 1)
			assert.Equal(t, "x", cfg.MCPServersRaw[0].Command)
			assert.Equal(t, []string{"2"}, cfg.MCPServersRaw[0].Args)
			assert.Equal(t, tt.mainFile, cfg.ConfigFile)
			require.NotNil(t, cfg.LocalOverlay)
			assert.Equal(t, filepath.Join(dir, tt.localFile), cfg.LocalOverlay.Path)
			assert.Equal(t, "mine", cfg.LocalOverlay.Doc["name"])
			assert.True(t, cfg.HasLocalInputs())
		})
	}
}

func TestLoadConfig_WithoutLocal(t *testing.T) {
	// Arrange
	base := t.TempDir()
	dir := filepath.Join(base, ".ai-rulez")
	writeProjectFile(t, dir, "config.toml", overlayMainTOML)
	writeProjectFile(t, dir, "config.local.toml", overlayLocalTOML)
	writeProjectFile(t, filepath.Join(dir, "local", "rules"), "mine.md", "# local rule\n")

	tests := []struct {
		name      string
		opts      []LoadOption
		wantName  string
		wantLocal bool
	}{
		{"default applies overlay and local content", nil, "mine", true},
		{"WithoutLocal skips both", []LoadOption{WithoutLocal()}, "shared", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			cfg, err := LoadConfig(context.Background(), base, tt.opts...)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tt.wantName, cfg.Name)
			assert.Equal(t, tt.wantLocal, cfg.LocalOverlay != nil)
			assert.Equal(t, tt.wantLocal, cfg.LocalContent != nil && !cfg.LocalContent.IsEmpty())
			assert.Equal(t, tt.wantLocal, cfg.HasLocalInputs())
		})
	}
}

func TestLoadConfig_NoOverlayIsUntouched(t *testing.T) {
	// Arrange
	base := t.TempDir()
	writeProjectFile(t, filepath.Join(base, ".ai-rulez"), "config.toml", overlayMainTOML)

	// Act
	cfg, err := LoadConfig(context.Background(), base)

	// Assert
	require.NoError(t, err)
	assert.Nil(t, cfg.LocalOverlay)
	assert.False(t, cfg.HasLocalInputs())
	assert.Equal(t, "shared", cfg.Name)
}

func TestFindLocalConfigFile_None(t *testing.T) {
	td := t.TempDir()
	// Act
	path, err := findLocalConfigFile(osView(td), td)

	// Assert
	require.NoError(t, err)
	assert.Empty(t, path)
}

func TestLoadConfig_LocalOverlay_DotConfigLayout(t *testing.T) {
	// Arrange
	base := t.TempDir()
	dir := filepath.Join(base, ".config", "ai-rulez")
	writeProjectFile(t, dir, "config.toml", overlayMainTOML)
	writeProjectFile(t, dir, "config.local.toml", overlayLocalTOML)

	// Act
	cfg, err := LoadConfig(context.Background(), base)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "mine", cfg.Name)
	require.NotNil(t, cfg.LocalOverlay)
	assert.Equal(t, filepath.Join(dir, "config.local.toml"), cfg.LocalOverlay.Path)
}

func TestLoadConfigFromFile_AppliesOverlayAndRejectsLocalFilePath(t *testing.T) {
	// Arrange
	base := t.TempDir()
	dir := filepath.Join(base, ".ai-rulez")
	writeProjectFile(t, dir, "config.toml", overlayMainTOML)
	writeProjectFile(t, dir, "config.local.toml", overlayLocalTOML)

	t.Run("main file path applies overlay", func(t *testing.T) {
		cfg, err := LoadConfigFromFile(context.Background(), filepath.Join(dir, "config.toml"))
		require.NoError(t, err)
		assert.Equal(t, "mine", cfg.Name)
	})
	t.Run("directory path applies overlay", func(t *testing.T) {
		cfg, err := LoadConfigFromFile(context.Background(), dir)
		require.NoError(t, err)
		assert.Equal(t, "mine", cfg.Name)
	})
	t.Run("directory path honors WithoutLocal", func(t *testing.T) {
		cfg, err := LoadConfigFromFile(context.Background(), dir, WithoutLocal())
		require.NoError(t, err)
		assert.Equal(t, "shared", cfg.Name)
	})
	t.Run("local file path is rejected", func(t *testing.T) {
		_, err := LoadConfigFromFile(context.Background(), filepath.Join(dir, "config.local.toml"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "local overlay")
	})
}

func TestLoadConfig_LocalOverlayErrorsNameTheLocalFile(t *testing.T) {
	tests := []struct {
		name      string
		localFile string
		localBody string
		wantErr   string
	}{
		{"syntax error", "config.local.toml", "name = ", "config.local.toml"},
		{"unknown key", "config.local.toml", "nmea = \"x\"", "config.local.toml"},
		{"version mismatch", "config.local.toml", "version = \"3.0\"\n", "config.local.toml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			base := t.TempDir()
			dir := filepath.Join(base, ".ai-rulez")
			writeProjectFile(t, dir, "config.toml", overlayMainTOML)
			writeProjectFile(t, dir, tt.localFile, tt.localBody)

			// Act
			_, err := LoadConfig(context.Background(), base)

			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestSaveConfig_RefusesMergedConfig(t *testing.T) {
	// Arrange
	base := t.TempDir()
	dir := filepath.Join(base, ".ai-rulez")
	writeProjectFile(t, dir, "config.toml", overlayMainTOML)
	writeProjectFile(t, dir, "config.local.toml", overlayLocalTOML)
	merged, err := LoadConfig(context.Background(), base)
	require.NoError(t, err)
	before, err := os.ReadFile(filepath.Join(dir, "config.toml"))
	require.NoError(t, err)

	tests := []struct {
		name string
		fn   func() error
	}{
		{"SaveConfig", func() error { return SaveConfig(merged, dir) }},
		{"MarshalTOML", func() error { _, err := MarshalTOML(merged); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			err := tt.fn()

			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), "local overlay")
		})
	}
	after, err := os.ReadFile(filepath.Join(dir, "config.toml"))
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after))

	t.Run("shared view saves", func(t *testing.T) {
		shared, err := LoadConfig(context.Background(), base, WithoutLocal())
		require.NoError(t, err)
		assert.NoError(t, SaveConfig(shared, dir))
	})
}

func TestDecodeViaJSONEquivalence(t *testing.T) {
	repoConfig, err := os.ReadFile(filepath.Join("..", "..", ".ai-rulez", "config.toml"))
	require.NoError(t, err)

	tests := []struct {
		name string
		file string
		body string
	}{
		{"repo config.toml", "config.toml", string(repoConfig)},
		{"minimal toml", "config.toml", "version = \"4.0\"\nname = \"x\"\n"},
		{"toml mixed presets", "config.toml", `version = "4.0"
name = "x"
presets = ["claude", {name = "mine", path = "OUT.md", type = "markdown"}]
`},
		{"toml builtins bool", "config.toml", "version = \"4.0\"\nname = \"x\"\nbuiltins = false\n"},
		{"toml builtins list", "config.toml", "version = \"4.0\"\nname = \"x\"\nbuiltins = [\"go\", \"!security\"]\n"},
		{"toml full", "config.toml", overlayMainTOML + "\n[header]\nstyle = \"compact\"\n\n[defaults.effort_by_preset]\nclaude = \"high\"\n"},
		{"toml with schema key", "config.toml", "schema = \"https://example.com/s.json\"\nversion = \"4.0\"\nname = \"x\"\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			path := filepath.Join(t.TempDir(), tt.file)
			data := []byte(tt.body)
			native, err := decodeConfigTOML(data, path)
			require.NoError(t, err)

			// Act
			doc, err := decodeConfigDoc(path, data)
			require.NoError(t, err)
			merged, _, err := MergeConfigDocs(doc, map[string]any{})
			require.NoError(t, err)
			viaJSON, err := decodeMergedDoc(path, path, merged)
			require.NoError(t, err)

			// Assert
			// The native TOML decode yields an empty (non-nil) preset slice for a
			// document without presets; the JSON route yields nil. Both mean "none".
			for _, c := range []*Config{native, viaJSON} {
				if len(c.Presets) == 0 {
					c.Presets = nil
				}
			}
			if !reflect.DeepEqual(native, viaJSON) {
				assert.Equal(t, native, viaJSON)
			}
		})
	}
}

func TestLoadConfig_LocalOverlayMergesLintPerKey(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, ".ai-rulez")
	writeProjectFile(t, dir, "config.toml", overlayMainTOML+"\n[lint]\nfail_on = \"warning\"\nignore = [\"AR401\"]\n")
	writeProjectFile(t, dir, "config.local.toml", "[lint]\nignore = [\"AR101\"]\n")

	cfg, err := LoadConfig(context.Background(), base)

	require.NoError(t, err)
	require.NotNil(t, cfg.Lint)
	assert.Equal(t, "warning", cfg.Lint.FailOn, "keys the overlay does not set keep the shared value")
	assert.Equal(t, []string{"AR101"}, cfg.Lint.Ignore, "keys the overlay sets replace the shared value")
}
