package plugin

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testConfig() *config.Config {
	return &config.Config{
		Name:    "basemind",
		Version: "4.0",
		BaseDir: "/tmp/out",
		Plugin: &config.PluginAuthoring{
			Name:        "basemind",
			DisplayName: "Basemind",
			Description: "Full AI context layer over MCP.",
			Version:     "0.19.2",
			Author:      &config.Author{Name: "Na'aman Hirschfeld", Email: "nhirschfeld@gmail.com"},
			Homepage:    "https://github.com/Goldziher/basemind",
			Repository:  "https://github.com/Goldziher/basemind",
			License:     "MIT",
			Keywords:    []string{"mcp", "rag"},
			Runtimes:    []string{"claude"},
			MCP: []config.PluginMCPLaunch{
				{Name: "basemind", Command: "${PLUGIN_ROOT}/scripts/mcp-launch.sh", Args: []string{"serve"}},
			},
		},
	}
}

// outputByPath indexes outputs by their path suffix for assertions.
func outputByPath(t *testing.T, outputs []config.OutputFile, suffix string) config.OutputFile {
	t.Helper()
	for _, o := range outputs {
		if filepath.ToSlash(o.Path) == suffix || hasSuffix(filepath.ToSlash(o.Path), suffix) {
			return o
		}
	}
	t.Fatalf("no output matching %q; got %d outputs", suffix, len(outputs))
	return config.OutputFile{}
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func TestGenerate_ClaudeManifest(t *testing.T) {
	cfg := testConfig()
	m, err := BuildManifest(cfg, nil)
	require.NoError(t, err)

	outputs, err := Generate(m, cfg.BaseDir)
	require.NoError(t, err)

	claudeOut := outputByPath(t, outputs, ".claude-plugin/plugin.json")
	require.NotNil(t, claudeOut.RawContent)

	var doc map[string]any
	require.NoError(t, json.Unmarshal(claudeOut.RawContent, &doc))

	assert.Equal(t, "basemind", doc["name"])
	assert.Equal(t, "Basemind", doc["displayName"])
	assert.Equal(t, "0.19.2", doc["version"])
	assert.Equal(t, "MIT", doc["license"])

	mcp, ok := doc["mcpServers"].(map[string]any)
	require.True(t, ok, "mcpServers should be present")
	bm, ok := mcp["basemind"].(map[string]any)
	require.True(t, ok)
	// Claude rewrites ${PLUGIN_ROOT} -> ${CLAUDE_PLUGIN_ROOT}.
	assert.Equal(t, "${CLAUDE_PLUGIN_ROOT}/scripts/mcp-launch.sh", bm["command"])
}

func TestGenerate_SingleMarketplace(t *testing.T) {
	cfg := testConfig()
	m, err := BuildManifest(cfg, nil)
	require.NoError(t, err)

	outputs, err := Generate(m, cfg.BaseDir)
	require.NoError(t, err)

	mktOut := outputByPath(t, outputs, ".claude-plugin/marketplace.json")
	var doc map[string]any
	require.NoError(t, json.Unmarshal(mktOut.RawContent, &doc))

	assert.Equal(t, "basemind", doc["name"])
	plugins, ok := doc["plugins"].([]any)
	require.True(t, ok)
	require.Len(t, plugins, 1)
	entry := plugins[0].(map[string]any)
	assert.Equal(t, "./", entry["source"])
	assert.Equal(t, "basemind", entry["name"])
	assert.Equal(t, "0.19.2", entry["version"])
}

func TestMCPServersFor_RemoteTransport(t *testing.T) {
	m := &Manifest{MCP: []config.PluginMCPLaunch{
		{Name: "remote", Transport: "http", URL: "https://example.com/mcp"},
		{Name: "local", Command: "${PLUGIN_ROOT}/run.sh", Args: []string{"serve"}},
	}}

	servers := mcpServersFor(m, config.PluginRuntimeClaude)

	// Remote server: type + url, no command (a command-less stdio entry is invalid).
	remote := servers["remote"]
	assert.Equal(t, "http", remote.Type)
	assert.Equal(t, "https://example.com/mcp", remote.URL)
	assert.Empty(t, remote.Command)

	// Local server: command rewritten, no type/url.
	local := servers["local"]
	assert.Equal(t, "${CLAUDE_PLUGIN_ROOT}/run.sh", local.Command)
	assert.Equal(t, []string{"serve"}, local.Args)
	assert.Empty(t, local.Type)
}

func TestRewriteRoot(t *testing.T) {
	const cmd = "${PLUGIN_ROOT}/scripts/mcp-launch.sh"
	assert.Equal(t, "${CLAUDE_PLUGIN_ROOT}/scripts/mcp-launch.sh", rewriteRoot(cmd, config.PluginRuntimeClaude))
	assert.Equal(t, "${extensionPath}/scripts/mcp-launch.sh", rewriteRoot(cmd, config.PluginRuntimeGemini))
	assert.Equal(t, "./scripts/mcp-launch.sh", rewriteRoot(cmd, config.PluginRuntimeCursor))
	assert.Equal(t, "./scripts/mcp-launch.sh", rewriteRoot(cmd, config.PluginRuntimeKimi))
	assert.Equal(t, "./scripts/mcp-launch.sh", rewriteRoot(cmd, config.PluginRuntimeCodex))
}
