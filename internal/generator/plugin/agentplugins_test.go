package plugin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/agentplugins"
	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderAgentPlugins_ManifestSkillsAndMCP(t *testing.T) {
	baseDir := t.TempDir()
	srcDir := t.TempDir()
	skillDir := filepath.Join(srcDir, "skills", "deploy")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: deploy\ndescription: Deploy the service.\n---\nbody\n"), 0o644))

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

	manifest := parseJSON(t, []byte(byPath[filepath.ToSlash(filepath.Join(baseDir, "plugin.json"))]))
	assert.Equal(t, agentplugins.PluginSchemaID(agentplugins.DefaultSpec), manifest["$schema"])
	assert.Equal(t, "acme.tools", manifest["name"])
	assert.Equal(t, "1.2.0", manifest["version"])
	assert.NotContains(t, manifest, "runtimes")
	assert.NotContains(t, manifest, "mcpServers")

	assert.Contains(t, byPath, filepath.ToSlash(filepath.Join(baseDir, "skills", "deploy", "SKILL.md")))

	mcp := parseJSON(t, []byte(byPath[filepath.ToSlash(filepath.Join(baseDir, "mcp.json"))]))
	assert.Equal(t, agentplugins.MCPSchemaID(agentplugins.DefaultSpec), mcp["$schema"])
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

// renderAP generates the agent-plugins bundle of m and returns its files by
// slash path relative to baseDir.
func renderAP(t *testing.T, m *Manifest) (map[string]config.OutputFile, string) {
	t.Helper()
	baseDir := t.TempDir()
	m.Runtimes = []string{config.PluginRuntimeAgentPlugins}
	outputs, err := Generate(m, baseDir)
	require.NoError(t, err)
	out := map[string]config.OutputFile{}
	for _, o := range outputs {
		rel, err := filepath.Rel(baseDir, o.Path)
		require.NoError(t, err)
		out[filepath.ToSlash(rel)] = o
	}
	return out, baseDir
}

func writeSkill(t *testing.T, name, md string, files map[string]string) config.ContentFile {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "skills", name)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(md), 0o644))
	for rel, body := range files {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, rel)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, rel), []byte(body), 0o755))
	}
	return config.ContentFile{Name: name, Path: filepath.Join(dir, "SKILL.md")}
}

func TestRenderAgentPlugins_SpecSelectsTheSchema(t *testing.T) {
	out, _ := renderAP(t, &Manifest{
		Name: "acme", Version: "1.0.0", Spec: agentplugins.Spec110,
		MCP: []config.PluginMCPLaunch{{Name: "remote", Transport: config.TransportHTTP, URL: "https://example.com/mcp"}},
	})
	assert.Equal(t, agentplugins.PluginSchemaID(agentplugins.Spec110), parseJSON(t, out["plugin.json"].RawContent)["$schema"])
	assert.Equal(t, agentplugins.MCPSchemaID(agentplugins.Spec110), parseJSON(t, out["mcp.json"].RawContent)["$schema"])
}

func TestRenderAgentPlugins_OutputValidatesAgainstTheSpec(t *testing.T) {
	skill := writeSkill(t, "deploy", "---\nname: deploy\ndescription: Deploy.\n---\nbody\n", map[string]string{"scripts/run.sh": "#!/bin/sh\n"})
	out, _ := renderAP(t, &Manifest{
		Name: "acme", Version: "1.0.0", Skills: []config.ContentFile{skill},
		MCP: []config.PluginMCPLaunch{{Name: "local", Command: "${PLUGIN_ROOT}/bin/server", Args: []string{"--root", "${PLUGIN_ROOT}"}}},
	})
	res := agentplugins.Validate(os.DirFS(t.TempDir()))
	assert.True(t, res.Rejected, "an empty directory is not a plugin")

	dir := t.TempDir()
	for rel, o := range out {
		dst := filepath.Join(dir, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(dst), 0o755))
		require.NoError(t, os.WriteFile(dst, o.RawContent, 0o644))
	}
	res = agentplugins.Validate(os.DirFS(dir))
	assert.False(t, agentplugins.HasErrors(res.Findings), "%v", res.Findings)
	assert.NotZero(t, out["skills/deploy/scripts/run.sh"].Mode, "the executable bit of a skill script is kept")
}

func TestRenderAgentPlugins_DropsWhatTheSpecRejects(t *testing.T) {
	good := writeSkill(t, "good", "---\nname: good\ndescription: Fine.\n---\n", nil)
	noDescription := writeSkill(t, "bare", "---\nname: bare\n---\nbody\n", nil)
	out, _ := renderAP(t, &Manifest{
		Name: "acme", Version: "1.0.0", Skills: []config.ContentFile{good, noDescription},
		MCP: []config.PluginMCPLaunch{
			{Name: "token", Command: "srv", Args: []string{"--key", "${API_KEY}"}},
			{Name: "off", Command: "srv", Disabled: true},
			{Name: "ok", Command: "srv", Env: map[string]string{"PLUGIN_DIR": "${PLUGIN_ROOT}/x"}},
		},
	})
	assert.Contains(t, out, "skills/good/SKILL.md")
	assert.NotContains(t, out, "skills/bare/SKILL.md", "a skill without a description is skipped by conformant clients")
	servers := parseJSON(t, out["mcp.json"].RawContent)["mcpServers"].(map[string]any)
	assert.Contains(t, servers, "ok")
	assert.NotContains(t, servers, "token", "${API_KEY} is never expanded by Agent Plugins clients")
	assert.NotContains(t, servers, "off", "a disabled server is not packaged")
}

func TestRenderAgentPlugins_IsDeterministic(t *testing.T) {
	skill := writeSkill(t, "deploy", "---\nname: deploy\ndescription: Deploy.\n---\n", nil)
	m := func() *Manifest {
		return &Manifest{Name: "acme", Version: "1.0.0", Skills: []config.ContentFile{skill}, Keywords: []string{"b", "a"},
			MCP: []config.PluginMCPLaunch{{Name: "z", Command: "z"}, {Name: "a", Command: "a", Env: map[string]string{"B": "1", "A": "2"}}}}
	}
	first, _ := renderAP(t, m())
	for range 5 {
		again, _ := renderAP(t, m())
		for rel, o := range first {
			assert.Equal(t, string(o.RawContent), string(again[rel].RawContent), rel)
		}
	}
}

func TestGenerate_ClaudeDoesNotReAddSkillsAgentPluginsDropped(t *testing.T) {
	good := writeSkill(t, "good", "---\nname: good\ndescription: Fine.\n---\n", map[string]string{"scripts/run.sh": "#!/bin/sh\n"})
	bare := writeSkill(t, "bare", "---\nname: bare\n---\nbody\n", map[string]string{"notes.md": "n\n"})
	baseDir := t.TempDir()
	outputs, err := Generate(&Manifest{
		Name: "acme", Version: "1.0.0", Skills: []config.ContentFile{good, bare},
		Runtimes: []string{config.PluginRuntimeAgentPlugins, config.PluginRuntimeClaude},
	}, baseDir)
	require.NoError(t, err)
	var paths []string
	for _, o := range outputs {
		rel, err := filepath.Rel(baseDir, o.Path)
		require.NoError(t, err)
		paths = append(paths, filepath.ToSlash(rel))
	}
	assert.Contains(t, paths, "skills/good/SKILL.md")
	assert.Contains(t, paths, "skills/good/scripts/run.sh")
	for _, p := range paths {
		assert.NotContains(t, p, "skills/bare", "the claude runtime must not re-add a dropped skill")
	}
}
