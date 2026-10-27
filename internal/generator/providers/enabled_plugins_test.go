package providers_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func basemindPlugin() config.EnabledPlugin {
	return config.EnabledPlugin{
		ID:          "basemind@basemind",
		Marketplace: &config.EnabledPluginMarketplace{Name: "basemind", Source: "Goldziher/basemind", Type: "github"},
		OpenCode:    "basemind-opencode",
	}
}

type enabledSettings struct {
	EnabledPlugins map[string]bool           `json:"enabledPlugins"`
	Marketplaces   map[string]map[string]any `json:"extraKnownMarketplaces"`
	Permissions    map[string][]string       `json:"permissions"`
}

func TestClaudeSettingsEnabledPlugins(t *testing.T) {
	t.Parallel()

	const existing = `{
  "permissions": {"allow": ["Read"]},
  "extraKnownMarketplaces": {"theirs": {"source": {"source": "github", "repo": "o/r"}}},
  "enabledPlugins": {"theirs-plugin@theirs": true, "basemind@basemind": false}
}
`
	relPath := filepath.Join(".claude", "settings.json")
	tests := []struct {
		name     string
		existing bool
		cfg      func(baseDir string) *config.Config
		assertFn func(t *testing.T, got enabledSettings)
	}{
		{
			name: "fresh document without manage or a local marketplace",
			cfg: func(baseDir string) *config.Config {
				return &config.Config{Name: "test", BaseDir: baseDir, EnabledPlugins: []config.EnabledPlugin{basemindPlugin()}}
			},
			assertFn: func(t *testing.T, got enabledSettings) {
				assert.Equal(t, map[string]bool{"basemind@basemind": true}, got.EnabledPlugins)
				assert.Equal(t, map[string]any{"source": "github", "repo": "Goldziher/basemind"}, got.Marketplaces["basemind"]["source"])
			},
		},
		{
			name:     "merge keeps the user's own marketplaces, plugins and keys",
			existing: true,
			cfg: func(baseDir string) *config.Config {
				return &config.Config{Name: "test", BaseDir: baseDir, EnabledPlugins: []config.EnabledPlugin{basemindPlugin()}}
			},
			assertFn: func(t *testing.T, got enabledSettings) {
				assert.Equal(t, map[string]bool{"theirs-plugin@theirs": true, "basemind@basemind": true}, got.EnabledPlugins)
				assert.Contains(t, got.Marketplaces, "theirs")
				assert.Contains(t, got.Marketplaces, "basemind")
				assert.Equal(t, []string{"Read"}, got.Permissions["allow"])
			},
		},
		{
			name: "id without a marketplace registers nothing",
			cfg: func(baseDir string) *config.Config {
				return &config.Config{Name: "test", BaseDir: baseDir, EnabledPlugins: []config.EnabledPlugin{{ID: "tool@official"}}}
			},
			assertFn: func(t *testing.T, got enabledSettings) {
				assert.Equal(t, map[string]bool{"tool@official": true}, got.EnabledPlugins)
				assert.Empty(t, got.Marketplaces)
			},
		},
		{
			name: "combines with the local marketplace and qualified enable_plugins",
			cfg: func(baseDir string) *config.Config {
				return &config.Config{
					Name: "test", BaseDir: baseDir,
					Marketplace:    &config.MarketplaceAuthoring{Name: "mk", OutputDir: "tools/mkt"},
					Claude:         &config.ClaudeConfig{Settings: &config.ClaudeSettings{Manage: true, EnablePlugins: []string{"own", "other@elsewhere"}}},
					EnabledPlugins: []config.EnabledPlugin{basemindPlugin()},
				}
			},
			assertFn: func(t *testing.T, got enabledSettings) {
				assert.Equal(t, map[string]bool{"own@mk": true, "other@elsewhere": true, "basemind@basemind": true}, got.EnabledPlugins)
				assert.Equal(t, map[string]any{"source": "directory", "path": "./tools/mkt"}, got.Marketplaces["mk"]["source"])
				assert.Contains(t, got.Marketplaces, "basemind")
			},
		},
		{
			name: "an entry never overrides disable_plugins",
			cfg: func(baseDir string) *config.Config {
				return &config.Config{
					Name: "test", BaseDir: baseDir,
					Claude:         &config.ClaudeConfig{Settings: &config.ClaudeSettings{Manage: true, DisablePlugins: []string{"basemind@basemind"}}},
					EnabledPlugins: []config.EnabledPlugin{basemindPlugin()},
				}
			},
			assertFn: func(t *testing.T, got enabledSettings) {
				assert.Equal(t, map[string]bool{"basemind@basemind": false}, got.EnabledPlugins)
			},
		},
		{
			name: "an entry never replaces the local marketplace registration",
			cfg: func(baseDir string) *config.Config {
				plugin := config.EnabledPlugin{
					ID:          "x@mk",
					Marketplace: &config.EnabledPluginMarketplace{Name: "mk", Source: "o/r", Type: "github"},
				}
				return &config.Config{
					Name: "test", BaseDir: baseDir,
					Marketplace:    &config.MarketplaceAuthoring{Name: "mk", OutputDir: "tools/mkt"},
					Claude:         &config.ClaudeConfig{Settings: &config.ClaudeSettings{Manage: true, EnablePlugins: []string{"own"}}},
					EnabledPlugins: []config.EnabledPlugin{plugin},
				}
			},
			assertFn: func(t *testing.T, got enabledSettings) {
				assert.Equal(t, map[string]any{"source": "directory", "path": "./tools/mkt"}, got.Marketplaces["mk"]["source"])
			},
		},
		{
			name: "manage with only qualified ids and no local marketplace",
			cfg: func(baseDir string) *config.Config {
				return &config.Config{
					Name: "test", BaseDir: baseDir,
					Claude: &config.ClaudeConfig{Settings: &config.ClaudeSettings{Manage: true, EnablePlugins: []string{"basemind@basemind"}}},
				}
			},
			assertFn: func(t *testing.T, got enabledSettings) {
				assert.Equal(t, map[string]bool{"basemind@basemind": true}, got.EnabledPlugins)
				assert.Empty(t, got.Marketplaces)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			baseDir := t.TempDir()
			if tt.existing {
				baseDir = writeFixture(t, relPath, existing)
			}
			gen := claudeGen(t)

			// Act
			outputs, err := gen.Generate(&config.ContentTree{}, baseDir, tt.cfg(baseDir))
			require.NoError(t, err)

			// Assert
			var got enabledSettings
			require.NoError(t, json.Unmarshal([]byte(requireFile(t, outputs, relPath).Content), &got))
			tt.assertFn(t, got)
		})
	}
}
