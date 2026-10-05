package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const staleTail = `
[marketplace]
name = "mk"
output_dir = "mkt"
[marketplace.from_domains]
name_prefix = "demo-"
`

func TestGeneratePlugin_RemovesStalePluginDirectory(t *testing.T) {
	dir := newDomainsProject(t, staleTail)
	require.NoError(t, loadDomainsProject(t, dir).GeneratePlugin(""))
	teamA := filepath.Join(dir, "mkt", "plugins", "demo-teama")
	teamB := filepath.Join(dir, "mkt", "plugins", "demo-teamb")
	require.DirExists(t, teamA)
	require.DirExists(t, teamB)

	// A hand-made directory and an unrelated file must survive.
	handMade := filepath.Join(dir, "mkt", "plugins", "mine")
	writeDomainsFile(t, filepath.Join(handMade, "keep.txt"), "mine")
	unrelated := filepath.Join(dir, "mkt", "README.md")
	writeDomainsFile(t, unrelated, "readme")

	require.NoError(t, os.RemoveAll(filepath.Join(dir, ".ai-rulez", "domains", "teamB")))
	gen := loadDomainsProject(t, dir)

	lines, err := gen.DryRunPlugin("")
	require.NoError(t, err)
	assert.Contains(t, lines, "delete-stale: "+filepath.Join("mkt", "plugins", "demo-teamb"))
	assert.DirExists(t, teamB, "dry run must not delete")

	require.Error(t, gen.VerifyPlugin(""), "verify must flag the stale directory")

	require.NoError(t, gen.GeneratePlugin(""))
	assert.NoDirExists(t, teamB)
	assert.DirExists(t, teamA)
	assert.FileExists(t, filepath.Join(handMade, "keep.txt"))
	assert.FileExists(t, unrelated)
	require.NoError(t, loadDomainsProject(t, dir).VerifyPlugin(""))
}

func TestGeneratePlugin_StaleDirKeepsFilesItDidNotGenerate(t *testing.T) {
	dir := newDomainsProject(t, staleTail)
	require.NoError(t, loadDomainsProject(t, dir).GeneratePlugin(""))
	teamB := filepath.Join(dir, "mkt", "plugins", "demo-teamb")
	extra := filepath.Join(teamB, "notes", "mine.md")
	writeDomainsFile(t, extra, "hand written")
	require.NoError(t, os.RemoveAll(filepath.Join(dir, ".ai-rulez", "domains", "teamB")))

	require.NoError(t, loadDomainsProject(t, dir).GeneratePlugin(""))
	assert.FileExists(t, extra, "a file ai-rulez did not generate is never deleted")
	assert.NoFileExists(t, filepath.Join(teamB, "agents", "ag.md"))
	assert.NoFileExists(t, filepath.Join(teamB, ".ai-rulez-generated.json"))
}

func TestPlacementReport(t *testing.T) {
	tests := []struct {
		name      string
		tail      string
		wantCore  []string
		wantIssue map[string]string // item name -> substring of the issue
		wantPlug  map[string][]string
	}{
		{
			name: "plugin-only items in enabled plugins are clean",
			tail: `
[placement]
default = "plugin"
core = ["core-s", "niche-s"]

[marketplace]
name = "mk"
output_dir = "mkt"
[marketplace.from_domains]
name_prefix = "demo-"

[claude.settings]
manage = true
enable_plugins = ["demo-teama", "demo-teamb"]
`,
			wantCore:  []string{"core-s", "niche-s"},
			wantIssue: map[string]string{},
			wantPlug:  map[string][]string{"a-s": {"demo-teama"}, "b-s": {"demo-teamb"}, "do-it": {"demo-teama"}},
		},
		{
			name: "plugin not enabled and no catalog is flagged",
			tail: `
[placement]
default = "plugin"
core = ["core-s", "niche-s"]

[marketplace]
name = "mk"
output_dir = "mkt"
[marketplace.from_domains]
name_prefix = "demo-"

[claude.settings]
manage = true
enable_plugins = ["demo-teama"]
`,
			wantCore:  []string{"core-s", "niche-s"},
			wantIssue: map[string]string{"b-s": "not enabled"},
			wantPlug:  map[string][]string{"a-s": {"demo-teama"}, "b-s": {"demo-teamb"}},
		},
		{
			name: "a catalog skill makes every plugin discoverable",
			tail: `
[placement]
default = "plugin"
core = ["core-s", "niche-s"]

[marketplace]
name = "mk"
output_dir = "mkt"
[marketplace.from_domains]
name_prefix = "demo-"
[marketplace.catalog_skill]
enabled = true
`,
			wantCore:  []string{"core-s", "niche-s"},
			wantIssue: map[string]string{},
		},
		{
			name: "plugin-only items outside any marketplace plugin are flagged",
			tail: `
[placement]
default = "plugin"
core = ["niche-s"]
`,
			wantCore:  []string{"niche-s"},
			wantIssue: map[string]string{"core-s": "not enabled", "a-s": "no plugin bundles", "b-s": "no plugin bundles", "do-it": "no plugin bundles"},
		},
		{
			name:      "default placement keeps everything core",
			tail:      "",
			wantCore:  []string{"a-s", "b-s", "core-s", "niche-s"},
			wantIssue: map[string]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := newDomainsProject(t, tt.tail)
			report, err := loadDomainsProject(t, dir).PlacementReport("")
			require.NoError(t, err)

			var core []string
			issues := map[string]string{}
			for _, it := range report.Items {
				if it.Destination == DestinationCore && it.Type == "skill" {
					core = append(core, it.Name)
				}
				if it.Issue != "" {
					issues[it.Name] = it.Issue
				}
				if want, ok := tt.wantPlug[it.Name]; ok {
					assert.Equal(t, want, it.Plugins, it.Name)
				}
			}
			assert.Equal(t, tt.wantCore, core)
			assert.Len(t, issues, len(tt.wantIssue), "%v", issues)
			for name, sub := range tt.wantIssue {
				assert.Contains(t, issues[name], sub, name)
			}
			assert.Len(t, report.Items, 5, "4 skills and 1 command")
		})
	}
}

func TestCollectPluginOutputs_CursorIndexForDomainPlugins(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		tail := staleTail
		if enabled {
			tail += "cursor_index = true\n"
		}
		// cursor_index belongs to [marketplace], not [marketplace.from_domains].
		tail = strings.Replace(tail, "[marketplace.from_domains]\nname_prefix = \"demo-\"\n", "", 1)
		if enabled {
			tail = strings.Replace(tail, "cursor_index = true\n", "cursor_index = true\n[marketplace.from_domains]\nname_prefix = \"demo-\"\n", 1)
		} else {
			tail += "[marketplace.from_domains]\nname_prefix = \"demo-\"\n"
		}
		dir := newDomainsProject(t, tail)
		gen := loadDomainsProject(t, dir)
		outputs, err := gen.collectPluginOutputs("")
		require.NoError(t, err)
		files := outputsByRel(t, dir, outputs)
		_, has := files["mkt/.cursor-plugin/marketplace.json"]
		assert.Equal(t, enabled, has, "cursor_index=%v", enabled)
		if enabled {
			assert.Contains(t, files["mkt/.cursor-plugin/marketplace.json"], `"source": "plugins/demo-teama"`)
			assert.Contains(t, files["mkt/.ai-rulez-generated.json"], ".cursor-plugin/marketplace.json")
		}
	}
}

func TestGeneratePlugin_ProfileLeavingDomainOutDoesNotDeleteItsPlugin(t *testing.T) {
	dir := newDomainsProject(t, staleTail+"\n[profiles]\nall = [\"teamA\", \"teamB\"]\nonly-a = [\"teamA\"]\n")
	require.NoError(t, loadDomainsProject(t, dir).GeneratePlugin("all"))
	teamB := filepath.Join(dir, "mkt", "plugins", "demo-teamb")
	require.DirExists(t, teamB)

	gen := loadDomainsProject(t, dir)
	lines, err := gen.DryRunPlugin("only-a")
	require.NoError(t, err)
	assert.NotContains(t, lines, "delete-stale: "+filepath.Join("mkt", "plugins", "demo-teamb"))
	require.NoError(t, gen.GeneratePlugin("only-a"))
	assert.DirExists(t, teamB, "a profile that omits a domain must not delete that domain's plugin")
}
