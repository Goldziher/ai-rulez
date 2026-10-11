package plugin

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testConfig() *config.Config {
	return &config.Config{
		Name:    "basemind",
		Version: "5.0",
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
	assert.NotContains(t, doc, "displayName", "the Claude Code plugin schema has no displayName; validate flags it as AR963")
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

func TestMCPServersFor_RewritesRootInCommandArgsAndEnv(t *testing.T) {
	tests := []struct {
		name        string
		runtime     string
		wantCommand string
		wantArg     string
		wantEnv     string
	}{
		{"claude", config.PluginRuntimeClaude, "${CLAUDE_PLUGIN_ROOT}/run.sh", "${CLAUDE_PLUGIN_ROOT}/lib", "${CLAUDE_PLUGIN_ROOT}/cfg"},
		{"gemini", config.PluginRuntimeGemini, "${extensionPath}/run.sh", "${extensionPath}/lib", "${extensionPath}/cfg"},
		{"cursor", config.PluginRuntimeCursor, "./run.sh", "./lib", "./cfg"},
		{"kimi", config.PluginRuntimeKimi, "./run.sh", "./lib", "./cfg"},
		{"codex", config.PluginRuntimeCodex, "./run.sh", "./lib", "./cfg"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Manifest{MCP: []config.PluginMCPLaunch{{
				Name:    "srv",
				Command: "${PLUGIN_ROOT}/run.sh",
				Args:    []string{"${PLUGIN_ROOT}/lib", "plain"},
				Env:     map[string]string{"ROOT": "${PLUGIN_ROOT}/cfg", "MODE": "ci"},
			}}}

			srv := mcpServersFor(m, tt.runtime)["srv"]

			assert.Equal(t, tt.wantCommand, srv.Command)
			assert.Equal(t, []string{tt.wantArg, "plain"}, srv.Args)
			assert.Equal(t, map[string]string{"ROOT": tt.wantEnv, "MODE": "ci"}, srv.Env)
		})
	}
}

func TestMCPServersFor_LeavesValuesWithoutPlaceholderUntouched(t *testing.T) {
	m := &Manifest{MCP: []config.PluginMCPLaunch{{
		Name:    "srv",
		Command: "uvx",
		Args:    []string{"--port", "8080"},
		Env:     map[string]string{"MODE": "ci"},
	}}}

	srv := mcpServersFor(m, config.PluginRuntimeClaude)["srv"]

	assert.Equal(t, "uvx", srv.Command)
	assert.Equal(t, []string{"--port", "8080"}, srv.Args)
	assert.Equal(t, map[string]string{"MODE": "ci"}, srv.Env)
}

func TestRewriteRoot(t *testing.T) {
	const cmd = "${PLUGIN_ROOT}/scripts/mcp-launch.sh"
	assert.Equal(t, "${CLAUDE_PLUGIN_ROOT}/scripts/mcp-launch.sh", rewriteRoot(cmd, config.PluginRuntimeClaude))
	assert.Equal(t, "${extensionPath}/scripts/mcp-launch.sh", rewriteRoot(cmd, config.PluginRuntimeGemini))
	assert.Equal(t, "./scripts/mcp-launch.sh", rewriteRoot(cmd, config.PluginRuntimeCursor))
	assert.Equal(t, "./scripts/mcp-launch.sh", rewriteRoot(cmd, config.PluginRuntimeKimi))
	assert.Equal(t, "./scripts/mcp-launch.sh", rewriteRoot(cmd, config.PluginRuntimeCodex))
}
