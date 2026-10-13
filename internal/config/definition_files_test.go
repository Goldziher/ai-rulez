package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const definitionMain = `version = "5.0"
name = "proj"
presets = ["claude"]
`

// definitionProject writes files relative to .ai-rulez/ (subdirectories allowed).
func definitionProject(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, ".ai-rulez")
	for name, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}
	return root
}

func TestDefinitionFiles_LoadByDefault(t *testing.T) {
	tests := []struct {
		name         string
		files        map[string]string
		wantPlugins  []string
		wantMarkets  []string
		wantSkills   map[string][]string
		wantErrParts []string
	}{
		{
			name: "plugin and marketplace files are loaded, name defaults to the file name",
			files: map[string]string{
				"config.toml":                definitionMain,
				"plugins/notify.toml":        "description = \"d\"\nskills = [\"notify-*\"]\n",
				"plugins/moon.toml":          "name = \"moon\"\nskills = [\"moon-*\"]\n",
				"marketplaces/public.toml":   "plugins = [\"notify\", \"moon\"]\noutput_dir = \"marketplaces/public\"\n",
				"marketplaces/internal.toml": "name = \"internal\"\nplugins = [\"moon\"]\n",
			},
			wantPlugins: []string{"moon", "notify"},
			wantMarkets: []string{"internal", "public"},
			wantSkills:  map[string][]string{"notify": {"notify-*"}, "moon": {"moon-*"}},
		},
		{
			name: "a name that differs from the file name is an error naming the file",
			files: map[string]string{
				"config.toml":         definitionMain,
				"plugins/notify.toml": "name = \"other\"\nskills = [\"a\"]\n",
			},
			wantErrParts: []string{"plugins/notify.toml", "file name must equal the name"},
		},
		{
			name: "an unknown key is an error naming the file and the key",
			files: map[string]string{
				"config.toml":              definitionMain,
				"marketplaces/public.toml": "plugins = [\"a\"]\nbogus = 1\n",
			},
			wantErrParts: []string{"marketplaces/public.toml", "bogus"},
		},
		{
			name: "a plugin defined inline and in a file is an error naming both",
			files: map[string]string{
				"config.toml":         definitionMain + "\n[[plugins_authoring]]\nname = \"notify\"\nskills = [\"x\"]\n",
				"plugins/notify.toml": "skills = [\"y\"]\n",
			},
			wantErrParts: []string{`"notify"`, "config.toml", "plugins/notify.toml"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := definitionProject(t, tt.files)

			// Act
			cfg, err := LoadConfig(context.Background(), root)

			// Assert
			if len(tt.wantErrParts) > 0 {
				require.Error(t, err)
				for _, part := range tt.wantErrParts {
					assert.Contains(t, err.Error(), part)
				}
				return
			}
			require.NoError(t, err)
			var plugins, markets []string
			for _, p := range cfg.PluginsAuthoring {
				plugins = append(plugins, p.Name)
				assert.Equal(t, tt.wantSkills[p.Name], p.Skills, p.Name)
			}
			for _, m := range cfg.MarketplacesAuthoring {
				markets = append(markets, m.Name)
			}
			assert.Equal(t, tt.wantPlugins, plugins)
			assert.Equal(t, tt.wantMarkets, markets)
		})
	}
}

func TestDefinitionFiles_SurviveLayersAndCanBeRemovedByALayer(t *testing.T) {
	// Arrange
	root := definitionProject(t, map[string]string{
		"config.toml":         definitionMain + "default_layers = [\"team\"]\n",
		"plugins/notify.toml": "skills = [\"notify-*\"]\n",
		"plugins/moon.toml":   "skills = [\"moon-*\"]\n",
		"config.team.toml":    "[[plugins_authoring]]\nname = \"moon\"\nremove = true\n\n[[plugins_authoring]]\nname = \"notify\"\ndescription = \"from the layer\"\n",
	})
	// Act
	cfg, err := LoadConfig(context.Background(), root)

	// Assert
	require.NoError(t, err)
	require.Len(t, cfg.PluginsAuthoring, 1)
	assert.Equal(t, "notify", cfg.PluginsAuthoring[0].Name)
	assert.Equal(t, "from the layer", cfg.PluginsAuthoring[0].Description)
	assert.Equal(t, []string{"notify-*"}, cfg.PluginsAuthoring[0].Skills)
}

func TestMarketplaceTargets(t *testing.T) {
	tests := []struct {
		name      string
		cfg       Config
		wantNames []string
		wantAll   []bool
	}{
		{name: "nothing configured", cfg: Config{}},
		{
			name:      "the single marketplace block lists every plugin",
			cfg:       Config{Marketplace: &MarketplaceAuthoring{Name: "m"}},
			wantNames: []string{"m"}, wantAll: []bool{true},
		},
		{
			name: "marketplace definitions list plugins by name",
			cfg: Config{MarketplacesAuthoring: []MarketplaceDefinition{
				{Name: "pub", Plugins: []string{"a"}}, {Name: "int"},
			}},
			wantNames: []string{"pub", "int"}, wantAll: []bool{false, false},
		},
		{
			name:      "plugin definitions alone get one index named after the project",
			cfg:       Config{Name: "proj", PluginsAuthoring: []MarketplacePlugin{{Name: "a"}}},
			wantNames: []string{"proj"}, wantAll: []bool{true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			targets := tt.cfg.MarketplaceTargets()

			// Assert
			var names []string
			var all []bool
			for _, target := range targets {
				names = append(names, target.Name)
				all = append(all, target.Plugins == nil)
			}
			assert.Equal(t, tt.wantNames, names)
			assert.Equal(t, tt.wantAll, all)
		})
	}
}

func TestPluginDefinitions_BothSpellings(t *testing.T) {
	cfg := Config{
		Marketplace:      &MarketplaceAuthoring{Name: "m", Plugins: []MarketplacePlugin{{Name: "a"}}},
		PluginsAuthoring: []MarketplacePlugin{{Name: "b"}},
	}
	var names []string
	for _, p := range cfg.PluginDefinitions() {
		names = append(names, p.Name)
	}
	assert.Equal(t, []string{"a", "b"}, names)
	assert.True(t, cfg.HasPluginSet())
	assert.False(t, (&Config{Plugin: &PluginAuthoring{Name: "x"}}).HasPluginSet())
}
