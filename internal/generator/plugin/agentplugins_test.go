package plugin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderAgentPlugins_ManifestSkillsAndMCP(t *testing.T) {
	baseDir := t.TempDir()
	srcDir := t.TempDir()
	skillDir := filepath.Join(srcDir, "skills", "deploy")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: deploy\n---\nbody\n"), 0o644))

	m := &Manifest{
		Name:        "acme.tools",
		Version:     "1.2.0",
		Description: "Portable plugin.",
		Author:      &config.Author{Name: "Acme"},
		Keywords:    []string{"mcp"},
		Runtimes:    []string{config.PluginRuntimeAgentPlugins},
		MCP: []config.PluginMCPLaunch{
			{Name: "local", Command: "${PLUGIN_ROOT}/bin/server", Args: []string{"serve"}},
			{Name: "remote", Transport: config.TransportHTTP, URL: "https://example.com/mcp"},
		},
		Skills: []config.ContentFile{{Name: "deploy", Path: filepath.Join(skillDir, "SKILL.md")}},
	}

	outputs, err := Generate(m, baseDir)
	require.NoError(t, err)

	byPath := map[string]string{}
	for _, o := range outputs {
		body := string(o.RawContent)
		if o.RawContent == nil {
			body = o.Content
		}
		byPath[filepath.ToSlash(o.Path)] = body
	}

	manifest := parseJSON(t, []byte(byPath[filepath.Join(baseDir, "plugin.json")]))
	assert.Equal(t, agentPluginsSchema, manifest["$schema"])
	assert.Equal(t, "acme.tools", manifest["name"])
	assert.Equal(t, "1.2.0", manifest["version"])
	assert.NotContains(t, manifest, "runtimes")
	assert.NotContains(t, manifest, "mcpServers")

	assert.Contains(t, byPath, filepath.Join(baseDir, "skills", "deploy", "SKILL.md"))

	mcp := parseJSON(t, []byte(byPath[filepath.Join(baseDir, "mcp.json")]))
	assert.Equal(t, agentPluginsMCPSchema, mcp["$schema"])
	servers := mcp["mcpServers"].(map[string]any)
	local := servers["local"].(map[string]any)
	assert.Equal(t, "stdio", local["type"])
	assert.Equal(t, "./bin/server", local["command"], "PLUGIN_ROOT-rooted command becomes plugin-relative")
	remote := servers["remote"].(map[string]any)
	assert.Equal(t, "streamable-http", remote["type"])
	assert.Equal(t, "https://example.com/mcp", remote["url"])
	assert.NotContains(t, remote, "command")
}

func TestRenderAgentPlugins_OmitsMCPWhenNoServers(t *testing.T) {
	baseDir := t.TempDir()
	m := &Manifest{Name: "skills-only", Version: "1.0.0", Runtimes: []string{config.PluginRuntimeAgentPlugins}}

	outputs, err := Generate(m, baseDir)
	require.NoError(t, err)
	for _, o := range outputs {
		assert.NotEqual(t, filepath.Join(baseDir, "mcp.json"), o.Path, "no MCP servers means no mcp.json")
	}
}
