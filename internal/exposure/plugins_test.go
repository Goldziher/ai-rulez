package exposure

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/v5/internal/testutil"
)

const marketplaceJSON = `{"name": "mkt", "plugins": [
  {"name": "one", "source": "./a/plugins/one"},
  {"name": "gone", "source": "./missing"},
  {"name": "bad", "source": "../escape"},
  {"name": "remote", "source": {"source": "github", "repo": "example/tools"}},
  {"name": "two", "source": "./b"}
]}`

func marketplaceRepo(t *testing.T, settings string) string {
	t.Helper()
	files := map[string]string{
		MarketplaceFile:                        marketplaceJSON,
		"a/plugins/one/skills/probe/SKILL.md":  skillMD("probe", "Probes things.", "when_to_use: When probing.\n"),
		"a/plugins/one/skills/quiet/SKILL.md":  skillMD("quiet", "User only.", "disable-model-invocation: true\n"),
		"a/plugins/one/skills/scoped/SKILL.md": "---\ndescription: Gated.\npaths: [src/**]\n---\n",
		"a/plugins/one/skills/deep/x/SKILL.md": skillMD("deep", "Too deep to be a skill."),
		"a/plugins/one/README.md":              "not a skill",
		"b/skills/lint/SKILL.md":               "---\nname: lint-all\ndescription: Lints.\n---\n",
	}
	if settings != "" {
		files[".claude/settings.json"] = settings
	}
	return writeRepo(t, files)
}

func pluginSkills(inv *Inventory) []string {
	var out []string
	for _, a := range inv.Artifacts {
		if a.Source == SourcePlugin {
			out = append(out, a.QualifiedName)
		}
	}
	return out
}

func TestBuild_PluginModes(t *testing.T) {
	tests := []struct {
		name        string
		settings    string
		mode        PluginMode
		requested   []string
		wantEnabled []string
		wantLoaded  []string
		wantSkills  []string
	}{
		{name: "enabled is the default and loads nothing unless enabled", wantEnabled: []string{}, wantLoaded: []string{}},
		{name: "enabled loads what the settings enable", settings: `{"enabledPlugins": {"one@mkt": true}}`, wantEnabled: []string{"one"}, wantLoaded: []string{"one"}, wantSkills: []string{"one:probe", "one:quiet", "one:scoped"}},
		{name: "enabled adds the requested plugins", mode: PluginsEnabled, requested: []string{"two", "ghost"}, wantEnabled: []string{}, wantLoaded: []string{"two"}, wantSkills: []string{"two:lint-all"}},
		{name: "none loads nothing", settings: `{"enabledPlugins": {"one@mkt": true}}`, mode: PluginsNone, requested: []string{"two"}, wantEnabled: []string{"one"}, wantLoaded: []string{}},
		{name: "all loads every available plugin", mode: PluginsAll, wantEnabled: []string{}, wantLoaded: []string{"one", "two"}, wantSkills: []string{"one:probe", "one:quiet", "one:scoped", "two:lint-all"}},
		{name: "a key must name the repository marketplace", settings: `{"enabledPlugins": {"one@other": true, "two": true, "ghost@mkt": true}}`, wantEnabled: []string{}, wantLoaded: []string{}},
		{name: "a disabled plugin is not loaded", settings: `{"enabledPlugins": {"one@mkt": false}}`, wantEnabled: []string{}, wantLoaded: []string{}},
		{name: "a listed but unavailable plugin is never loaded", settings: `{"enabledPlugins": {"gone@mkt": true, "remote@mkt": true}}`, mode: PluginsAll, wantEnabled: []string{}, wantLoaded: []string{"one", "two"}, wantSkills: []string{"one:probe", "one:quiet", "one:scoped", "two:lint-all"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := marketplaceRepo(t, tt.settings)

			// Act
			inv := build(t, root, func(o *Options) { o.Plugins, o.Requested = tt.mode, tt.requested })

			// Assert
			wantMode := tt.mode
			if wantMode == "" {
				wantMode = PluginsEnabled
			}
			assert.Equal(t, wantMode, inv.Plugins.Mode)
			assert.Equal(t, "mkt", inv.Plugins.Marketplace)
			assert.Equal(t, []PluginRef{{Name: "one", Path: "a/plugins/one"}, {Name: "two", Path: "b"}}, inv.Plugins.Available)
			assert.Equal(t, tt.wantEnabled, inv.Plugins.Enabled)
			assert.Equal(t, tt.wantLoaded, inv.Plugins.Loaded)
			assert.Equal(t, tt.wantSkills, pluginSkills(inv))
		})
	}
}

func TestBuild_PluginSkillsAreQualified(t *testing.T) {
	root := marketplaceRepo(t, `{"enabledPlugins": {"one@mkt": true, "two@mkt": true}}`)

	inv := build(t, root)

	probe := named(t, inv, KindSkill, "one:probe")
	assert.Equal(t, "probe", probe.Name)
	assert.Equal(t, "one", probe.Plugin)
	assert.Equal(t, "a/plugins/one", probe.PluginPath)
	assert.Equal(t, SourcePlugin, probe.Source)
	assert.Equal(t, "a/plugins/one/skills/probe/SKILL.md", probe.Path)
	assert.Equal(t, RootScope, probe.Scope)
	assert.Equal(t, "one:probe: Probes things. When probing.", probe.ListingText)
	assert.Equal(t, probe.ListingText, probe.Text)
	assert.Equal(t, PhaseStart, probe.Phase)
	assert.Equal(t, PhaseManual, named(t, inv, KindSkill, "one:quiet").Phase)
	assert.Equal(t, PhaseOnDemand, named(t, inv, KindSkill, "one:scoped").Phase)
	lint := named(t, inv, KindSkill, "two:lint-all")
	assert.Equal(t, "lint-all", lint.Name, "the frontmatter name wins over the directory")
	assert.Equal(t, "b", lint.PluginPath)
	assert.Equal(t, "two:lint-all: Lints.", lint.Text)
}

func TestBuild_SkillOverridesDoNotApplyToPluginSkills(t *testing.T) {
	root := marketplaceRepo(t, `{"enabledPlugins": {"one@mkt": true}, "skillOverrides": {"probe": "off", "one:probe": "off"}}`)

	inv := build(t, root)

	assert.Contains(t, pluginSkills(inv), "one:probe")
}

func TestBuild_LocalSettingsDisableAPlugin(t *testing.T) {
	root := marketplaceRepo(t, `{"enabledPlugins": {"one@mkt": true, "two@mkt": true}}`)
	writeFile(t, root, ".claude/settings.local.json", `{"enabledPlugins": {"two@mkt": false}}`)

	inv := build(t, root)

	assert.Equal(t, []string{"one"}, inv.Plugins.Loaded)
}

func TestBuild_Marketplace(t *testing.T) {
	tests := []struct {
		name, content string
		wantProblem   bool
	}{
		{name: "no marketplace"},
		{name: "malformed JSON", content: "not json", wantProblem: true},
		{name: "not an object", content: "[]", wantProblem: true},
		{name: "a bad entry is left out", content: `{"name": "mkt", "plugins": [1]}`},
		{name: "no plugins", content: `{"name": "mkt"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string]string{"p/skills/s/SKILL.md": skillMD("s", "d")}
			if tt.content != "" {
				files[MarketplaceFile] = tt.content
			}
			root := writeRepo(t, files)

			inv := build(t, root, func(o *Options) { o.Plugins = PluginsAll })

			assert.Empty(t, inv.Plugins.Available)
			assert.Empty(t, pluginSkills(inv))
			assert.Equal(t, tt.wantProblem, len(inv.Problems) == 1, "%v", inv.Problems)
		})
	}
}

func TestBuild_PluginSourceThroughASymlinkIsNotAvailable(t *testing.T) {
	outside := writeRepo(t, map[string]string{"skills/s/SKILL.md": skillMD("s", "d")})
	root := writeRepo(t, map[string]string{MarketplaceFile: `{"name": "mkt", "plugins": [{"name": "esc", "source": "./link"}]}`})
	testutil.SymlinkOrSkip(t, outside, filepath.Join(root, "link"))

	inv := build(t, root, func(o *Options) { o.Plugins = PluginsAll })

	assert.Empty(t, inv.Plugins.Available)
}

func TestBuild_PluginRootPrefixesBareSources(t *testing.T) {
	root := writeRepo(t, map[string]string{
		MarketplaceFile:                 `{"name": "mkt", "metadata": {"pluginRoot": "./plugins"}, "plugins": [{"name": "kit", "source": "kit"}]}`,
		"plugins/kit/skills/s/SKILL.md": skillMD("s", "d"),
	})

	inv := build(t, root, func(o *Options) { o.Plugins = PluginsAll })

	assert.Equal(t, []string{"kit:s"}, pluginSkills(inv))
}

func TestPluginMode_Valid(t *testing.T) {
	for _, m := range PluginModes() {
		assert.True(t, m.Valid(), m)
	}
	assert.False(t, PluginMode("some").Valid())
}

func TestAreaScope(t *testing.T) {
	tests := []struct {
		name string
		a    Artifact
		want string
	}{
		{name: "the root stays the root scope", a: Artifact{Scope: RootScope}, want: RootScope},
		{name: "a nested scope is kept", a: Artifact{Scope: "svc/api"}, want: "svc/api"},
		{name: "a plugin skill is owned by its plugin directory", a: Artifact{Source: SourcePlugin, Scope: RootScope, PluginPath: "a/plugins/one"}, want: "a/plugins/one"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, AreaScope(&tt.a))
		})
	}
}

func TestBuild_AnUnnamedMarketplaceEnablesNothing(t *testing.T) {
	tests := []struct {
		name     string
		settings string
	}{
		{name: "a key with an empty marketplace", settings: `{"enabledPlugins": {"one@": true}}`},
		{name: "a key with a marketplace", settings: `{"enabledPlugins": {"one@mkt": true}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := writeRepo(t, map[string]string{
				MarketplaceFile:           `{"plugins": [{"name": "one", "source": "./a"}]}`,
				"a/skills/probe/SKILL.md": skillMD("probe", "Probes things."),
				".claude/settings.json":   tt.settings,
			})

			// Act
			inv := build(t, root, func(*Options) {})

			// Assert
			assert.Empty(t, inv.Plugins.Enabled)
			assert.Empty(t, pluginSkills(inv))
		})
	}
}

func TestBuild_SettingsThroughASymlinkedClaudeDirAreNotRead(t *testing.T) {
	// Arrange
	outside := writeRepo(t, map[string]string{"settings.json": `{"enabledPlugins": {"one@mkt": true}}`})
	root := marketplaceRepo(t, "")
	testutil.SymlinkOrSkip(t, outside, filepath.Join(root, ".claude"))

	// Act
	inv := build(t, root)

	// Assert
	assert.Empty(t, inv.Plugins.Enabled)
	assert.Empty(t, pluginSkills(inv))
	var refused []string
	for _, p := range inv.Problems {
		refused = append(refused, p.Path)
	}
	assert.Contains(t, refused, ".claude/settings.json")
}
