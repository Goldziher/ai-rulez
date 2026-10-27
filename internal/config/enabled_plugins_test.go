package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeEnabledPlugins(t *testing.T) {
	// Arrange
	body := `
version = "5.0"
name = "x"

[[enabled_plugins]]
id = "basemind@basemind"
marketplace = { name = "basemind", source = "Goldziher/basemind", type = "github" }
opencode = "basemind-opencode"

[[enabled_plugins]]
id = "tool@official"
`

	// Act
	cfg, err := decodeConfigTOML([]byte(body), "config.toml")

	// Assert
	require.NoError(t, err)
	require.Len(t, cfg.EnabledPlugins, 2)
	first := cfg.EnabledPlugins[0]
	assert.Equal(t, "basemind@basemind", first.ID)
	assert.Equal(t, "basemind-opencode", first.OpenCode)
	require.NotNil(t, first.Marketplace)
	assert.Equal(t, "basemind", first.Marketplace.Name)
	assert.Equal(t, "Goldziher/basemind", first.Marketplace.Source)
	assert.Equal(t, "github", first.Marketplace.Type)
	assert.Nil(t, cfg.EnabledPlugins[1].Marketplace)
	assert.True(t, cfg.HasEnabledPlugins())
}

func TestValidateEnabledPlugins(t *testing.T) {
	github := &EnabledPluginMarketplace{Name: "basemind", Source: "Goldziher/basemind", Type: "github"}
	market := func(source, kind string) *EnabledPluginMarketplace {
		return &EnabledPluginMarketplace{Name: "b", Source: source, Type: kind}
	}
	tests := []struct {
		name    string
		plugins []EnabledPlugin
		wantErr string
	}{
		{name: "id only", plugins: []EnabledPlugin{{ID: "tool@official"}}},
		{name: "github marketplace and opencode", plugins: []EnabledPlugin{{ID: "basemind@basemind", Marketplace: github, OpenCode: "basemind-opencode"}}},
		{name: "scoped and versioned npm package", plugins: []EnabledPlugin{{ID: "a@b", OpenCode: "@scope/pkg@1.2.3"}}},
		{name: "git marketplace with ref", plugins: []EnabledPlugin{{ID: "a@b", Marketplace: &EnabledPluginMarketplace{
			Name: "b", Source: "https://example.com/b.git", Type: "git", Ref: "v1"}}}},
		{name: "scp style git source", plugins: []EnabledPlugin{{ID: "a@b", Marketplace: market("git@github.com:org/b.git", "git")}}},
		{name: "directory marketplace", plugins: []EnabledPlugin{{ID: "a@b", Marketplace: market("./plugins", "directory")}}},
		{name: "same marketplace declared twice identically", plugins: []EnabledPlugin{
			{ID: "a@basemind", Marketplace: github}, {ID: "b@basemind", Marketplace: github}}},
		{name: "empty id", plugins: []EnabledPlugin{{}}, wantErr: "enabled_plugins[0].id"},
		{name: "bare name", plugins: []EnabledPlugin{{ID: "basemind"}}, wantErr: "name@marketplace"},
		{name: "two at signs", plugins: []EnabledPlugin{{ID: "a@b@c"}}, wantErr: "name@marketplace"},
		{name: "uppercase and slash", plugins: []EnabledPlugin{{ID: "A/b@c"}}, wantErr: "not a valid plugin id"},
		{name: "traversal", plugins: []EnabledPlugin{{ID: "a..b@c"}}, wantErr: "not a valid plugin id"},
		{name: "quote in id", plugins: []EnabledPlugin{{ID: `a"@c`}}, wantErr: "not a valid plugin id"},
		{name: "duplicate id", plugins: []EnabledPlugin{{ID: "a@b"}, {ID: "a@b"}}, wantErr: "listed twice"},
		{name: "marketplace name differs from id", plugins: []EnabledPlugin{{ID: "a@other", Marketplace: github}}, wantErr: "does not match"},
		{name: "marketplace unknown type", plugins: []EnabledPlugin{{ID: "a@b", Marketplace: market("x", "ftp")}}, wantErr: "unsupported marketplace type"},
		{name: "marketplace missing type", plugins: []EnabledPlugin{{ID: "a@b", Marketplace: market("o/r", "")}}, wantErr: "unsupported marketplace type"},
		{name: "marketplace missing source", plugins: []EnabledPlugin{{ID: "a@b", Marketplace: market("", "github")}}, wantErr: "needs a source"},
		{name: "github source not owner repo", plugins: []EnabledPlugin{{ID: "a@b", Marketplace: market("https://github.com/o/r", "github")}}, wantErr: "owner/repo"},
		{name: "http git source rejected", plugins: []EnabledPlugin{{ID: "a@b", Marketplace: market("http://example.com/b.git", "git")}}, wantErr: "https"},
		{name: "git protocol rejected", plugins: []EnabledPlugin{{ID: "a@b", Marketplace: market("git://example.com/b.git", "url")}}, wantErr: "https"},
		{name: "directory escaping project", plugins: []EnabledPlugin{{ID: "a@b", Marketplace: market("../elsewhere", "directory")}}, wantErr: "project-relative"},
		{name: "ref with whitespace", plugins: []EnabledPlugin{{ID: "a@b", Marketplace: &EnabledPluginMarketplace{
			Name: "b", Source: "o/r", Type: "github", Ref: "a b"}}}, wantErr: "ref"},
		{name: "conflicting marketplace sources", plugins: []EnabledPlugin{
			{ID: "a@basemind", Marketplace: github},
			{ID: "b@basemind", Marketplace: &EnabledPluginMarketplace{Name: "basemind", Source: "other/repo", Type: "github"}},
		}, wantErr: "declared twice"},
		{name: "opencode path rejected", plugins: []EnabledPlugin{{ID: "a@b", OpenCode: "./local.js"}}, wantErr: "npm package"},
		{name: "opencode with whitespace rejected", plugins: []EnabledPlugin{{ID: "a@b", OpenCode: "pkg --flag"}}, wantErr: "npm package"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := &Config{Version: "5.0", Name: "x", EnabledPlugins: tt.plugins}

			// Act
			err := cfg.validateEnabledPlugins()

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

func TestClaudeSettingsQualifiedPluginIDs(t *testing.T) {
	yes := true
	tests := []struct {
		name    string
		cfg     *Config
		wantErr string
	}{
		{name: "qualified id without a local marketplace", cfg: &Config{Claude: &ClaudeConfig{Settings: &ClaudeSettings{
			Manage: true, EnablePlugins: []string{"basemind@basemind"}}}}},
		{name: "qualified id with a local marketplace", cfg: &Config{Marketplace: &MarketplaceAuthoring{Name: "m"},
			Claude: &ClaudeConfig{Settings: &ClaudeSettings{Manage: true, EnablePlugins: []string{"basemind@basemind", "own"}}}}},
		{name: "bare name still needs the local marketplace", cfg: &Config{Claude: &ClaudeConfig{Settings: &ClaudeSettings{
			Manage: true, EnablePlugins: []string{"own"}}}}, wantErr: "needs a [marketplace] name"},
		{name: "explicit register still needs the local marketplace", cfg: &Config{Claude: &ClaudeConfig{Settings: &ClaudeSettings{
			Manage: true, RegisterMarketplace: &yes, EnablePlugins: []string{"a@b"}}}}, wantErr: "needs a [marketplace] name"},
		{name: "bad marketplace part", cfg: &Config{Claude: &ClaudeConfig{Settings: &ClaudeSettings{
			Manage: true, EnablePlugins: []string{"a@B/"}}}}, wantErr: "not a valid plugin name"},
		{name: "two at signs", cfg: &Config{Claude: &ClaudeConfig{Settings: &ClaudeSettings{
			Manage: true, EnablePlugins: []string{"a@b@c"}}}}, wantErr: "not a valid plugin name"},
		{name: "enable and disable the same qualified id", cfg: &Config{Marketplace: &MarketplaceAuthoring{Name: "m"},
			Claude: &ClaudeConfig{Settings: &ClaudeSettings{Manage: true, EnablePlugins: []string{"own"}, DisablePlugins: []string{"own@m"}}}},
			wantErr: "both enabled and disabled"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			err := tt.cfg.validateClaudeSettings()

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

func TestQualifiedPluginID(t *testing.T) {
	cfg := &Config{Marketplace: &MarketplaceAuthoring{Name: "local"}}

	assert.Equal(t, "own@local", cfg.QualifiedPluginID("own"))
	assert.Equal(t, "basemind@basemind", cfg.QualifiedPluginID("basemind@basemind"))
	assert.Equal(t, "own", (&Config{}).QualifiedPluginID("own"))
}

func TestEnabledPluginMarketplaceClaudeSource(t *testing.T) {
	tests := []struct {
		name string
		in   EnabledPluginMarketplace
		want MarketplaceSource
	}{
		{"github", EnabledPluginMarketplace{Source: "o/r", Type: "github", Ref: "main"}, MarketplaceSource{Source: "github", Repo: "o/r", Ref: "main"}},
		{"git", EnabledPluginMarketplace{Source: "https://h/r.git", Type: "git"}, MarketplaceSource{Source: "git", URL: "https://h/r.git"}},
		{"url", EnabledPluginMarketplace{Source: "https://h/m.json", Type: "url"}, MarketplaceSource{Source: "url", URL: "https://h/m.json"}},
		{"directory", EnabledPluginMarketplace{Source: "./p", Type: "directory"}, MarketplaceSource{Source: "directory", Path: "./p"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, *tt.in.ClaudeSource())
		})
	}
}

func TestValidateEnabledPluginsAgainstClaudeSettings(t *testing.T) {
	yes := true
	local := &MarketplaceAuthoring{Name: "m"}
	settings := func(s ClaudeSettings) *ClaudeConfig {
		s.Manage = true
		return &ClaudeConfig{Settings: &s}
	}
	dir := &EnabledPluginMarketplace{Name: "b", Source: "./plugins", Type: "directory"}
	gh := &EnabledPluginMarketplace{Name: "m", Source: "o/r", Type: "github"}
	tests := []struct {
		name    string
		cfg     *Config
		wantErr string
	}{
		{name: "listed in disable_plugins", cfg: &Config{Claude: settings(ClaudeSettings{DisablePlugins: []string{"a@b"}}),
			EnabledPlugins: []EnabledPlugin{{ID: "a@b"}}}, wantErr: `plugin "a@b" is enabled in enabled_plugins and disabled in claude.settings.disable_plugins`},
		{name: "bare disable name qualified with the local marketplace", cfg: &Config{Marketplace: local,
			Claude: settings(ClaudeSettings{DisablePlugins: []string{"own"}}), EnabledPlugins: []EnabledPlugin{{ID: "own@m"}}},
			wantErr: "disabled in claude.settings.disable_plugins"},
		{name: "different ids do not conflict", cfg: &Config{Marketplace: local,
			Claude: settings(ClaudeSettings{DisablePlugins: []string{"own"}}), EnabledPlugins: []EnabledPlugin{{ID: "own@other"}}}},
		{name: "also in enable_plugins is only redundant", cfg: &Config{Marketplace: local,
			Claude: settings(ClaudeSettings{EnablePlugins: []string{"own"}}), EnabledPlugins: []EnabledPlugin{{ID: "own@m"}}}},
		{name: "marketplace named like the registered local one", cfg: &Config{Marketplace: local,
			Claude: settings(ClaudeSettings{RegisterMarketplace: &yes}), EnabledPlugins: []EnabledPlugin{{ID: "a@m", Marketplace: gh}}},
			wantErr: `marketplace "m" is the marketplace of this project`},
		{name: "same name but the local marketplace is not registered", cfg: &Config{Marketplace: local,
			EnabledPlugins: []EnabledPlugin{{ID: "a@m", Marketplace: gh}}}},
		{name: "directory marketplace in project scope", cfg: &Config{EnabledPlugins: []EnabledPlugin{{ID: "a@b", Marketplace: dir}}}},
		{name: "directory marketplace in user scope", cfg: &Config{UserScope: true, EnabledPlugins: []EnabledPlugin{{ID: "a@b", Marketplace: dir}}},
			wantErr: "cannot use a directory marketplace in the user config"},
		{name: "github marketplace in user scope", cfg: &Config{UserScope: true, EnabledPlugins: []EnabledPlugin{
			{ID: "a@m", Marketplace: gh}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			err := tt.cfg.validateEnabledPlugins()

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
