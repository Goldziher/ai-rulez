package plugin

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixtureDir is the committed basemind authoring fixture, relative to this package.
const fixtureDir = "../../../tests/fixtures/plugin/basemind"

// generateFixture loads the basemind fixture and returns the rendered outputs
// indexed by their slash-normalized path (rooted at an arbitrary out dir). JSON
// files are compared semantically, so key order is irrelevant.
func generateFixture(t *testing.T) map[string][]byte {
	t.Helper()
	cfg, err := config.LoadConfig(context.Background(), fixtureDir)
	require.NoError(t, err)
	require.NotNil(t, cfg.Plugin, "fixture must define a [plugin] block")

	m, err := BuildManifest(cfg, cfg.Content)
	require.NoError(t, err)

	outs, err := Generate(m, "/out")
	require.NoError(t, err)

	byPath := make(map[string][]byte, len(outs))
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

func parseJSON(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var doc map[string]any
	require.NoError(t, json.Unmarshal(data, &doc))
	return doc
}

func TestGolden_AllRuntimeManifestsEmitted(t *testing.T) {
	out := generateFixture(t)
	for _, p := range []string{
		".claude-plugin/plugin.json",
		".claude-plugin/marketplace.json",
		".cursor-plugin/plugin.json",
		".codex-plugin/plugin.json",
		".mcp.json",
		"gemini-extension.json",
		"kimi.plugin.json",
		".factory-plugin/plugin.json",
		".hermes/plugins/basemind/plugin.yaml",
		".hermes/plugins/basemind/__init__.py",
	} {
		assert.Contains(t, out, p, "expected manifest %s to be generated", p)
	}
}

func TestGolden_ClaudeHasNoSkillsKeyAndRewritesRoot(t *testing.T) {
	doc := parseJSON(t, generateFixture(t)[".claude-plugin/plugin.json"])
	assert.NotContains(t, doc, "skills", "Claude auto-discovers skills; no skills key")
	mcp := doc["mcpServers"].(map[string]any)["basemind"].(map[string]any)
	assert.Equal(t, "${CLAUDE_PLUGIN_ROOT}/scripts/mcp-launch.sh", mcp["command"])
}

func TestGolden_CodexExternalMCPAndInterface(t *testing.T) {
	out := generateFixture(t)
	doc := parseJSON(t, out[".codex-plugin/plugin.json"])
	assert.Equal(t, "./.mcp.json", doc["mcpServers"], "Codex references root MCP companion file")
	assert.Equal(t, "./skills/", doc["skills"])
	require.Contains(t, doc, "interface")
	iface := doc["interface"].(map[string]any)
	assert.Equal(t, "Code-map MCP server for navigating large codebases", iface["shortDescription"])
	assert.Equal(t, "https://example.com/privacy", iface["privacyPolicyURL"])
	assert.Equal(t, "https://example.com/terms", iface["termsOfServiceURL"])
	assert.Equal(t, "./assets/logo.png", iface["logo"])
	assert.Contains(t, out, "assets/logo.png")

	mcpFile := parseJSON(t, out[".mcp.json"])
	cmd := mcpFile["mcpServers"].(map[string]any)["basemind"].(map[string]any)["command"]
	assert.Equal(t, "./scripts/mcp-launch.sh", cmd)
}

func TestGolden_GeminiContextAndInlineHooks(t *testing.T) {
	doc := parseJSON(t, generateFixture(t)["gemini-extension.json"])
	assert.Equal(t, "GEMINI.md", doc["contextFileName"])
	mcp := doc["mcpServers"].(map[string]any)["basemind"].(map[string]any)
	assert.Equal(t, "${extensionPath}/scripts/mcp-launch.sh", mcp["command"])
	hooks := doc["hooks"].(map[string]any)
	assert.Contains(t, hooks, "SessionStart")
}

func TestGolden_KimiSessionStartAndSkillInstructions(t *testing.T) {
	doc := parseJSON(t, generateFixture(t)["kimi.plugin.json"])
	assert.Equal(t, "./skills/", doc["skills"])
	assert.Equal(t, "basemind", doc["sessionStart"].(map[string]any)["skill"])
	assert.Contains(t, doc["skillInstructions"], "Prefer basemind tools")
	mcp := doc["mcpServers"].(map[string]any)["basemind"].(map[string]any)
	assert.Equal(t, "./scripts/mcp-launch.sh", mcp["command"])
}

func TestGolden_FactoryMetadataOnly(t *testing.T) {
	doc := parseJSON(t, generateFixture(t)[".factory-plugin/plugin.json"])
	assert.Equal(t, "developer-tools", doc["category"])
	assert.NotContains(t, doc, "mcpServers", "Factory is metadata-only")
	assert.NotContains(t, doc, "skills")
}

func TestGolden_CursorHooksUseCursorRootVar(t *testing.T) {
	doc := parseJSON(t, generateFixture(t)[".cursor-plugin/hooks/hooks.json"])
	hooks := doc["hooks"].(map[string]any)
	groups := hooks["SessionStart"].([]any)
	action := groups[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)
	assert.Contains(t, action["command"], "${CURSOR_PLUGIN_ROOT:-.}")
	assert.Equal(t, false, action["async"])
}

func TestGolden_MarketplaceSinglePluginSource(t *testing.T) {
	doc := parseJSON(t, generateFixture(t)[".claude-plugin/marketplace.json"])
	plugins := doc["plugins"].([]any)
	require.Len(t, plugins, 1)
	entry := plugins[0].(map[string]any)
	assert.Equal(t, "./", entry["source"])
	assert.Equal(t, "developer-tools", entry["category"])
	assert.Equal(t, []any{"mcp", "code-intelligence", "rag"}, entry["tags"])
}

func TestGolden_SkillContentBundledVerbatim(t *testing.T) {
	out := generateFixture(t)
	// Claude bundles at root skills/; Cursor and Codex under their plugin dirs.
	for _, p := range []string{
		"skills/basemind/SKILL.md",
		".cursor-plugin/skills/basemind/SKILL.md",
		"skills/basemind/SKILL.md",
	} {
		require.Contains(t, out, p, "expected bundled skill %s", p)
		assert.Contains(t, string(out[p]), "name: basemind", "skill frontmatter passed through verbatim")
	}
	for _, p := range []string{
		"skills/basemind/references/usage.md",
		".cursor-plugin/skills/basemind/references/usage.md",
	} {
		assert.Contains(t, out, p, "expected referenced skill resource %s", p)
	}
}
