package config

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A top-level key written after a [table] header belongs to that table, so TOML
// silently drops it. The loader refuses it instead: the key names a top-level
// key, so the author meant it to apply globally.
func TestLoadConfig_SwallowedTopLevelKey(t *testing.T) {
	tests := []struct {
		name  string
		table string
		key   string
		value string
	}{
		{"agents_md under rules", "rules", "agents_md", "false"},
		{"builtins under rules", "rules", "builtins", `["rust"]`},
		{"gitignore under rules", "rules", "gitignore", "true"},
		{"presets under rules", "rules", "presets", `["claude"]`},
		{"version under rules", "rules", "version", `"5.0"`},
		{"builtins under header", "header", "builtins", `["rust"]`},
		{"agents_md under defaults", "defaults", "agents_md", "false"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			base := t.TempDir()
			body := "version = \"5.0\"\nname = \"t\"\npresets = [\"claude\"]\n\n[" + tt.table + "]\n" +
				tt.key + " = " + tt.value + "\n"
			writeProjectFile(t, filepath.Join(base, ".ai-rulez"), "config.toml", body)

			// Act
			_, err := LoadConfig(context.Background(), base)

			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.key)
			assert.Contains(t, err.Error(), "["+tt.table+"]")
			assert.Contains(t, err.Error(), "move `"+tt.key+"` above the first [table]")
		})
	}
}

// The same keys above the first table load normally and take effect: the fix the
// error names is real.
func TestLoadConfig_TopLevelKeysAboveTheFirstTable(t *testing.T) {
	// Arrange
	base := t.TempDir()
	body := `version = "5.0"
name = "t"
presets = ["claude"]
agents_md = false
gitignore = true
builtins = ["rust"]

[rules]
mode = "inline"
baz_scoped = "root"
`
	writeProjectFile(t, filepath.Join(base, ".ai-rulez"), "config.toml", body)

	// Act
	cfg, err := LoadConfig(context.Background(), base)

	// Assert
	require.NoError(t, err)
	assert.False(t, cfg.AgentsMD)
	require.NotNil(t, cfg.Gitignore)
	assert.True(t, *cfg.Gitignore)
	require.NotNil(t, cfg.Builtins)
	assert.Contains(t, cfg.Builtins.Names, "rust")
	require.NotNil(t, cfg.Rules)
	assert.Equal(t, RulesModeInline, cfg.Rules.Mode)
	assert.Equal(t, BazScopedRoot, cfg.Rules.BazScoped)
}

// A genuinely unknown key inside a known table is not a swallowed top-level key:
// it keeps its existing behavior (a schema finding, not a load error).
func TestLoadConfig_UnknownKeyInsideKnownTableStillLoads(t *testing.T) {
	// Arrange
	base := t.TempDir()
	body := "version = \"5.0\"\nname = \"t\"\npresets = [\"claude\"]\n\n[rules]\nmode = \"split\"\nnope = 1\n"
	writeProjectFile(t, filepath.Join(base, ".ai-rulez"), "config.toml", body)

	// Act
	cfg, err := LoadConfig(context.Background(), base)
	require.NoError(t, err)

	findings, err := SchemaFindings(cfg)

	// Assert
	require.NoError(t, err)
	var got []string
	for _, f := range findings {
		f.File = filepath.Base(f.File)
		got = append(got, f.String())
	}
	assert.Contains(t, got, "config.toml: unknown key \"rules.nope\"")
}

// Tables whose own keys share a name with a top-level key are not swallowed
// keys: [plugin] really defines name, version and description.
func TestLoadConfig_TablesThatReuseTopLevelKeyNames(t *testing.T) {
	// Arrange
	base := t.TempDir()
	body := `version = "5.0"
name = "t"
presets = ["claude"]

[plugin]
name = "p"
version = "1.0.0"
description = "d"

[placement]
default = "core"
plugin = ["a"]

[claude.skills]
hide_from_menu = true
`
	writeProjectFile(t, filepath.Join(base, ".ai-rulez"), "config.toml", body)

	// Act
	cfg, err := LoadConfig(context.Background(), base)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, cfg.Plugin)
	assert.Equal(t, "p", cfg.Plugin.Name)
	require.NotNil(t, cfg.Placement)
	assert.Equal(t, "core", cfg.Placement.Default)
}
