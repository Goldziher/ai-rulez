package config

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	toml "github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeTOMLProject creates a .ai-rulez/ directory with the given config.toml.
func writeTOMLProject(t *testing.T, tomlBody string) string {
	t.Helper()
	baseDir := t.TempDir()
	configDir := filepath.Join(baseDir, aiRulezDirName)
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, configTOMLFilename), []byte(tomlBody), 0o644))
	return baseDir
}

func TestSaveConfig_TOMLProjectWritesTOMLNotYAML(t *testing.T) {
	baseDir := writeTOMLProject(t, "version = \"4.0\"\nname = \"proj\"\npresets = [\"claude\"]\n")

	cfg, err := LoadConfig(context.Background(), baseDir)
	require.NoError(t, err)
	require.Equal(t, configTOMLFilename, cfg.ConfigFile)

	cfg.InstalledSkills = append(cfg.InstalledSkills, InstalledSkillConfig{Name: "demo", Source: "https://example.com/demo.git"})
	require.NoError(t, SaveConfig(cfg, cfg.ConfigDir))

	configDir := filepath.Join(baseDir, aiRulezDirName)
	assert.NoFileExists(t, filepath.Join(configDir, "config.yaml"), "a TOML project must not sprout config.yaml")

	reloaded, err := LoadConfig(context.Background(), baseDir)
	require.NoError(t, err)
	require.Len(t, reloaded.InstalledSkills, 1)
	assert.Equal(t, "demo", reloaded.InstalledSkills[0].Name)
}

func TestSaveConfig_DefaultsToTOMLWhenNoConfigExists(t *testing.T) {
	configDir := t.TempDir()
	cfg := &Config{Version: "4.0", Name: "proj", Presets: []Preset{{BuiltIn: "claude"}}}

	require.NoError(t, SaveConfig(cfg, configDir))
	assert.FileExists(t, filepath.Join(configDir, configTOMLFilename))
	assert.NoFileExists(t, filepath.Join(configDir, "config.yaml"))
}

func TestMarshalTOML_RoundTripsAllPresetKinds(t *testing.T) {
	cfg := &Config{
		Version: "4.0",
		Name:    "proj",
		Presets: []Preset{
			{BuiltIn: "claude"},
			{Name: "docs", Type: PresetTypeMarkdown, Path: "DOCS.md", Template: "hi"},
			{Name: "xum-like", Provider: ".ai-rulez/providers/xum-like.toml"},
		},
	}

	data, err := MarshalTOML(cfg)
	require.NoError(t, err)

	var decoded struct {
		Presets []any `toml:"presets"`
	}
	require.NoError(t, toml.Unmarshal(data, &decoded))
	require.Len(t, decoded.Presets, 3)
	assert.Equal(t, "claude", decoded.Presets[0])

	custom, ok := decoded.Presets[1].(map[string]any)
	require.True(t, ok, "custom preset must encode as an inline table")
	assert.Equal(t, "docs", custom["name"])
	assert.Equal(t, "markdown", custom["type"])

	provider, ok := decoded.Presets[2].(map[string]any)
	require.True(t, ok, "provider preset must encode as an inline table")
	assert.Equal(t, "xum-like", provider["name"])
	assert.Equal(t, ".ai-rulez/providers/xum-like.toml", provider["provider"])
}

func TestLoadConfigTOML_CustomAndProviderPresets(t *testing.T) {
	baseDir := writeTOMLProject(t, `
version = "4.0"
name = "proj"
presets = [
  "claude",
  { name = "docs", type = "markdown", path = "DOCS.md", template = "hi" },
  { name = "xum-like", provider = ".ai-rulez/providers/xum-like.toml" },
]
`)

	cfg, err := LoadConfig(context.Background(), baseDir)
	require.NoError(t, err)
	require.Len(t, cfg.Presets, 3)
	assert.True(t, cfg.Presets[0].IsBuiltIn())
	assert.Equal(t, "claude", cfg.Presets[0].BuiltIn)

	assert.False(t, cfg.Presets[1].IsBuiltIn())
	assert.Equal(t, "docs", cfg.Presets[1].Name)
	assert.Equal(t, PresetTypeMarkdown, cfg.Presets[1].Type)
	assert.Equal(t, "DOCS.md", cfg.Presets[1].Path)

	assert.Equal(t, "xum-like", cfg.Presets[2].Name)
	assert.Equal(t, ".ai-rulez/providers/xum-like.toml", cfg.Presets[2].Provider)
	assert.True(t, cfg.Presets[2].IsValid())
}

func TestPresetProvider_JSONRoundTrip(t *testing.T) {
	encoded, err := json.Marshal(Preset{Name: "p", Provider: "spec.toml"})
	require.NoError(t, err)
	var decoded Preset
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	assert.Equal(t, "spec.toml", decoded.Provider)
	assert.True(t, decoded.IsValid())
}

func TestLintConfig_TOMLRoundTripAndLoad(t *testing.T) {
	src := `version = "4.0"
name = "proj"
presets = ["claude"]

[lint]
fail_on = "warning"
ignore = ["AR401"]
allow_paths = [".claude/**"]

[lint.severity]
glob-no-match = "warning"

[lint.description]
min_length = 30

[lint.budgets.skill]
max_lines = 400

[lint.require_metadata]
skill = ["owner"]
`
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".ai-rulez"), 0o755))
	path := filepath.Join(dir, ".ai-rulez", "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(src), 0o644))

	cfg, err := LoadConfigFromFile(context.Background(), path)
	require.NoError(t, err)
	require.NotNil(t, cfg.Lint)
	assert.Equal(t, "warning", cfg.Lint.FailOn)
	assert.Equal(t, []string{"AR401"}, cfg.Lint.Ignore)
	assert.Equal(t, "warning", cfg.Lint.Severity["glob-no-match"])
	assert.Equal(t, 30, cfg.Lint.Description.MinLength)
	assert.Equal(t, 400, cfg.Lint.Budgets["skill"].MaxLines)
	assert.Equal(t, []string{"owner"}, cfg.Lint.RequireMetadata["skill"])

	out, err := MarshalTOML(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, out, 0o644))
	again, err := LoadConfigFromFile(context.Background(), path)
	require.NoError(t, err)
	assert.Equal(t, cfg.Lint, again.Lint, "[lint] must survive a config rewrite")
}

// tomlKeys lists the TOML key of every serialized field of a struct type.
func tomlKeys(t reflect.Type) map[string]bool {
	keys := map[string]bool{}
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("toml")
		name := strings.Split(tag, ",")[0]
		if name == "" || name == "-" {
			continue
		}
		keys[name] = true
	}
	return keys
}

// A key Config reads but tomlOutput lacks is silently dropped by every config
// rewrite (CRUD, MCP, migrate, convert).
func TestTOMLOutputCarriesEveryConfigKey(t *testing.T) {
	// Arrange
	want := tomlKeys(reflect.TypeOf(Config{}))
	got := tomlKeys(reflect.TypeOf(tomlOutput{}))

	// Act
	var missing []string
	for k := range want {
		if !got[k] {
			missing = append(missing, k)
		}
	}

	// Assert
	assert.Empty(t, missing, "tomlOutput is missing Config keys")
}

func TestMarshalTOMLKeepsOKFTable(t *testing.T) {
	// Arrange
	cfg := &Config{Version: "4.0", Name: "x", OKF: &OKFConfig{Dir: "docs/okf"}}

	// Act
	out, err := MarshalTOML(cfg)

	// Assert
	require.NoError(t, err)
	assert.Contains(t, string(out), "[okf]")
}
