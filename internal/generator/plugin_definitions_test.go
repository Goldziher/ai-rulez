package generator

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// defsProjectHeader has no [plugin] block: every plugin comes from a definition.
const defsProjectHeader = `version = "5.0"
name = "demo"
presets = ["claude"]
gitignore = false

[[mcp_servers]]
name = "moon-ear"
command = "moon-ear"
args = ["serve"]

[[mcp_servers]]
name = "other"
command = "other"
`

// newDefsProject writes root skills notify-a, notify-b and moon-a, a command
// notify and an agent helper, plus the given config tail (appended after the
// header, so the tail starts with its own tables).
func newDefsProject(t *testing.T, tail string, extra map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	rulez := filepath.Join(dir, ".ai-rulez")
	writeDomainsFile(t, filepath.Join(rulez, "config.toml"), defsProjectHeader+tail)
	for _, skill := range []string{"notify-a", "notify-b", "moon-a"} {
		writeDomainsFile(t, filepath.Join(rulez, "skills", skill, "SKILL.md"), "---\ndescription: "+skill+"\n---\nbody\n")
	}
	writeDomainsFile(t, filepath.Join(rulez, "commands", "notify.md"), "---\ndescription: cmd\n---\nrun\n")
	writeDomainsFile(t, filepath.Join(rulez, "agents", "helper.md"), "---\ndescription: helper\n---\nagent\n")
	for rel, body := range extra {
		writeDomainsFile(t, filepath.Join(rulez, filepath.FromSlash(rel)), body)
	}
	return dir
}

const twoPluginsTail = `
[[plugins_authoring]]
name = "macos-notifications"
version = "1.2.0"
description = "Notifications"
runtimes = ["claude"]
skills = ["notify-*"]
commands = ["notify"]
mcp_servers = ["moon-ear"]
[plugins_authoring.author]
name = "Tooling"

[[plugins_authoring]]
name = "moon-ear"
version = "0.3.0"
runtimes = ["claude", "codex"]
skills = ["moon-*"]
agents = ["helper"]
`

func marketplaceIndex(t *testing.T, byRel map[string]string, rel string) (names, sources []string) {
	t.Helper()
	require.Contains(t, byRel, rel)
	var doc struct {
		Plugins []struct {
			Name   string `json:"name"`
			Source string `json:"source"`
		} `json:"plugins"`
	}
	require.NoError(t, json.Unmarshal([]byte(byRel[rel]), &doc))
	for _, p := range doc.Plugins {
		names = append(names, p.Name)
		sources = append(sources, p.Source)
	}
	return names, sources
}

func TestCollectPluginOutputs_PluginsAuthoring(t *testing.T) {
	tests := []struct {
		name       string
		tail       string
		wantFiles  []string
		wantAbsent []string
		wantNames  []string
	}{
		{
			name: "each plugin bundles only its own selection",
			tail: twoPluginsTail,
			wantFiles: []string{
				"plugins/macos-notifications/skills/notify-a/SKILL.md",
				"plugins/macos-notifications/skills/notify-b/SKILL.md",
				"plugins/macos-notifications/commands/notify.md",
				"plugins/macos-notifications/.claude-plugin/plugin.json",
				"plugins/moon-ear/skills/moon-a/SKILL.md",
				"plugins/moon-ear/agents/helper.md",
				"plugins/moon-ear/.codex-plugin/plugin.json",
				"plugins/moon-ear/.ai-rulez-generated.json",
				".claude-plugin/marketplace.json",
				".ai-rulez-generated.json",
			},
			wantAbsent: []string{
				"plugins/macos-notifications/skills/moon-a/SKILL.md",
				"plugins/macos-notifications/agents/helper.md",
				"plugins/macos-notifications/.codex-plugin/plugin.json",
				"plugins/moon-ear/skills/notify-a/SKILL.md",
				"plugins/moon-ear/commands/notify.md",
			},
			wantNames: []string{"macos-notifications", "moon-ear"},
		},
		{
			name: "the same two definitions as marketplace.plugins entries behave alike",
			tail: `
[marketplace]
name = "demo"
[[marketplace.plugins]]
name = "macos-notifications"
skills = ["notify-*"]
[[marketplace.plugins]]
name = "moon-ear"
skills = ["moon-*"]
`,
			wantFiles:  []string{"plugins/macos-notifications/skills/notify-a/SKILL.md", "plugins/moon-ear/skills/moon-a/SKILL.md"},
			wantAbsent: []string{"plugins/moon-ear/skills/notify-a/SKILL.md"},
			wantNames:  []string{"macos-notifications", "moon-ear"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := newDefsProject(t, tt.tail, nil)
			gen := loadDomainsProject(t, dir)

			// Act
			outputs, err := gen.collectPluginOutputs("")

			// Assert
			require.NoError(t, err)
			byRel := outputsByRel(t, dir, outputs)
			for _, want := range tt.wantFiles {
				assert.Contains(t, byRel, want)
			}
			for _, absent := range tt.wantAbsent {
				assert.NotContains(t, byRel, absent)
			}
			names, sources := marketplaceIndex(t, byRel, ".claude-plugin/marketplace.json")
			assert.Equal(t, tt.wantNames, names)
			for i, name := range names {
				assert.Equal(t, "./plugins/"+name, sources[i])
			}
		})
	}
}

func TestCollectPluginOutputs_PluginsAuthoring_PerPluginFields(t *testing.T) {
	// Arrange
	hooksTail := strings.Replace(twoPluginsTail, "name = \"Tooling\"\n", `name = "Tooling"

[[plugins_authoring.hooks]]
event = "SessionStart"
[[plugins_authoring.hooks.hooks]]
type = "command"
command = "echo hi"
`, 1)
	dir := newDefsProject(t, hooksTail, nil)
	gen := loadDomainsProject(t, dir)

	// Act
	outputs, err := gen.collectPluginOutputs("")

	// Assert
	require.NoError(t, err)
	byRel := outputsByRel(t, dir, outputs)
	var notify, moon struct {
		Author     map[string]string `json:"author"`
		Version    string            `json:"version"`
		MCPServers map[string]any    `json:"mcpServers"`
	}
	require.NoError(t, json.Unmarshal([]byte(byRel["plugins/macos-notifications/.claude-plugin/plugin.json"]), &notify))
	require.NoError(t, json.Unmarshal([]byte(byRel["plugins/moon-ear/.claude-plugin/plugin.json"]), &moon))
	assert.Equal(t, "Tooling", notify.Author["name"])
	assert.Equal(t, "1.2.0", notify.Version)
	assert.Equal(t, []string{"moon-ear"}, mapKeys(notify.MCPServers), "only the named server is bundled")
	assert.Empty(t, moon.MCPServers, "a plugin that names no server bundles none")
	assert.Equal(t, "0.3.0", moon.Version)
	assert.NotContains(t, byRel, "plugins/moon-ear/hooks/hooks.json", "hooks belong to the plugin that declares them")
	require.Contains(t, byRel, "plugins/macos-notifications/hooks/hooks.json")
	assert.Contains(t, byRel["plugins/macos-notifications/hooks/hooks.json"], "echo hi")
}

func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func TestCollectPluginOutputs_PluginsAuthoring_Errors(t *testing.T) {
	tests := []struct {
		name    string
		tail    string
		wantErr string
	}{
		{
			name: "an MCP server the root does not define",
			tail: `
[[plugins_authoring]]
name = "a"
skills = ["notify-*"]
mcp_servers = ["ghost"]
`,
			wantErr: `bundles MCP server "ghost"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := newDefsProject(t, tt.tail, nil)
			gen := loadDomainsProjectUnvalidated(t, dir)

			// Act
			_, err := gen.collectPluginOutputs("")

			// Assert
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestCollectPluginOutputs_PluginNameSelector(t *testing.T) {
	tests := []struct {
		name       string
		names      []string
		wantFiles  []string
		wantAbsent []string
		wantErr    string
	}{
		{
			name:       "one plugin only, without the index",
			names:      []string{"moon-ear"},
			wantFiles:  []string{"plugins/moon-ear/skills/moon-a/SKILL.md"},
			wantAbsent: []string{"plugins/macos-notifications/.claude-plugin/plugin.json", ".claude-plugin/marketplace.json"},
		},
		{name: "an unknown name lists the plugins", names: []string{"ghost"}, wantErr: "moon-ear"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			dir := newDefsProject(t, twoPluginsTail, nil)
			gen := loadDomainsProject(t, dir).WithPluginNames(tt.names)

			// Act
			outputs, err := gen.collectPluginOutputs("")

			// Assert
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			byRel := outputsByRel(t, dir, outputs)
			for _, want := range tt.wantFiles {
				assert.Contains(t, byRel, want)
			}
			for _, absent := range tt.wantAbsent {
				assert.NotContains(t, byRel, absent)
			}
		})
	}
}

func TestCollectPluginOutputs_PluginsFromDefinitionFiles(t *testing.T) {
	// Arrange: the same two plugins, one file each, no inline definitions.
	dir := newDefsProject(t, "", map[string]string{
		"plugins/macos-notifications.toml": "version = \"1.2.0\"\nruntimes = [\"claude\"]\nskills = [\"notify-*\"]\n",
		"plugins/moon-ear.toml":            "version = \"0.3.0\"\nruntimes = [\"claude\"]\nskills = [\"moon-*\"]\n",
	})
	gen := loadDomainsProject(t, dir)

	// Act
	outputs, err := gen.collectPluginOutputs("")

	// Assert
	require.NoError(t, err)
	byRel := outputsByRel(t, dir, outputs)
	names, _ := marketplaceIndex(t, byRel, ".claude-plugin/marketplace.json")
	assert.Equal(t, []string{"macos-notifications", "moon-ear"}, names)
	assert.Contains(t, byRel, "plugins/moon-ear/skills/moon-a/SKILL.md")
}

func TestVerifyPlugin_PluginsAuthoring_ReportsThePluginPerDriftEntry(t *testing.T) {
	// Arrange
	dir := newDefsProject(t, twoPluginsTail, nil)
	gen := loadDomainsProject(t, dir)
	require.NoError(t, gen.GeneratePlugin(""))
	require.NoError(t, gen.VerifyPlugin(""), "freshly generated output verifies")

	// A hand edit in one plugin is drift naming that plugin only.
	skill := filepath.Join(dir, "plugins", "moon-ear", "skills", "moon-a", "SKILL.md")
	require.NoError(t, os.WriteFile(skill, []byte("edited"), 0o600))

	// Act
	err := gen.VerifyPlugin("")

	// Assert
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrPluginDrift))
	drift := PluginDriftOf(err)
	require.NotEmpty(t, drift)
	for _, item := range drift {
		assert.Equal(t, "moon-ear", item.Plugin, item.Path)
	}
}

func TestGeneratePlugin_PluginsAuthoring_RemovesAPluginThatIsNoLongerDefined(t *testing.T) {
	// Arrange
	dir := newDefsProject(t, twoPluginsTail, nil)
	gen := loadDomainsProject(t, dir)
	require.NoError(t, gen.GeneratePlugin(""))
	require.DirExists(t, filepath.Join(dir, "plugins", "moon-ear"))

	cfgPath := filepath.Join(dir, ".ai-rulez", "config.toml")
	body, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	cut := strings.Index(string(body), "[[plugins_authoring]]\nname = \"moon-ear\"")
	require.Positive(t, cut)
	require.NoError(t, os.WriteFile(cfgPath, body[:cut], 0o600))

	// Act
	regenerated := loadDomainsProject(t, dir)
	require.NoError(t, regenerated.GeneratePlugin(""))

	// Assert
	assert.NoDirExists(t, filepath.Join(dir, "plugins", "moon-ear"))
	assert.DirExists(t, filepath.Join(dir, "plugins", "macos-notifications"))
}

// loadDomainsProjectUnvalidated loads a project without Validate, to reach the
// generator's own checks.
func loadDomainsProjectUnvalidated(t *testing.T, dir string) *Generator {
	t.Helper()
	cfg, err := config.LoadConfig(context.Background(), dir)
	require.NoError(t, err)
	return NewGenerator(cfg)
}
