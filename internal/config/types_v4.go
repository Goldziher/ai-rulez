package config

// PluginConfig represents a plugin to install from a marketplace
type PluginConfig struct {
	Marketplace string `yaml:"marketplace" json:"marketplace" toml:"marketplace"`
	Name        string `yaml:"name" json:"name" toml:"name"`
	Scope       string `yaml:"scope,omitempty" json:"scope,omitempty" toml:"scope,omitempty"` // "project" or "user"
	Enabled     *bool  `yaml:"enabled,omitempty" json:"enabled,omitempty" toml:"enabled,omitempty"`
}

// IsEnabled returns true if the plugin is enabled (defaults to true)
func (p *PluginConfig) IsEnabled() bool {
	if p == nil || p.Enabled == nil {
		return true
	}
	return *p.Enabled
}

// GetScope returns the scope, defaulting to "project"
func (p *PluginConfig) GetScope() string {
	if p == nil || p.Scope == "" {
		return "project"
	}
	return p.Scope
}

// MarketplaceConfig represents a custom plugin marketplace
type MarketplaceConfig struct {
	Name   string `yaml:"name" json:"name" toml:"name"`
	Source string `yaml:"source" json:"source" toml:"source"`
	Type   string `yaml:"type" json:"type" toml:"type"` // "github", "git", "local", "url"
}
