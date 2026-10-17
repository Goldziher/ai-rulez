package policy

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lockfile"
)

func pluginLockedConfig(t *testing.T, marketplace, plugin string) *config.Config {
	t.Helper()
	cfg := testConfig(t, "name = \"x\"\n")
	body := "version = 1\n\n[[marketplace]]\nname = \"acme\"\nsource = \"acme/plugins\"\ncommit = \"aaaa\"\ndigest = \"" + marketplace + "\"\n\n" +
		"[[plugin]]\nname = \"tools\"\nsource = \"marketplace:acme\"\ncommit = \"bbbb\"\ndigest = \"" + plugin + "\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(cfg.ConfigDir, lockfile.FileName), []byte(body), 0o644))
	cfg.InstalledMarketplaces = []config.InstalledMarketplaceConfig{{Name: "acme", Source: "acme/plugins"}, {Name: "other", Source: "acme/other"}}
	cfg.InstalledPlugins = []config.InstalledPluginConfig{{Name: "tools", Marketplace: "acme"}, {Name: "direct", Source: "acme/direct"}}
	return cfg
}

func TestDenyDigests_DropsMarketplacesAndPlugins(t *testing.T) {
	tests := []struct {
		name        string
		marketplace string
		plugin      string
		wantMarkets []string
		wantPlugins []string
		wantKinds   []string
	}{
		{"nothing denied", goodDigest, goodDigest, []string{"acme", "other"}, []string{"tools", "direct"}, nil},
		{"denied plugin", goodDigest, badDigest, []string{"acme", "other"}, []string{"direct"}, []string{lockfile.KindPlugin}},
		{"denied marketplace", badDigest, goodDigest, []string{"other"}, []string{"tools", "direct"}, []string{lockfile.KindMarketplace}},
		{"both denied", badDigest, badDigest, []string{"other"}, []string{"direct"}, []string{lockfile.KindMarketplace, lockfile.KindPlugin}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := pluginLockedConfig(t, tt.marketplace, tt.plugin)
			res := Resolve([]Layer{layer("managed", Policy{Sources: Sources{DenyDigests: []string{badDigest}}})})

			// Act
			out := res.Apply(cfg).Outcome

			// Assert
			var markets, plugins, kinds []string
			for _, m := range cfg.InstalledMarketplaces {
				markets = append(markets, m.Name)
			}
			for _, p := range cfg.InstalledPlugins {
				plugins = append(plugins, p.Name)
			}
			for _, v := range out.Violations {
				require.NotNil(t, v.Subject)
				kinds = append(kinds, v.Subject.Kind)
				assert.Equal(t, badDigest, v.Subject.Digest)
			}
			assert.Equal(t, tt.wantMarkets, markets)
			assert.Equal(t, tt.wantPlugins, plugins)
			assert.Equal(t, tt.wantKinds, kinds)
		})
	}
}

func TestSources_HostRulesApplyToMarketplacesAndPlugins(t *testing.T) {
	// Arrange
	cfg := testConfig(t, "name = \"x\"\n")
	cfg.InstalledMarketplaces = []config.InstalledMarketplaceConfig{
		{Name: "good", Source: "github.com/acme/plugins"}, {Name: "bad", Source: "https://evil.example/m.git"}, {Name: "local", Source: "./vendor/m"},
		{Name: "url", Source: "https://evil.example/marketplace.json"},
	}
	cfg.InstalledPlugins = []config.InstalledPluginConfig{
		{Name: "good", Source: "https://github.com/acme/p"}, {Name: "bad", Source: "https://evil.example/p.git"}, {Name: "via", Marketplace: "good"},
	}
	res := Resolve([]Layer{layer("managed", Policy{Sources: Sources{Allowed: List{Set: true, Items: []string{"github.com/acme"}}}})})

	// Act
	out := res.Apply(cfg).Outcome

	// Assert
	var markets, plugins []string
	for _, m := range cfg.InstalledMarketplaces {
		markets = append(markets, m.Name)
	}
	for _, p := range cfg.InstalledPlugins {
		plugins = append(plugins, p.Name)
	}
	assert.Equal(t, []string{"good", "local"}, markets)
	assert.Equal(t, []string{"good", "via"}, plugins)
	assert.Len(t, out.Violations, 3)
}

func TestMinReleaseAgeFloor_AppliesToMarketplacesAndPlugins(t *testing.T) {
	// Arrange
	cfg := testConfig(t, "name = \"x\"\n")
	cfg.InstalledMarketplaces = []config.InstalledMarketplaceConfig{{Name: "m", Source: "acme/m", Version: "^1", MinReleaseAge: "1d"}}
	cfg.InstalledPlugins = []config.InstalledPluginConfig{{Name: "p", Source: "acme/p", Version: "^1", MinReleaseAge: "1d"}}
	res := Resolve([]Layer{layer("managed", Policy{Sources: Sources{MinReleaseAge: 7 * 24 * time.Hour}})})

	// Act
	out := res.Apply(cfg).Outcome

	// Assert
	assert.Equal(t, "7d", cfg.InstalledMarketplaces[0].MinReleaseAge)
	assert.Equal(t, "7d", cfg.InstalledPlugins[0].MinReleaseAge)
	assert.Len(t, out.Violations, 2)
}

func TestApplyContent_PluginHooksAndMCPServers(t *testing.T) {
	newConfig := func() *config.Config {
		cfg := contentConfig(t)
		cfg.Hooks = []config.HookGroup{
			{Event: "Stop", Builtin: config.HookBuiltinPlugin, Hooks: []config.HookAction{{Command: "plugin-hook"}}},
			{Event: "Stop", Hooks: []config.HookAction{{Command: "own-hook"}}},
		}
		cfg.MCPServers = map[string]*config.MCPServer{
			"own":       {Name: "own", Command: "bash"},
			"allowed":   {Name: "allowed", Command: "npx"},
			"forbidden": {Name: "forbidden", Command: "bash"},
			"remote":    {Name: "remote", URL: "https://e.com/mcp"},
			"stream":    {Name: "stream", Type: "sse", URL: "https://e.com/sse"},
		}
		cfg.PluginMCPServers = []string{"allowed", "forbidden", "remote", "stream"}
		return cfg
	}
	t.Run("hooks forbidden drops only the plugin's", func(t *testing.T) {
		cfg := newConfig()
		res := Resolve([]Layer{layer("managed", Policy{Hooks: Hooks{Forbidden: true}})})

		got := res.ApplyContent(cfg)

		require.Len(t, got, 1)
		assert.Equal(t, "AR748", got[0].Code)
		assert.Equal(t, "hooks.allow", got[0].Key)
		require.Len(t, cfg.Hooks, 1)
		assert.Equal(t, "own-hook", cfg.Hooks[0].Hooks[0].Command)
	})
	t.Run("mcp commands and transports are bounded", func(t *testing.T) {
		cfg := newConfig()
		res := Resolve([]Layer{layer("managed", Policy{MCP: MCP{AllowedCommands: List{Set: true, Items: []string{"npx"}}, DenyTransports: []string{"sse"}}})})

		got := res.ApplyContent(cfg)

		assert.Len(t, got, 2, "bash (stdio, not allowed) and the sse server")
		assert.Contains(t, cfg.MCPServers, "own", "the repository's own server is bounded by config.toml, not by this check")
		assert.Contains(t, cfg.MCPServers, "allowed")
		assert.Contains(t, cfg.MCPServers, "remote")
		assert.NotContains(t, cfg.MCPServers, "forbidden")
		assert.NotContains(t, cfg.MCPServers, "stream")
	})
	t.Run("no policy changes nothing", func(t *testing.T) {
		cfg := newConfig()
		assert.Empty(t, Resolve([]Layer{layer("managed", Policy{})}).ApplyContent(cfg))
		assert.Len(t, cfg.Hooks, 2)
		assert.Len(t, cfg.MCPServers, 5)
	})
}
