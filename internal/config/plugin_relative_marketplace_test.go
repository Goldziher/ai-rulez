package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRelativeDirectoryMarketplace(t *testing.T) {
	t.Parallel()
	off := false
	tests := []struct {
		name     string
		cfg      Config
		wantPath string
		wantOK   bool
	}{
		{"settings not managed", Config{}, "", false},
		{
			"default directory source",
			Config{Marketplace: &MarketplaceAuthoring{OutputDir: "mkt"}, Claude: &ClaudeConfig{Settings: &ClaudeSettings{Manage: true}}},
			"./mkt", true,
		},
		{
			"default source at the project root",
			Config{Claude: &ClaudeConfig{Settings: &ClaudeSettings{Manage: true}}},
			".", true,
		},
		{
			"marketplace registration off",
			Config{Claude: &ClaudeConfig{Settings: &ClaudeSettings{Manage: true, RegisterMarketplace: &off}}},
			"", false,
		},
		{
			"absolute directory",
			Config{Claude: &ClaudeConfig{Settings: &ClaudeSettings{Manage: true, MarketplaceSource: &MarketplaceSource{Source: "directory", Path: "/opt/mkt"}}}},
			"", false,
		},
		{
			"relative explicit directory",
			Config{Claude: &ClaudeConfig{Settings: &ClaudeSettings{Manage: true, MarketplaceSource: &MarketplaceSource{Source: "directory", Path: "./m"}}}},
			"./m", true,
		},
		{
			"github source",
			Config{Claude: &ClaudeConfig{Settings: &ClaudeSettings{Manage: true, MarketplaceSource: &MarketplaceSource{Source: "github", Repo: "o/r"}}}},
			"", false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path, ok := tt.cfg.RelativeDirectoryMarketplace()
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantPath, path)
		})
	}
}
