package plugin

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cf(name string) config.ContentFile {
	return config.ContentFile{Name: name, Path: "/p/" + name + "/SKILL.md"}
}

func domainTree() *config.ContentTree {
	return &config.ContentTree{
		Skills: []config.ContentFile{cf("root-a"), cf("shared")},
		Domains: map[string]*config.Domain{
			"Backend": {Name: "Backend", Skills: []config.ContentFile{cf("py"), cf("shared")}, Commands: []config.ContentFile{cf("run")}},
			"ui":      {Name: "ui", Skills: []config.ContentFile{cf("react")}, Agents: []config.ContentFile{cf("critic")}},
			"empty":   {Name: "empty"},
			"builtin": {Name: "builtin", Builtin: true, Skills: []config.ContentFile{cf("b")}},
			"virtual": {Name: "virtual", Skills: []config.ContentFile{{Name: "v", Path: "builtin://virtual/v"}}},
		},
	}
}

func names(files []config.ContentFile) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Name)
	}
	return out
}

func TestPlanDomainPlugins(t *testing.T) {
	no := false
	tests := []struct {
		name      string
		mkt       *config.MarketplaceAuthoring
		rootPlug  *config.PluginAuthoring
		wantNames []string
		check     func(t *testing.T, plan []PlannedPlugin)
		wantErr   string
	}{
		{
			name:      "one per non-empty, non-builtin domain with lower-cased names",
			mkt:       &config.MarketplaceAuthoring{Name: "mk", FromDomains: &config.DomainPluginsConfig{NamePrefix: "x-"}},
			wantNames: []string{"x-backend", "x-ui"},
			check: func(t *testing.T, plan []PlannedPlugin) {
				assert.Equal(t, []string{"Backend"}, plan[0].Domains)
				assert.Equal(t, []string{"py", "shared"}, names(plan[0].Skills))
				assert.Equal(t, []string{"run"}, names(plan[0].Commands))
				assert.Equal(t, []string{"critic"}, names(plan[1].Agents))
				assert.Equal(t, "1.0.0", plan[0].Version)
				assert.Equal(t, "./plugins/x-backend", plan[0].Source())
			},
		},
		{
			name:      "include and exclude accept globs",
			mkt:       &config.MarketplaceAuthoring{Name: "mk", FromDomains: &config.DomainPluginsConfig{Include: []string{"*"}, Exclude: []string{"back*"}}},
			wantNames: []string{"ui"},
		},
		{
			name:      "disabled block generates nothing",
			mkt:       &config.MarketplaceAuthoring{Name: "mk", FromDomains: &config.DomainPluginsConfig{Enabled: &no}},
			wantNames: nil,
		},
		{
			name: "defaults cascade entry, from_domains, [plugin]",
			mkt: &config.MarketplaceAuthoring{Name: "mk", FromDomains: &config.DomainPluginsConfig{
				PluginDefaults: config.PluginDefaults{Category: "dev", DefaultEnabled: &no},
			}},
			rootPlug:  &config.PluginAuthoring{Name: "r", Version: "9.9.9", Keywords: []string{"k"}, Runtimes: []string{"claude"}},
			wantNames: []string{"backend", "ui"},
			check: func(t *testing.T, plan []PlannedPlugin) {
				assert.Equal(t, "9.9.9", plan[0].Version)
				assert.Equal(t, "dev", plan[0].Category)
				assert.Equal(t, []string{"k"}, plan[0].Keywords)
				assert.Equal(t, []string{"claude"}, plan[0].Runtimes)
				require.NotNil(t, plan[0].DefaultEnabled)
				assert.False(t, *plan[0].DefaultEnabled)
			},
		},
		{
			name: "declared plugin mixes domains and root content, root wins on collisions",
			mkt: &config.MarketplaceAuthoring{Name: "mk", Plugins: []config.MarketplacePlugin{
				{Name: "mix", Domains: []string{"Backend", "ui"}, Skills: []string{"root-*", "shared"}},
			}},
			wantNames: []string{"mix"},
			check: func(t *testing.T, plan []PlannedPlugin) {
				assert.Equal(t, []string{"py", "shared", "react", "root-a"}, names(plan[0].Skills))
			},
		},
		{
			name: "declared plugin replaces derived of the same name",
			mkt: &config.MarketplaceAuthoring{Name: "mk",
				FromDomains: &config.DomainPluginsConfig{},
				Plugins:     []config.MarketplacePlugin{{Name: "ui", Skills: []string{"root-a"}, Description: "mine"}}},
			wantNames: []string{"backend", "ui"},
			check: func(t *testing.T, plan []PlannedPlugin) {
				assert.Equal(t, "mine", plan[1].Description)
				assert.Equal(t, []string{"root-a"}, names(plan[1].Skills))
			},
		},
		{
			name:      "declared plugin with no matching content is skipped",
			mkt:       &config.MarketplaceAuthoring{Name: "mk", Plugins: []config.MarketplacePlugin{{Name: "none", Domains: []string{"ghost"}}}},
			wantNames: nil,
		},
		{
			name:    "domain name that cannot be a plugin name is an error",
			mkt:     &config.MarketplaceAuthoring{Name: "mk", FromDomains: &config.DomainPluginsConfig{Include: []string{"*"}}},
			wantErr: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cfg := &config.Config{Marketplace: tt.mkt, Plugin: tt.rootPlug}
			tree := domainTree()
			if tt.name == "domain name that cannot be a plugin name is an error" {
				tree.Domains["bad name"] = &config.Domain{Name: "bad name", Skills: []config.ContentFile{cf("z")}}
				tt.wantErr = "valid plugin name"
			}

			// Act
			plan, err := PlanDomainPlugins(cfg, tree)

			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			var got []string
			for _, p := range plan {
				got = append(got, p.Name)
			}
			assert.Equal(t, tt.wantNames, got)
			if tt.check != nil {
				tt.check(t, plan)
			}
		})
	}
}

func TestIncludeDomainContent(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		want     []string
	}{
		{name: "no patterns is root only", want: []string{"root-a", "shared"}},
		{name: "named domain, root wins collisions", patterns: []string{"Backend"}, want: []string{"root-a", "shared", "py"}},
		{name: "glob takes domains in name order and skips builtin and virtual content", patterns: []string{"*"}, want: []string{"root-a", "shared", "py", "react"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			skills, _, _ := includeDomainContent(domainTree(), tt.patterns)

			// Assert
			assert.Equal(t, tt.want, names(skills))
		})
	}
}

func TestRenderMonorepoMarketplace_DetailedEntries(t *testing.T) {
	no := false
	entries := []MemberEntry{
		{Name: "minimal", Source: "./m", Category: "x", Version: "1"},
		{
			Name: "rich", Source: "./plugins/rich", Detailed: true, Category: "dev", Version: "2.0.0",
			Keywords: []string{"k"}, DefaultEnabled: &no,
			Relevance: &config.PluginRelevance{Topic: "T", Signals: config.RelevanceSignals{
				FilesRead: []string{"**/*.tf"}, ManifestDeps: []config.ManifestDepMatch{{File: "package\\.json$", Pattern: "sdk"}},
			}},
		},
	}

	// Act
	out, err := RenderMonorepoMarketplace(MarketInfo{Name: "mk"}, entries, "/base")

	// Assert
	require.NoError(t, err)
	var doc struct {
		Plugins []map[string]any `json:"plugins"`
	}
	require.NoError(t, json.Unmarshal(out.RawContent, &doc))
	assert.Equal(t, map[string]any{"name": "minimal", "source": "./m"}, doc.Plugins[0], "members keep the minimal entry")
	rich := doc.Plugins[1]
	assert.Equal(t, "dev", rich["category"])
	assert.Equal(t, "2.0.0", rich["version"])
	assert.Equal(t, false, rich["defaultEnabled"])
	assert.Equal(t, map[string]any{
		"topic": "T",
		"signals": map[string]any{
			"filesRead":    []any{"**/*.tf"},
			"manifestDeps": []any{map[string]any{"file": "package\\.json$", "pattern": "sdk"}},
		},
	}, rich["relevance"])
}

func TestCatalogSkill(t *testing.T) {
	no := false
	cfg := &config.Config{Marketplace: &config.MarketplaceAuthoring{
		Name: "mk", OutputDir: "tools/mkt", CatalogSkill: &config.CatalogSkillConfig{Enabled: true},
	}}
	plan := []PlannedPlugin{
		{Name: "a", Description: "pipes | here", Skills: []config.ContentFile{cf("s2"), cf("s1")}},
		{Name: "b", Description: "off", DefaultEnabled: &no},
	}

	// Act
	skill := CatalogSkill(cfg, plan)

	// Assert
	assert.Equal(t, "plugin-catalog", skill.Name)
	assert.Equal(t, "generated://plugin-catalog/SKILL.md", skill.Path)
	assert.NotEmpty(t, skill.Metadata.Extra["description"])
	assert.Contains(t, skill.Content, "| `a` | on | pipes / here | `s1`, `s2` |")
	assert.Contains(t, skill.Content, "| `b` | off | off | - |")
	assert.Contains(t, skill.Content, "claude plugin marketplace add ./tools/mkt")
	assert.Contains(t, skill.Content, "claude plugin install <plugin>@mk")
	assert.False(t, strings.Contains(skill.Content, "\r"))
}

func TestBundledNames(t *testing.T) {
	tests := []struct {
		name       string
		cfg        *config.Config
		plan       []PlannedPlugin
		wantSkills []string
	}{
		{
			name:       "root [plugin] bundle covers root skills only by default",
			cfg:        &config.Config{Plugin: &config.PluginAuthoring{Name: "r"}},
			wantSkills: []string{"root-a", "shared"},
		},
		{
			name:       "include_domains extends the root bundle",
			cfg:        &config.Config{Plugin: &config.PluginAuthoring{Name: "r", IncludeDomains: []string{"ui"}}},
			wantSkills: []string{"root-a", "shared", "react"},
		},
		{
			name: "domain plugins replace the root bundle",
			cfg: &config.Config{
				Plugin:      &config.PluginAuthoring{Name: "r"},
				Marketplace: &config.MarketplaceAuthoring{Name: "mk", FromDomains: &config.DomainPluginsConfig{}},
			},
			plan:       []PlannedPlugin{{Name: "p", Skills: []config.ContentFile{cf("py")}, Commands: []config.ContentFile{cf("run")}}},
			wantSkills: []string{"py"},
		},
		{name: "nothing configured bundles nothing", cfg: &config.Config{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			skills, _ := BundledNames(tt.cfg, domainTree(), tt.plan)

			// Assert
			var got []string
			for name := range skills {
				got = append(got, name)
			}
			assert.ElementsMatch(t, tt.wantSkills, got)
		})
	}
}
