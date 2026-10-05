package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// layoutManifest builds a plugin with one skill, one command and one agent
// authored on disk, bundled for the given runtimes.
func layoutManifest(t *testing.T, runtimes ...string) (*Manifest, string) {
	t.Helper()
	src := t.TempDir()
	write := func(rel, body string) string {
		path := filepath.Join(src, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
		return path
	}
	skill := write("skills/deploy/SKILL.md", "---\nname: deploy\ndescription: Deploy it\n---\nbody\n")
	command := write("commands/ship.md", "---\ndescription: Ship \"it\"\n---\nShip $ARGUMENTS now.\nUse \"\"\" carefully \\ ok.\n")
	agent := write("agents/reviewer.md", "---\nname: reviewer\ndescription: Reviews\n---\nReview.\n")

	m := &Manifest{
		Name:        "acme.tools",
		Version:     "1.2.0",
		Description: "Acme tools.",
		Author:      &config.Author{Name: "Acme", Email: "dev@acme.example"},
		Category:    "Developer Tools",
		Runtimes:    runtimes,
		Market:      MarketInfo{Name: "acme", Description: "Acme marketplace", Owner: &config.Author{Name: "Acme"}},
		Interface:   &config.PluginInterface{DisplayName: "Acme Tools", ShortDescription: "Tools for Acme"},
		MCP:         []config.PluginMCPLaunch{{Name: "srv", Command: "${PLUGIN_ROOT}/bin/srv", Args: []string{"serve"}}},
		Skills:      []config.ContentFile{{Name: "deploy", Path: skill}},
		Commands:    []config.ContentFile{{Name: "ship", Path: command}},
		Agents:      []config.ContentFile{{Name: "reviewer", Path: agent}},
		SourceDir:   src,
	}
	return m, src
}

func render(t *testing.T, m *Manifest) map[string][]byte {
	t.Helper()
	outs, err := Generate(m, "/out")
	require.NoError(t, err)
	byPath := map[string][]byte{}
	for _, o := range outs {
		rel, err := filepath.Rel("/out", o.Path)
		require.NoError(t, err)
		body := o.RawContent
		if body == nil {
			body = []byte(o.Content)
		}
		byPath[filepath.ToSlash(rel)] = body
	}
	return byPath
}

func TestCodexManifestLayouts(t *testing.T) {
	tests := []struct {
		name       string
		codex      *config.CodexExtras
		wantLegacy bool
		wantRoot   bool
	}{
		{"default is legacy", nil, true, false},
		{"explicit legacy", &config.CodexExtras{Manifest: "legacy"}, true, false},
		{"root", &config.CodexExtras{Manifest: "root"}, false, true},
		{"both", &config.CodexExtras{Manifest: "both"}, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := layoutManifest(t, config.PluginRuntimeCodex)
			m.Codex = tt.codex
			out := render(t, m)

			assert.Equal(t, tt.wantLegacy, hasPath(out, ".codex-plugin/plugin.json"))
			assert.Equal(t, tt.wantRoot, hasPath(out, "plugin.json"))
			assert.Equal(t, tt.wantRoot, hasPath(out, "mcp.json"))
			assert.Contains(t, out, "skills/deploy/SKILL.md")
			assert.Contains(t, out, ".ai-rulez-generated.json", "every bundle carries a provenance sidecar")
			assert.NotContains(t, out, ".agents/plugins/marketplace.json", "the Codex index is opt-in")

			if tt.wantRoot {
				doc := parseJSON(t, out["plugin.json"])
				assert.Equal(t, agentPluginsSchema, doc["$schema"])
				assert.NotContains(t, doc, "skills", "skills/ is fixed in the portable format")
				assert.NotContains(t, doc, "mcpServers", "MCP lives in mcp.json")
				iface := doc["extensions"].(map[string]any)["com.openai"].(map[string]any)["interface"].(map[string]any)
				assert.Equal(t, "Acme Tools", iface["displayName"])
				mcp := parseJSON(t, out["mcp.json"])
				srv := mcp["mcpServers"].(map[string]any)["srv"].(map[string]any)
				assert.Equal(t, "stdio", srv["type"], "the portable mcp.json requires a transport type")
				assert.Equal(t, "./bin/srv", srv["command"])
			}
			if tt.wantLegacy {
				doc := parseJSON(t, out[".codex-plugin/plugin.json"])
				assert.Equal(t, "./.mcp.json", doc["mcpServers"])
			}
		})
	}
}

func hasPath(out map[string][]byte, p string) bool { _, ok := out[p]; return ok }

func TestCodexRootSharesPluginJSONWithAgentPlugins(t *testing.T) {
	m, _ := layoutManifest(t, config.PluginRuntimeCodex, config.PluginRuntimeAgentPlugins)
	m.Codex = &config.CodexExtras{Manifest: "root"}
	both := render(t, m)

	m2, _ := layoutManifest(t, config.PluginRuntimeAgentPlugins)
	m2.Codex = &config.CodexExtras{Manifest: "root"}
	alone := render(t, m2)
	assert.NotContains(t, string(alone["plugin.json"]), "com.openai",
		"the Codex overlay is only written when the codex runtime is bundled")
	assert.Contains(t, string(both["plugin.json"]), "com.openai")
}

func TestCodexSinglePluginMarketplace(t *testing.T) {
	m, _ := layoutManifest(t, config.PluginRuntimeCodex)
	m.Codex = &config.CodexExtras{Manifest: "root", Marketplace: true}
	out := render(t, m)

	doc := parseJSON(t, out[".agents/plugins/marketplace.json"])
	assert.Equal(t, "acme", doc["name"])
	plugins := doc["plugins"].([]any)
	require.Len(t, plugins, 1)
	entry := plugins[0].(map[string]any)
	assert.Equal(t, "acme.tools", entry["name"])
	assert.Equal(t, map[string]any{"source": "local", "path": "./"}, entry["source"])
	assert.Equal(t, "AVAILABLE", entry["policy"].(map[string]any)["installation"])
	assert.Equal(t, "Developer Tools", entry["category"])

	sidecar := parseJSON(t, out[".ai-rulez-generated.json"])
	assert.Contains(t, sidecar["outputs"], ".agents/plugins/marketplace.json", "provenance covers the index")
}

func TestCursorSinglePluginMarketplace(t *testing.T) {
	m, _ := layoutManifest(t, config.PluginRuntimeCursor)
	assert.NotContains(t, render(t, m), ".cursor-plugin/marketplace.json", "opt-in")

	m.Cursor = &config.CursorExtras{Marketplace: true}
	out := render(t, m)
	doc := parseJSON(t, out[".cursor-plugin/marketplace.json"])
	assert.Equal(t, "acme", doc["name"])
	assert.Equal(t, "Acme", doc["owner"].(map[string]any)["name"])
	assert.Equal(t, "Acme marketplace", doc["metadata"].(map[string]any)["description"])
	entry := doc["plugins"].([]any)[0].(map[string]any)
	assert.Equal(t, "acme.tools", entry["name"])
	assert.Equal(t, ".", entry["source"])
	assert.Contains(t, out, ".cursor-plugin/plugin.json")
}

func TestRenderCursorMarketplace_Members(t *testing.T) {
	out, err := RenderCursorMarketplace(
		MarketInfo{Name: "acme", Owner: &config.Author{Name: "Acme"}},
		[]MemberEntry{
			{Name: "a", Description: "A", Source: "./plugins/a"},
			{Name: "b", Source: "./plugins/b", Detailed: true, Version: "2.0.0", Category: "x"},
		}, "/out")
	require.NoError(t, err)
	doc := parseJSON(t, out.RawContent)
	plugins := doc["plugins"].([]any)
	assert.Equal(t, "plugins/a", plugins[0].(map[string]any)["source"])
	assert.Equal(t, "2.0.0", plugins[1].(map[string]any)["version"])
	assert.Equal(t, filepath.Join("/out", ".cursor-plugin", "marketplace.json"), out.Path)
}

func TestCopilotRuntime(t *testing.T) {
	m, _ := layoutManifest(t, config.PluginRuntimeCopilot)
	out := render(t, m)

	doc := parseJSON(t, out["plugin.json"])
	assert.Equal(t, agentPluginsSchema, doc["$schema"])
	assert.Equal(t, "acme.tools", doc["name"])
	assert.NotContains(t, doc, "extensions", "the Codex overlay belongs to the codex runtime")

	assert.Contains(t, out, "skills/deploy/SKILL.md")
	assert.Contains(t, out, "mcp.json")
	assert.Contains(t, out, "com.github.copilot/agents/reviewer.agent.md")
	agent := string(out["com.github.copilot/agents/reviewer.agent.md"])
	assert.True(t, strings.HasPrefix(agent, "---\nname: reviewer\ndescription: Reviews\n---\n"), "frontmatter passes through verbatim")
	assert.True(t, strings.HasSuffix(agent, "Review.\n"))
	for p := range out {
		assert.NotContains(t, p, "commands", "copilot command/hook formats are undocumented and not emitted")
		assert.NotContains(t, p, "hooks")
	}

	market := parseJSON(t, out[".github/plugin/marketplace.json"])
	assert.Equal(t, "acme", market["name"])
	assert.Equal(t, "Acme", market["owner"].(map[string]any)["name"])
	entry := market["plugins"].([]any)[0].(map[string]any)
	assert.Equal(t, "./", entry["source"])
	assert.Equal(t, "1.2.0", entry["version"])
	assert.NotContains(t, market, "description", "Copilot carries the description under metadata")

	sidecar := parseJSON(t, out[".ai-rulez-generated.json"])
	for _, p := range []string{"plugin.json", "mcp.json", "com.github.copilot/agents/reviewer.agent.md", ".github/plugin/marketplace.json"} {
		assert.Contains(t, sidecar["outputs"], p, "provenance covers %s", p)
	}
}

func TestCopilotIsOptIn(t *testing.T) {
	assert.NotContains(t, config.AllPluginRuntimes, config.PluginRuntimeCopilot)
	assert.Contains(t, config.KnownPluginRuntimes, config.PluginRuntimeCopilot)
}

func TestGeminiCommands(t *testing.T) {
	m, _ := layoutManifest(t, config.PluginRuntimeGemini)
	assert.NotContains(t, render(t, m), "commands/ship.toml", "opt-in")

	m.Gemini = &config.GeminiExtras{Commands: true}
	out := render(t, m)
	got := string(out["commands/ship.toml"])
	want := "description = \"Ship \\\"it\\\"\"\n" +
		"prompt = \"\"\"\nShip {{args}} now.\nUse \\\"\\\"\\\" carefully \\\\ ok.\n\"\"\"\n"
	assert.Equal(t, want, got)
	assert.Contains(t, out, "gemini-extension.json")
	assert.Contains(t, out, ".ai-rulez-generated.json")
}

func TestLegacyDefaultsEmitNoNewFiles(t *testing.T) {
	m, _ := layoutManifest(t, config.AllPluginRuntimes...)
	out := render(t, m)
	for _, p := range []string{
		"plugin.json", "mcp.json", "commands/ship.toml", ".cursor-plugin/marketplace.json",
		".agents/plugins/marketplace.json", ".github/plugin/marketplace.json", "com.github.copilot/agents/reviewer.agent.md",
	} {
		assert.NotContains(t, out, p, "default output must be unchanged: %s", p)
	}
}
