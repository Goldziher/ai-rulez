package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfigTOML_PluginAuthoring(t *testing.T) {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "config.toml")
	content := `
version = "4.0"
name = "basemind"
description = "Code-map MCP server."

[plugin]
name = "basemind"
display_name = "Basemind"
description = "Full AI context layer over MCP."
version = "0.19.2"
homepage = "https://github.com/Goldziher/basemind"
repository = "https://github.com/Goldziher/basemind"
license = "MIT"
keywords = ["mcp", "rag"]
runtimes = ["claude", "cursor", "codex"]

[plugin.author]
name = "Na'aman Hirschfeld"
email = "nhirschfeld@gmail.com"

[[plugin.mcp]]
name = "basemind"
command = "${PLUGIN_ROOT}/scripts/mcp-launch.sh"
args = ["serve"]

[[plugin.hooks]]
event = "SessionStart"
matcher = "startup|resume"

[[plugin.hooks.hooks]]
type = "command"
command = "${PLUGIN_ROOT}/hooks/run-hook.cmd session-start"
async = false

[plugin.statusline]
script = ".claude-plugin/statusline.sh"
command = "bm-statusline"

[plugin.kimi]
skill_instructions = "Prefer basemind tools over grep."
session_start_skill = "basemind"

[marketplace]
name = "basemind"
description = "basemind marketplace"

[marketplace.owner]
name = "Na'aman Hirschfeld"
email = "nhirschfeld@gmail.com"
`
	require.NoError(t, os.WriteFile(configFile, []byte(content), 0o644))

	cfg, err := loadConfigTOML(configFile)
	require.NoError(t, err)

	require.NotNil(t, cfg.Plugin)
	assert.Equal(t, "basemind", cfg.Plugin.Name)
	assert.Equal(t, "Basemind", cfg.Plugin.DisplayName)
	assert.Equal(t, "0.19.2", cfg.Plugin.Version)
	assert.Equal(t, []string{"mcp", "rag"}, cfg.Plugin.Keywords)
	assert.Equal(t, []string{"claude", "cursor", "codex"}, cfg.Plugin.Runtimes)

	require.NotNil(t, cfg.Plugin.Author)
	assert.Equal(t, "nhirschfeld@gmail.com", cfg.Plugin.Author.Email)

	require.Len(t, cfg.Plugin.MCP, 1)
	assert.Equal(t, "${PLUGIN_ROOT}/scripts/mcp-launch.sh", cfg.Plugin.MCP[0].Command)
	assert.Equal(t, []string{"serve"}, cfg.Plugin.MCP[0].Args)

	require.Len(t, cfg.Plugin.Hooks, 1)
	assert.Equal(t, "SessionStart", cfg.Plugin.Hooks[0].Event)
	require.Len(t, cfg.Plugin.Hooks[0].Hooks, 1)
	assert.Equal(t, "command", cfg.Plugin.Hooks[0].Hooks[0].Type)

	require.NotNil(t, cfg.Plugin.Statusline)
	assert.Equal(t, "bm-statusline", cfg.Plugin.Statusline.Command)

	require.NotNil(t, cfg.Plugin.Kimi)
	assert.Equal(t, "basemind", cfg.Plugin.Kimi.SessionStartSkill)

	require.NotNil(t, cfg.Marketplace)
	assert.Equal(t, "basemind", cfg.Marketplace.Name)
	require.NotNil(t, cfg.Marketplace.Owner)
	assert.Equal(t, "Na'aman Hirschfeld", cfg.Marketplace.Owner.Name)
}

func TestResolvedRuntimes(t *testing.T) {
	t.Run("empty returns all runtimes", func(t *testing.T) {
		p := &PluginAuthoring{}
		assert.Equal(t, AllPluginRuntimes, p.ResolvedRuntimes())
	})
	t.Run("explicit list is returned verbatim", func(t *testing.T) {
		p := &PluginAuthoring{Runtimes: []string{"claude", "codex"}}
		assert.Equal(t, []string{"claude", "codex"}, p.ResolvedRuntimes())
	})
	t.Run("nil receiver returns all runtimes", func(t *testing.T) {
		var p *PluginAuthoring
		assert.Equal(t, AllPluginRuntimes, p.ResolvedRuntimes())
	})
}

func TestValidatePluginAuthoring(t *testing.T) {
	base := func() *PluginAuthoring {
		return &PluginAuthoring{
			Name:        "basemind",
			Version:     "0.19.2",
			Description: "Full AI context layer.",
			Runtimes:    []string{"claude"},
		}
	}

	tests := []struct {
		name    string
		mutate  func(*PluginAuthoring)
		wantErr string
	}{
		{name: "valid minimal", mutate: func(*PluginAuthoring) {}},
		{name: "missing name", mutate: func(p *PluginAuthoring) { p.Name = "" }, wantErr: "requires field 'name'"},
		{name: "missing version", mutate: func(p *PluginAuthoring) { p.Version = "" }, wantErr: "requires field 'version'"},
		{name: "bad version", mutate: func(p *PluginAuthoring) { p.Version = "not-a-version" }, wantErr: "invalid version"},
		{name: "good prerelease version", mutate: func(p *PluginAuthoring) { p.Version = "1.2.3-beta.1" }},
		{name: "short version", mutate: func(p *PluginAuthoring) { p.Version = "1.2" }, wantErr: "invalid version"},
		{name: "missing description", mutate: func(p *PluginAuthoring) { p.Description = "" }, wantErr: "requires field 'description'"},
		{name: "safe content root", mutate: func(p *PluginAuthoring) { p.ContentRoot = "plugin" }},
		{name: "absolute content root", mutate: func(p *PluginAuthoring) { p.ContentRoot = "/tmp/plugin" }, wantErr: "unsafe content root"},
		{name: "Windows absolute content root", mutate: func(p *PluginAuthoring) { p.ContentRoot = `C:\tmp\plugin` }, wantErr: "unsafe content root"},
		{name: "escaping content root", mutate: func(p *PluginAuthoring) { p.ContentRoot = "../plugin" }, wantErr: "unsafe content root"},
		{name: "Windows escaping Hermes source", mutate: func(p *PluginAuthoring) {
			p.Hermes = &HermesExtras{Source: `..\outside.py`}
		}, wantErr: "unsafe Hermes source"},
		{name: "unknown runtime", mutate: func(p *PluginAuthoring) { p.Runtimes = []string{"claude", "bogus"} }, wantErr: "unknown runtime"},
		{name: "duplicate runtime", mutate: func(p *PluginAuthoring) { p.Runtimes = []string{"claude", "claude"} }, wantErr: "duplicate runtime"},
		{name: "Hermes runtime", mutate: func(p *PluginAuthoring) { p.Runtimes = []string{"hermes"} }},
		{name: "mcp missing name", mutate: func(p *PluginAuthoring) { p.MCP = []PluginMCPLaunch{{Command: "x"}} }, wantErr: "MCP entry"},
		{name: "stdio mcp missing command", mutate: func(p *PluginAuthoring) { p.MCP = []PluginMCPLaunch{{Name: "s"}} }, wantErr: "no command"},
		{name: "http mcp missing url", mutate: func(p *PluginAuthoring) {
			p.MCP = []PluginMCPLaunch{{Name: "r", Transport: "http"}}
		}, wantErr: "no url"},
		{name: "http mcp with url ok", mutate: func(p *PluginAuthoring) {
			p.MCP = []PluginMCPLaunch{{Name: "r", Transport: "http", URL: "https://x"}}
		}},
		{name: "statusline missing script", mutate: func(p *PluginAuthoring) { p.Statusline = &Statusline{Command: "bm"} }, wantErr: "statusline requires 'script'"},
		{name: "hook missing event", mutate: func(p *PluginAuthoring) { p.Hooks = []HookGroup{{Matcher: "x"}} }, wantErr: "missing 'event'"},
		{name: "hook action missing command", mutate: func(p *PluginAuthoring) {
			p.Hooks = []HookGroup{{Event: "SessionStart", Hooks: []HookAction{{Type: "command"}}}}
		}, wantErr: "missing 'command'"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := base()
			tc.mutate(p)
			cfg := &Config{Plugin: p}
			err := cfg.validatePluginAuthoring()
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestValidatePluginAuthoring_CodexRequiresCanonicalMetadata(t *testing.T) {
	plugin := &PluginAuthoring{
		Name:        "basemind",
		Version:     "1.0.0",
		Description: "Code map.",
		Runtimes:    []string{"codex"},
	}
	err := (&Config{Plugin: plugin}).validatePluginAuthoring()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "author.name")

	plugin.Author = &Author{Name: "Goldziher"}
	plugin.Interface = &PluginInterface{
		DisplayName:      "basemind",
		ShortDescription: "Code map",
		LongDescription:  "Navigate source code with a structural index.",
		DeveloperName:    "Goldziher",
		Category:         "Developer Tools",
		Capabilities:     []string{"Read"},
		DefaultPrompt:    []string{"Map this repository."},
	}
	require.NoError(t, (&Config{Plugin: plugin}).validatePluginAuthoring())
}

func TestValidatePluginAuthoring_NilIsValid(t *testing.T) {
	cfg := &Config{}
	require.NoError(t, cfg.validatePluginAuthoring())
	require.NoError(t, cfg.validateMarketplaceAuthoring())
}

func TestValidateMarketplaceAuthoring(t *testing.T) {
	t.Run("missing name", func(t *testing.T) {
		cfg := &Config{Marketplace: &MarketplaceAuthoring{}}
		err := cfg.validateMarketplaceAuthoring()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires field 'name'")
	})
	t.Run("duplicate member", func(t *testing.T) {
		cfg := &Config{Marketplace: &MarketplaceAuthoring{
			Name:    "mp",
			Members: []string{"plugins/a", "plugins/a"},
		}}
		err := cfg.validateMarketplaceAuthoring()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate member")
	})
	t.Run("member path traversal rejected", func(t *testing.T) {
		for _, bad := range []string{"../outside", `plugins\..\..\etc`, "/abs/path", `C:\abs\path`} {
			cfg := &Config{Marketplace: &MarketplaceAuthoring{Name: "mp", Members: []string{bad}}}
			err := cfg.validateMarketplaceAuthoring()
			require.Error(t, err, "expected %q to be rejected", bad)
			assert.Contains(t, err.Error(), "invalid member path")
		}
	})
	t.Run("valid", func(t *testing.T) {
		cfg := &Config{Marketplace: &MarketplaceAuthoring{
			Name:    "mp",
			Members: []string{"plugins/a", "plugins/b"},
		}}
		require.NoError(t, cfg.validateMarketplaceAuthoring())
	})
}
