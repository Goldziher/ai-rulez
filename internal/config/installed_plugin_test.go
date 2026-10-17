package config

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

func TestClassifyPluginSource(t *testing.T) {
	tests := []struct {
		source string
		want   string
		url    string
	}{
		{"acme/plugins", PluginSourceGitHub, "https://github.com/acme/plugins.git"},
		{"github:acme/plugins", PluginSourceGitHub, "https://github.com/acme/plugins.git"},
		{"acme/plugins.git", PluginSourceGitHub, "https://github.com/acme/plugins.git"},
		{"https://github.com/acme/plugins", PluginSourceGit, "https://github.com/acme/plugins"},
		{"git+https://example.com/x.git", PluginSourceGit, "https://example.com/x.git"},
		{"git@github.com:acme/p.git", PluginSourceGit, "git@github.com:acme/p.git"},
		{"ssh://git@example.com/x.git", PluginSourceGit, "ssh://git@example.com/x.git"},
		{"file:///srv/repo", PluginSourceGit, "file:///srv/repo"},
		{"https://example.com/marketplace.json", PluginSourceIndexURL, "https://example.com/marketplace.json"},
		{"https://example.com/m.json?token=x", PluginSourceIndexURL, "https://example.com/m.json?token=x"},
		{"./vendor/m", PluginSourceLocal, "./vendor/m"},
		{"../shared", PluginSourceLocal, "../shared"},
		{"/abs/dir", PluginSourceLocal, "/abs/dir"},
		{"~/plugins", PluginSourceLocal, "~/plugins"},
		{".", PluginSourceLocal, "."},
		{"C:/plugins", PluginSourceLocal, "C:/plugins"},
		{"a/b/c", PluginSourceLocal, "a/b/c"},
	}
	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			assert.Equal(t, tt.want, ClassifyPluginSource(tt.source))
			assert.Equal(t, tt.url, PluginGitURL(tt.source))
		})
	}
}

func TestValidateInstalledMarketplaces(t *testing.T) {
	tests := []struct {
		name    string
		in      []InstalledMarketplaceConfig
		wantErr string
	}{
		{"github shorthand", []InstalledMarketplaceConfig{{Name: "m", Source: "acme/plugins"}}, ""},
		{"https repository with a range", []InstalledMarketplaceConfig{{Name: "m", Source: "https://github.com/a/b", Version: "^1.2"}}, ""},
		{"index url", []InstalledMarketplaceConfig{{Name: "m", Source: "https://example.com/m.json"}}, ""},
		{"local directory", []InstalledMarketplaceConfig{{Name: "m", Source: "./vendor/m"}}, ""},
		{"missing name", []InstalledMarketplaceConfig{{Source: "a/b"}}, "name"},
		{"missing source", []InstalledMarketplaceConfig{{Name: "m"}}, "source"},
		{"duplicate", []InstalledMarketplaceConfig{{Name: "m", Source: "a/b"}, {Name: "m", Source: "c/d"}}, "duplicate"},
		{"plain http", []InstalledMarketplaceConfig{{Name: "m", Source: "http://example.com/x.git"}}, "http://"},
		{"plain http index", []InstalledMarketplaceConfig{{Name: "m", Source: "http://example.com/m.json"}}, "http://"},
		{"credentials in a url", []InstalledMarketplaceConfig{{Name: "m", Source: "https://user:pw@example.com/x.git"}}, "credentials"},
		{"token user in a url", []InstalledMarketplaceConfig{{Name: "m", Source: "https://ghp_x@example.com/x.git"}}, "credentials"},
		{"credentials in an index url", []InstalledMarketplaceConfig{{Name: "m", Source: "https://u:p@example.com/m.json"}}, "credentials"},
		{"option source", []InstalledMarketplaceConfig{{Name: "m", Source: "--upload-pack=x"}}, "option"},
		{"transport helper", []InstalledMarketplaceConfig{{Name: "m", Source: "ext::sh -c id"}}, "transport helper"},
		{"control characters", []InstalledMarketplaceConfig{{Name: "m", Source: "a/b\nc"}}, "control"},
		{"ref option", []InstalledMarketplaceConfig{{Name: "m", Source: "a/b", Ref: "--evil"}}, "ref"},
		{"path escape", []InstalledMarketplaceConfig{{Name: "m", Source: "a/b", Path: "../x"}}, "path"},
		{"absolute path", []InstalledMarketplaceConfig{{Name: "m", Source: "a/b", Path: "/etc"}}, "path"},
		{"invalid trust", []InstalledMarketplaceConfig{{Name: "m", Source: "a/b", Trust: "loud"}}, "trust"},
		{"ref on a local directory", []InstalledMarketplaceConfig{{Name: "m", Source: "./d", Ref: "main"}}, "git source"},
		{"version on an index url", []InstalledMarketplaceConfig{{Name: "m", Source: "https://e.com/m.json", Version: "^1"}}, "git source"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateInstalledMarketplaces(tt.in)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestValidateInstalledPlugins(t *testing.T) {
	mps := []InstalledMarketplaceConfig{{Name: "acme", Source: "acme/plugins"}}
	off := false
	tests := []struct {
		name    string
		in      []InstalledPluginConfig
		wantErr string
	}{
		{"from a marketplace", []InstalledPluginConfig{{Name: "p", Marketplace: "acme"}}, ""},
		{"marketplace entry under another name", []InstalledPluginConfig{{Name: "p", Marketplace: "acme", Plugin: "tools"}}, ""},
		{"from a repository", []InstalledPluginConfig{{Name: "p", Source: "acme/tools", Ref: "v1", Path: "plugins/p"}}, ""},
		{"from a local directory", []InstalledPluginConfig{{Name: "p", Source: "./vendor/p"}}, ""},
		{"all options", []InstalledPluginConfig{{Name: "p", Marketplace: "acme", Include: []string{"skills", "mcp"}, Exclude: []string{"hooks"},
			NamePrefix: "acme-", Domain: "d", Profiles: []string{"x"}, Trust: TrustWarn, EnableHooks: true, EnableMCP: &off, Format: "claude"}}, ""},
		{"version range on a repository", []InstalledPluginConfig{{Name: "p", Source: "acme/tools", Version: "^2"}}, ""},
		{"missing name", []InstalledPluginConfig{{Marketplace: "acme"}}, "name"},
		{"duplicate", []InstalledPluginConfig{{Name: "p", Marketplace: "acme"}, {Name: "p", Marketplace: "acme"}}, "duplicate"},
		{"neither marketplace nor source", []InstalledPluginConfig{{Name: "p"}}, "required"},
		{"both marketplace and source", []InstalledPluginConfig{{Name: "p", Marketplace: "acme", Source: "a/b"}}, "cannot be combined"},
		{"unknown marketplace", []InstalledPluginConfig{{Name: "p", Marketplace: "nope"}}, "not an installed_marketplaces"},
		{"ref on a marketplace plugin", []InstalledPluginConfig{{Name: "p", Marketplace: "acme", Ref: "main"}}, "marketplace's own"},
		{"version on a marketplace plugin", []InstalledPluginConfig{{Name: "p", Marketplace: "acme", Version: "^1"}}, "marketplace's own"},
		{"plugin entry without marketplace", []InstalledPluginConfig{{Name: "p", Source: "a/b", Plugin: "x"}}, "needs marketplace"},
		{"index url as a plugin source", []InstalledPluginConfig{{Name: "p", Source: "https://e.com/m.json"}}, "marketplace.json"},
		{"http source", []InstalledPluginConfig{{Name: "p", Source: "http://e.com/x.git"}}, "http://"},
		{"credentials", []InstalledPluginConfig{{Name: "p", Source: "https://u:p@e.com/x.git"}}, "credentials"},
		{"option ref", []InstalledPluginConfig{{Name: "p", Source: "a/b", Ref: "-x"}}, "ref"},
		{"path escape", []InstalledPluginConfig{{Name: "p", Source: "a/b", Path: "a/../../b"}}, "path"},
		{"ref on a local plugin", []InstalledPluginConfig{{Name: "p", Source: "./d", Ref: "main"}}, "git source"},
		{"unknown kind", []InstalledPluginConfig{{Name: "p", Marketplace: "acme", Include: []string{"themes"}}}, "unknown component kind"},
		{"unknown excluded kind", []InstalledPluginConfig{{Name: "p", Marketplace: "acme", Exclude: []string{"x"}}}, "unknown component kind"},
		{"unknown format", []InstalledPluginConfig{{Name: "p", Marketplace: "acme", Format: "vscode"}}, "format"},
		{"bad prefix", []InstalledPluginConfig{{Name: "p", Marketplace: "acme", NamePrefix: "../x"}}, "name_prefix"},
		{"invalid trust", []InstalledPluginConfig{{Name: "p", Marketplace: "acme", Trust: "x"}}, "trust"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateInstalledPlugins(tt.in, mps)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestInstalledPlugin_Helpers(t *testing.T) {
	off := false
	p := InstalledPluginConfig{Name: "p", Include: []string{"skills", "hooks"}, Exclude: []string{"hooks"}}
	assert.True(t, p.Wants(PluginKindSkills))
	assert.False(t, p.Wants(PluginKindHooks), "exclude wins over include")
	assert.False(t, p.Wants(PluginKindAgents), "include narrows")
	assert.True(t, (&InstalledPluginConfig{}).Wants(PluginKindMCP))
	assert.True(t, p.MCPEnabled())
	p.EnableMCP = &off
	assert.False(t, p.MCPEnabled())
	assert.Equal(t, "p", p.EntryName())
	p.Plugin = "other"
	assert.Equal(t, "other", p.EntryName())
	assert.Equal(t, "^1.2", (&InstalledPluginConfig{Version: "^1.2"}).RequestedRef())
	assert.Equal(t, "v1", (&InstalledMarketplaceConfig{Ref: "v1"}).RequestedRef())
}

func TestPluginTrust_InheritsTheMarketplace(t *testing.T) {
	cfg := &Config{InstalledMarketplaces: []InstalledMarketplaceConfig{{Name: "m", Source: "a/b", Trust: TrustWarn}}}
	assert.Equal(t, TrustWarn, cfg.PluginTrust(&InstalledPluginConfig{Name: "p", Marketplace: "m"}))
	assert.Equal(t, TrustError, cfg.PluginTrust(&InstalledPluginConfig{Name: "p", Marketplace: "m", Trust: TrustError}))
	assert.Empty(t, cfg.PluginTrust(&InstalledPluginConfig{Name: "p", Source: "a/b"}))
}

func TestInstalledPlugins_TOMLRoundTrip(t *testing.T) {
	// Arrange
	src := `version = "5.0"
name = "t"
presets = ["claude"]

[[installed_marketplaces]]
name = "acme"
source = "acme/plugins"
version = "^1.2"
tag_prefix = "v"
trust = "warn"

[[installed_plugins]]
name = "formatter"
marketplace = "acme"
plugin = "fmt"
include = ["skills", "mcp"]
name_prefix = "acme-"
enable_hooks = true
enable_mcp = false
`

	// Act
	cfg, err := DecodeTOMLConfig([]byte(src), "config.toml")
	require.NoError(t, err)
	out, err := MarshalTOML(cfg)
	require.NoError(t, err)
	again, err := DecodeTOMLConfig(out, "config.toml")
	require.NoError(t, err)

	// Assert
	require.Len(t, cfg.InstalledMarketplaces, 1)
	require.Len(t, cfg.InstalledPlugins, 1)
	assert.Equal(t, cfg.InstalledMarketplaces, again.InstalledMarketplaces)
	assert.Equal(t, cfg.InstalledPlugins, again.InstalledPlugins)
	assert.True(t, cfg.InstalledPlugins[0].EnableHooks)
	require.NotNil(t, cfg.InstalledPlugins[0].EnableMCP)
	assert.False(t, *cfg.InstalledPlugins[0].EnableMCP)
	assert.Contains(t, string(out), "[[installed_plugins]]")
}

func TestInstalledPlugins_VersionKeysAreValidated(t *testing.T) {
	tests := []struct {
		name string
		toml string
		want string
	}{
		{"ref and version", "[[installed_marketplaces]]\nname=\"m\"\nsource=\"a/b\"\nref=\"main\"\nversion=\"^1\"\n", "AR731"},
		{"prefix without version", "[[installed_plugins]]\nname=\"p\"\nsource=\"a/b\"\ntag_prefix=\"v\"\n", "AR731"},
		{"bad constraint", "[[installed_plugins]]\nname=\"p\"\nsource=\"a/b\"\nversion=\"not a range\"\n", "AR731"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := DecodeTOMLConfig([]byte("version = \"5.0\"\nname = \"t\"\npresets = [\"claude\"]\n"+tt.toml), "config.toml")
			require.NoError(t, err)
			err = cfg.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestResolveInstalledPlugins_MergesAndScopes(t *testing.T) {
	// Arrange
	cfg := &Config{
		InstalledPlugins: []InstalledPluginConfig{{Name: "p", Source: "./p"}},
		Content: &ContentTree{
			Skills:  []ContentFile{{Name: "local-skill"}},
			Domains: map[string]*Domain{"backend": {Name: "backend", Agents: []ContentFile{{Name: "dom-agent"}}}},
		},
		MCPServers: map[string]*MCPServer{"existing": {Name: "existing", Command: "x"}},
	}
	res := &PluginResolution{
		Skills:   []ContentFile{{Name: "local-skill"}, {Name: "plug-skill", Profiles: []string{"dev"}}, {Name: "plug-skill"}},
		Agents:   []ContentFile{{Name: "dom-agent"}, {Name: "scoped", Domain: "backend"}, {Name: "orphan", Domain: "nope"}},
		Commands: []ContentFile{{Name: "cmd"}},
		Rules:    []ContentFile{{Name: "rule"}},
		Hooks:    []HookGroup{{Event: "Stop", Hooks: []HookAction{{Command: "x"}}}},
		MCP:      []MCPServer{{Name: "existing"}, {Name: "new", Command: "y"}},
		Scan:     []ImportedScanFile{{Path: "/p/hooks.json"}},
	}

	// Act
	err := resolveInstalledPluginsIfNeeded(context.Background(), cfg, func(context.Context, *Config) (*PluginResolution, error) { return res, nil })

	// Assert
	require.NoError(t, err)
	var skills []string
	for _, s := range cfg.Content.Skills {
		skills = append(skills, s.Name)
	}
	assert.Equal(t, []string{"local-skill", "plug-skill"}, skills, "local wins and the first plugin skill wins")
	assert.Equal(t, []string{"dom-agent", "scoped"}, names(cfg.Content.Domains["backend"].Agents))
	assert.Equal(t, []string{"orphan"}, names(cfg.Content.Agents), "an unknown domain falls back to the root")
	assert.Equal(t, []string{"cmd"}, names(cfg.Content.Commands))
	assert.Equal(t, []string{"rule"}, names(cfg.Content.Rules))
	require.Len(t, cfg.Hooks, 1)
	assert.Equal(t, HookBuiltinPlugin, cfg.Hooks[0].Builtin)
	assert.Equal(t, "x", cfg.MCPServers["existing"].Command, "an existing server is not replaced")
	assert.Contains(t, cfg.MCPServers, "new")
	assert.Equal(t, []string{"new"}, cfg.PluginMCPServers)
	assert.Len(t, cfg.ImportedScan, 1)
}

func TestResolveInstalledPlugins_Errors(t *testing.T) {
	cfg := func() *Config { return &Config{InstalledPlugins: []InstalledPluginConfig{{Name: "p", Source: "./p"}}} }
	t.Run("no resolver", func(t *testing.T) {
		err := resolveInstalledPluginsIfNeeded(context.Background(), cfg(), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "resolver")
	})
	t.Run("a lock violation is fatal", func(t *testing.T) {
		err := resolveInstalledPluginsIfNeeded(context.Background(), cfg(), func(context.Context, *Config) (*PluginResolution, error) {
			return nil, ErrLockViolation
		})
		require.ErrorIs(t, err, ErrLockViolation)
	})
	t.Run("an unresolved plugin is fatal", func(t *testing.T) {
		err := resolveInstalledPluginsIfNeeded(context.Background(), cfg(), func(context.Context, *Config) (*PluginResolution, error) {
			return nil, ErrPluginUnresolved
		})
		require.ErrorIs(t, err, ErrPluginUnresolved)
	})
	t.Run("another failure is tolerated", func(t *testing.T) {
		err := resolveInstalledPluginsIfNeeded(context.Background(), cfg(), func(context.Context, *Config) (*PluginResolution, error) {
			return nil, assert.AnError
		})
		require.NoError(t, err)
	})
	t.Run("nothing configured", func(t *testing.T) {
		require.NoError(t, resolveInstalledPluginsIfNeeded(context.Background(), &Config{}, nil))
	})
}

func TestSelectContent_ScopesPluginAgentsCommandsAndRules(t *testing.T) {
	// Arrange
	tree := &ContentTree{
		Agents:   []ContentFile{{Name: "all"}, {Name: "dev-only", Profiles: []string{"dev"}}},
		Commands: []ContentFile{{Name: "c-dev", Profiles: []string{"dev"}}},
		Rules:    []ContentFile{{Name: "r-prod", Profiles: []string{"prod"}}},
		Domains:  map[string]*Domain{"d": {Name: "d", FromInclude: true, Agents: []ContentFile{{Name: "d-dev", Profiles: []string{"dev"}}, {Name: "d-prod", Profiles: []string{"prod"}}}}},
	}
	c := &Config{}

	// Act
	got, err := c.selectContent(tree, nil, "prod")

	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{"all"}, names(got.Agents))
	assert.Empty(t, got.Commands)
	assert.Equal(t, []string{"r-prod"}, names(got.Rules))
	assert.Equal(t, []string{"d-prod"}, names(got.Domains["d"].Agents))
	assert.True(t, strings.Contains(strings.Join(names(tree.Domains["d"].Agents), ","), "d-dev"), "the source tree is not modified")
}

func names(files []ContentFile) []string {
	out := []string{}
	for _, f := range files {
		out = append(out, f.Name)
	}
	return out
}

func TestResolveInstalledPlugins_ConflictWarningsAreSaidOncePerRun(t *testing.T) {
	// Arrange: the same project loaded twice, as a command does (lock, then generate)
	dir := t.TempDir()
	log := &recordingLogger{}
	ctx := logger.WithContext(context.Background(), log)
	res := &PluginResolution{
		Skills: []ContentFile{{Name: "dup"}},
		MCP:    []MCPServer{{Name: "existing"}},
	}
	load := func() {
		cfg := &Config{
			BaseDir:          dir,
			InstalledPlugins: []InstalledPluginConfig{{Name: "p", Source: "./p"}},
			Content:          &ContentTree{Skills: []ContentFile{{Name: "dup"}}},
			MCPServers:       map[string]*MCPServer{"existing": {Name: "existing", Command: "x"}},
		}
		require.NoError(t, resolveInstalledPluginsIfNeeded(ctx, cfg, func(context.Context, *Config) (*PluginResolution, error) { return res, nil }))
	}

	// Act
	load()
	load()

	// Assert
	assert.Len(t, log.warns, 2, "one warning per conflict, not one per load: %v", log.warns)
}
