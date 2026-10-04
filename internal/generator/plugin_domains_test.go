package generator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const domainsProjectHeader = `version = "4.0"
name = "demo"
presets = ["claude"]
gitignore = false

[plugin]
name = "root"
version = "1.2.3"
description = "Root bundle"
runtimes = ["claude"]

[plugin.author]
name = "Demo Team"
`

func writeDomainsFile(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
}

// newDomainsProject writes a project with root skills core-s and niche-s
// (targets: cursor), domain teamA (skill a-s, command do-it) and teamB (skill
// b-s, agent ag), plus the given config tail.
func newDomainsProject(t *testing.T, tail string) string {
	t.Helper()
	dir := t.TempDir()
	rulez := filepath.Join(dir, ".ai-rulez")
	writeDomainsFile(t, filepath.Join(rulez, "config.toml"), domainsProjectHeader+tail)
	writeDomainsFile(t, filepath.Join(rulez, "skills", "core-s", "SKILL.md"), "---\ndescription: core\n---\nbody\n")
	writeDomainsFile(t, filepath.Join(rulez, "skills", "niche-s", "SKILL.md"), "---\ndescription: niche\ntargets: [cursor]\n---\nbody\n")
	writeDomainsFile(t, filepath.Join(rulez, "domains", "teamA", "skills", "a-s", "SKILL.md"), "---\ndescription: a\n---\nbody\n")
	writeDomainsFile(t, filepath.Join(rulez, "domains", "teamA", "commands", "do-it.md"), "---\ndescription: cmd\n---\nrun\n")
	writeDomainsFile(t, filepath.Join(rulez, "domains", "teamB", "skills", "b-s", "SKILL.md"), "---\ndescription: b\n---\nbody\n")
	writeDomainsFile(t, filepath.Join(rulez, "domains", "teamB", "agents", "ag.md"), "---\ndescription: ag\n---\nagent\n")
	return dir
}

func loadDomainsProject(t *testing.T, dir string) *Generator {
	t.Helper()
	cfg, err := config.LoadConfig(context.Background(), dir)
	require.NoError(t, err)
	require.NoError(t, cfg.Validate())
	return NewGenerator(cfg)
}

func outputsByRel(t *testing.T, base string, outputs []config.OutputFile) map[string]string {
	t.Helper()
	byRel := make(map[string]string, len(outputs))
	for _, o := range outputs {
		if o.IsDir {
			continue
		}
		rel, err := filepath.Rel(base, o.Path)
		require.NoError(t, err)
		body := o.RawContent
		if body == nil {
			body = []byte(o.Content)
		}
		byRel[filepath.ToSlash(rel)] = string(body)
	}
	return byRel
}

func TestCollectPluginOutputs_FromDomains(t *testing.T) {
	tests := []struct {
		name        string
		tail        string
		wantPlugins []string
		wantFiles   []string
		wantAbsent  []string
	}{
		{
			name: "one plugin per domain under the output dir",
			tail: `
[marketplace]
name = "mk"
output_dir = "mkt"
[marketplace.from_domains]
name_prefix = "demo-"
`,
			wantPlugins: []string{"demo-teama", "demo-teamb"},
			wantFiles: []string{
				"mkt/.claude-plugin/marketplace.json",
				"mkt/plugins/demo-teama/skills/a-s/SKILL.md",
				"mkt/plugins/demo-teama/commands/do-it.md",
				"mkt/plugins/demo-teama/.claude-plugin/plugin.json",
				"mkt/plugins/demo-teamb/agents/ag.md",
				"mkt/plugins/demo-teamb/.ai-rulez-generated.json",
				"mkt/.ai-rulez-generated.json",
			},
			wantAbsent: []string{".agents/plugins/marketplace.json", "mkt/.agents/plugins/marketplace.json"},
		},
		{
			name: "exclude drops a domain",
			tail: `
[marketplace]
name = "mk"
[marketplace.from_domains]
exclude = ["teamB"]
`,
			wantPlugins: []string{"teama"},
			wantFiles:   []string{"plugins/teama/skills/a-s/SKILL.md"},
			wantAbsent:  []string{"plugins/teamb/skills/b-s/SKILL.md"},
		},
		{
			name: "declared plugin replaces the derived one and may mix root content",
			tail: `
[marketplace]
name = "mk"
[marketplace.from_domains]
include = ["teamA"]
[[marketplace.plugins]]
name = "teama"
domains = ["teamB"]
skills = ["core-s"]
`,
			wantPlugins: []string{"teama"},
			wantFiles:   []string{"plugins/teama/skills/b-s/SKILL.md", "plugins/teama/skills/core-s/SKILL.md"},
			wantAbsent:  []string{"plugins/teama/skills/a-s/SKILL.md"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := newDomainsProject(t, tt.tail)
			gen := loadDomainsProject(t, dir)

			// Act
			outputs, err := gen.collectPluginOutputs("")

			// Assert
			require.NoError(t, err)
			byRel := outputsByRel(t, gen.config.BaseDir, outputs)
			for _, want := range tt.wantFiles {
				assert.Contains(t, byRel, want)
			}
			for _, absent := range tt.wantAbsent {
				assert.NotContains(t, byRel, absent)
			}
			marketPath := ".claude-plugin/marketplace.json"
			if strings.Contains(tt.tail, "output_dir") {
				marketPath = "mkt/" + marketPath
			}
			var mkt struct {
				Owner   map[string]string `json:"owner"`
				Plugins []struct {
					Name    string `json:"name"`
					Source  string `json:"source"`
					Version string `json:"version"`
				} `json:"plugins"`
			}
			require.NoError(t, json.Unmarshal([]byte(byRel[marketPath]), &mkt))
			var names []string
			for _, p := range mkt.Plugins {
				names = append(names, p.Name)
				assert.Equal(t, "./plugins/"+p.Name, p.Source)
				assert.Equal(t, "1.2.3", p.Version, "version falls back to the [plugin] block")
			}
			assert.Equal(t, tt.wantPlugins, names)
			assert.Equal(t, "Demo Team", mkt.Owner["name"], "owner falls back to the [plugin] author")
		})
	}
}

func TestCollectPluginOutputs_FromDomains_Deterministic(t *testing.T) {
	// Arrange
	dir := newDomainsProject(t, "\n[marketplace]\nname = \"mk\"\n[marketplace.from_domains]\n")
	gen := loadDomainsProject(t, dir)

	// Act
	first, err := gen.collectPluginOutputs("")
	require.NoError(t, err)
	second, err := gen.collectPluginOutputs("")
	require.NoError(t, err)

	// Assert
	assert.Equal(t, outputsByRel(t, dir, first), outputsByRel(t, dir, second))
}

func TestVerifyPlugin_FromDomains(t *testing.T) {
	// Arrange
	dir := newDomainsProject(t, "\n[marketplace]\nname = \"mk\"\noutput_dir = \"mkt\"\n[marketplace.from_domains]\n")
	gen := loadDomainsProject(t, dir)
	require.NoError(t, gen.GeneratePlugin(""))

	// Act / Assert: freshly generated output verifies
	require.NoError(t, gen.VerifyPlugin(""))

	// A hand edit of a bundled file is drift.
	skill := filepath.Join(dir, "mkt", "plugins", "teama", "skills", "a-s", "SKILL.md")
	require.NoError(t, os.WriteFile(skill, []byte("edited"), 0o600))
	assert.Error(t, gen.VerifyPlugin(""))
}

func TestBuildPluginManifest_IncludeDomains(t *testing.T) {
	tests := []struct {
		name       string
		include    string
		wantSkills []string
	}{
		{name: "default stays root-only", include: "", wantSkills: []string{"core-s", "niche-s"}},
		{name: "glob adds every domain", include: `include_domains = ["*"]`, wantSkills: []string{"core-s", "niche-s", "a-s", "b-s"}},
		{name: "named domain", include: `include_domains = ["teamB"]`, wantSkills: []string{"core-s", "niche-s", "b-s"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := newDomainsProject(t, "")
			cfgPath := filepath.Join(dir, ".ai-rulez", "config.toml")
			body, err := os.ReadFile(cfgPath)
			require.NoError(t, err)
			updated := strings.Replace(string(body), `runtimes = ["claude"]`, `runtimes = ["claude"]`+"\n"+tt.include, 1)
			require.NoError(t, os.WriteFile(cfgPath, []byte(updated), 0o600))
			gen := loadDomainsProject(t, dir)

			// Act
			manifest, err := gen.buildPluginManifest("")

			// Assert
			require.NoError(t, err)
			var got []string
			for _, s := range manifest.Skills {
				got = append(got, s.Name)
			}
			assert.ElementsMatch(t, tt.wantSkills, got)
		})
	}
}

const placementTail = `
[marketplace]
name = "mk"
output_dir = "mkt"
[marketplace.from_domains]
name_prefix = "demo-"
[marketplace.catalog_skill]
enabled = true

[placement]
plugin = ["domains/*"]
core = ["domains/teamB/b-s"]
honor_targets = true

[claude.settings]
manage = true
enable_plugins = ["demo-teama"]
`

func TestCollectOutputs_PlacementSettingsAndCatalog(t *testing.T) {
	// Arrange
	dir := newDomainsProject(t, placementTail)
	writeDomainsFile(t, filepath.Join(dir, ".claude", "settings.json"),
		"{\n  \"permissions\": {\"allow\": [\"x\"]},\n  \"enabledPlugins\": {\"other@x\": true}\n}\n")
	gen := loadDomainsProject(t, dir)

	// Act
	outputs, _, err := gen.collectOutputs("")

	// Assert
	require.NoError(t, err)
	byRel := outputsByRel(t, dir, outputs)
	assert.Contains(t, byRel, ".claude/skills/core-s/SKILL.md")
	assert.Contains(t, byRel, ".claude/skills/b-s/SKILL.md", "core pattern wins over the domains/* plugin pattern")
	assert.NotContains(t, byRel, ".claude/skills/a-s/SKILL.md", "plugin-only domain skill stays out of .claude/skills")
	assert.NotContains(t, byRel, ".claude/skills/do-it/SKILL.md", "placement covers commands")
	assert.NotContains(t, byRel, ".claude/skills/niche-s/SKILL.md", "honor_targets drops a skill not targeting claude")
	require.Contains(t, byRel, ".claude/skills/plugin-catalog/SKILL.md")
	catalog := byRel[".claude/skills/plugin-catalog/SKILL.md"]
	assert.Contains(t, catalog, "`demo-teama`")
	assert.Contains(t, catalog, "claude plugin marketplace add ./mkt")
	assert.NotContains(t, byRel["mkt/plugins/demo-teama/skills/a-s/SKILL.md"], "plugin-catalog")

	var settings struct {
		Permissions            map[string][]string       `json:"permissions"`
		EnabledPlugins         map[string]bool           `json:"enabledPlugins"`
		ExtraKnownMarketplaces map[string]map[string]any `json:"extraKnownMarketplaces"`
	}
	require.NoError(t, json.Unmarshal([]byte(byRel[".claude/settings.json"]), &settings))
	assert.Equal(t, []string{"x"}, settings.Permissions["allow"], "unowned keys survive")
	assert.Equal(t, map[string]bool{"other@x": true, "demo-teama@mk": true}, settings.EnabledPlugins)
	assert.Equal(t, map[string]any{"source": "directory", "path": "./mkt"}, settings.ExtraKnownMarketplaces["mk"]["source"])
}

func TestCollectOutputs_DefaultsLeaveClaudeOutputUnchanged(t *testing.T) {
	// Arrange: no placement, no claude.settings, no catalog.
	dir := newDomainsProject(t, "")
	gen := loadDomainsProject(t, dir)

	// Act
	outputs, _, err := gen.collectOutputs("")

	// Assert
	require.NoError(t, err)
	byRel := outputsByRel(t, dir, outputs)
	for _, skill := range []string{"core-s", "niche-s", "a-s", "b-s", "do-it"} {
		assert.Contains(t, byRel, ".claude/skills/"+skill+"/SKILL.md")
	}
	assert.NotContains(t, byRel, ".claude/settings.json")
	assert.NotContains(t, byRel, ".claude/skills/plugin-catalog/SKILL.md")
}
