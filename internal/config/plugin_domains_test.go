package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeConfigTOML_DomainPlugins(t *testing.T) {
	// Arrange
	data := []byte(`
name = "x"
[marketplace]
name = "mk"
output_dir = "out"
[marketplace.from_domains]
name_prefix = "a-"
version = "2.0.0"
default_enabled = false
exclude = ["tools"]
[[marketplace.plugins]]
name = "p"
domains = ["d"]
version = "3"
[marketplace.plugins.relevance.signals]
files_read = ["**/*.tf"]
[marketplace.catalog_skill]
enabled = true
[placement]
plugin = ["domains/*"]
honor_targets = true
[claude.settings]
manage = true
enable_plugins = ["p"]
[claude.settings.marketplace_source]
source = "github"
repo = "org/repo"
`)

	// Act
	cfg, err := decodeConfigTOML(data, "test.toml")

	// Assert
	require.NoError(t, err)
	fd := cfg.Marketplace.FromDomains
	assert.Equal(t, "a-", fd.NamePrefix)
	assert.Equal(t, "2.0.0", fd.Version, "shared plugin fields decode inline")
	require.NotNil(t, fd.DefaultEnabled)
	assert.False(t, *fd.DefaultEnabled)
	assert.True(t, fd.IsEnabled())
	require.Len(t, cfg.Marketplace.Plugins, 1)
	assert.Equal(t, "3", cfg.Marketplace.Plugins[0].Version)
	assert.Equal(t, []string{"**/*.tf"}, cfg.Marketplace.Plugins[0].Relevance.Signals.FilesRead)
	assert.True(t, cfg.Marketplace.CatalogSkill.Enabled)
	assert.True(t, cfg.Placement.HonorTargets)
	assert.True(t, cfg.ManagesClaudeSettings())
	assert.Equal(t, "org/repo", cfg.Claude.Settings.MarketplaceSource.Repo)
	assert.True(t, cfg.HasPluginAuthoring())
}

func TestValidateDomainPluginConfig(t *testing.T) {
	boolPtr := func(b bool) *bool { return &b }
	base := func() *Config {
		return &Config{Marketplace: &MarketplaceAuthoring{Name: "mk", FromDomains: &DomainPluginsConfig{}}}
	}
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{name: "valid minimal", mutate: func(*Config) {}},
		{name: "unsafe output_dir", mutate: func(c *Config) { c.Marketplace.OutputDir = "../x" }, wantErr: "unsafe output_dir"},
		{name: "output_dir in .git", mutate: func(c *Config) { c.Marketplace.OutputDir = ".git/hooks" }, wantErr: "unsafe output_dir"},
		{name: "output_dir in .ai-rulez", mutate: func(c *Config) { c.Marketplace.OutputDir = ".ai-rulez/x" }, wantErr: "unsafe output_dir"},
		{name: "members with output_dir", mutate: func(c *Config) {
			c.Marketplace.Members = []string{"a"}
			c.Marketplace.OutputDir = "out"
		}, wantErr: "combines members"},
		{name: "bad prefix", mutate: func(c *Config) { c.Marketplace.FromDomains.NamePrefix = "A/" }, wantErr: "name_prefix"},
		{name: "malformed glob", mutate: func(c *Config) { c.Marketplace.FromDomains.Include = []string{"[x"} }, wantErr: "malformed glob"},
		{name: "unknown runtime", mutate: func(c *Config) { c.Marketplace.FromDomains.Runtimes = []string{"nope"} }, wantErr: "runtime"},
		{name: "plugin without content", mutate: func(c *Config) {
			c.Marketplace.Plugins = []MarketplacePlugin{{Name: "p"}}
		}, wantErr: "selects no content"},
		{name: "plugin bad name", mutate: func(c *Config) {
			c.Marketplace.Plugins = []MarketplacePlugin{{Name: "P/x", Domains: []string{"d"}}}
		}, wantErr: "not a valid plugin name"},
		{name: "duplicate plugin", mutate: func(c *Config) {
			c.Marketplace.Plugins = []MarketplacePlugin{{Name: "p", Domains: []string{"d"}}, {Name: "p", Domains: []string{"e"}}}
		}, wantErr: "duplicate plugin"},
		{name: "relevance without signals", mutate: func(c *Config) {
			c.Marketplace.Plugins = []MarketplacePlugin{{Name: "p", Domains: []string{"d"}, Relevance: &PluginRelevance{Topic: "t"}}}
		}, wantErr: "at least one signal"},
		{name: "relevance host with scheme", mutate: func(c *Config) {
			c.Marketplace.Plugins = []MarketplacePlugin{{Name: "p", Domains: []string{"d"},
				Relevance: &PluginRelevance{Signals: RelevanceSignals{Hosts: []string{"https://x.io"}}}}}
		}, wantErr: "bare hostname"},
		{name: "valid relevance", mutate: func(c *Config) {
			c.Marketplace.Plugins = []MarketplacePlugin{{Name: "p", Domains: []string{"d"},
				PluginDefaults: PluginDefaults{DefaultEnabled: boolPtr(false)},
				Relevance:      &PluginRelevance{Signals: RelevanceSignals{CLI: []string{"terraform"}}}}}
		}},
		{name: "catalog without domain plugins", mutate: func(c *Config) {
			c.Marketplace.FromDomains = nil
			c.Marketplace.CatalogSkill = &CatalogSkillConfig{Enabled: true}
		}, wantErr: "generates no domain plugins"},
		{name: "placement unknown default", mutate: func(c *Config) { c.Placement = &PlacementConfig{Default: "both"} }, wantErr: "not core or plugin"},
		{name: "placement core and plugin overlap", mutate: func(c *Config) {
			c.Placement = &PlacementConfig{Core: []string{"a"}, Plugin: []string{"./A"}}
		}, wantErr: "both core and plugin"},
		{name: "placement malformed glob", mutate: func(c *Config) { c.Placement = &PlacementConfig{Plugin: []string{"[a"}} }, wantErr: "malformed glob"},
		{name: "settings enable and disable same", mutate: func(c *Config) {
			c.Claude = &ClaudeConfig{Settings: &ClaudeSettings{Manage: true, EnablePlugins: []string{"p"}, DisablePlugins: []string{"p"}}}
		}, wantErr: "both enabled and disabled"},
		{name: "settings without marketplace", mutate: func(c *Config) {
			c.Marketplace = nil
			c.Claude = &ClaudeConfig{Settings: &ClaudeSettings{Manage: true}}
		}, wantErr: "needs a [marketplace] name"},
		{name: "settings unmanaged is not validated", mutate: func(c *Config) {
			c.Marketplace = nil
			c.Claude = &ClaudeConfig{Settings: &ClaudeSettings{EnablePlugins: []string{"P/"}}}
		}},
		{name: "settings source missing field", mutate: func(c *Config) {
			c.Claude = &ClaudeConfig{Settings: &ClaudeSettings{Manage: true, MarketplaceSource: &MarketplaceSource{Source: "github"}}}
		}, wantErr: "requires \"repo\""},
		{name: "settings source unknown kind", mutate: func(c *Config) {
			c.Claude = &ClaudeConfig{Settings: &ClaudeSettings{Manage: true, MarketplaceSource: &MarketplaceSource{Source: "ftp"}}}
		}, wantErr: "unsupported marketplace source"},
		{name: "plugin include_domains malformed", mutate: func(c *Config) {
			c.Plugin = &PluginAuthoring{Name: "p", Version: "1.0.0", Description: "d", Runtimes: []string{PluginRuntimeClaude}, IncludeDomains: []string{"[x"}}
		}, wantErr: "malformed glob"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := base()
			tt.mutate(cfg)

			// Act
			err := firstError(cfg.validateMarketplaceAuthoring(), cfg.validatePlacement(), cfg.validateClaudeSettings(), cfg.validatePluginAuthoring())

			// Assert
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func firstError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
