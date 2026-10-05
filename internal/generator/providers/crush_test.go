package providers_test

import (
	"testing"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCrush_Generate(t *testing.T) {
	t.Parallel()

	outputs := batchAGenerate(t, "crush", batchAConfig())

	for _, path := range []string{"CRUSH.md", ".crush/skills/demo/SKILL.md", "crush.json"} {
		_, ok := outputByPath(outputs, path)
		assert.True(t, ok, path)
	}
	// Crush has no rules folder, subagent or command format.
	for _, path := range []string{".crush/agents/scout.md", ".crush/commands/ship.md", ".crush/rules/tsx.md"} {
		_, ok := outputByPath(outputs, path)
		assert.False(t, ok, path)
	}

	root, _ := outputByPath(outputs, "CRUSH.md")
	assert.Contains(t, root.Content, "TSX_RULE", "Crush has no rules folder, so rules inline")
	assert.Contains(t, root.Content, "LAYOUT_CTX")
}

// TestCrush_MCP pins the typed entries under the `mcp` key of crush.json.
func TestCrush_MCP(t *testing.T) {
	t.Parallel()

	cfg := batchAConfig()
	off := false
	cfg.MCPServers["off"] = &config.MCPServer{Name: "off", Command: "off-cmd", Enabled: &off}

	outputs := batchAGenerate(t, "crush", cfg)
	crush, ok := outputByPath(outputs, "crush.json")
	require.True(t, ok)
	servers := batchAMCPServers(t, crush, "mcp")

	assert.Equal(t, map[string]any{
		"type": "stdio", "command": "npx", "args": []any{"-y", "pkg"}, "env": map[string]any{"K": "v"},
	}, servers["local"])
	assert.Equal(t, map[string]any{
		"type": "http", "url": "https://x.test/mcp", "headers": map[string]any{"Authorization": "Bearer t"},
	}, servers["http"])
	assert.Equal(t, map[string]any{"type": "sse", "url": "https://x.test/sse"}, servers["sse"])
	assert.NotContains(t, servers, "off", "a disabled server is left out")
}

func TestCrush_Global(t *testing.T) {
	t.Parallel()

	want := providers.GlobalPaths{
		RootFile: batchAJoin(".config/crush/CRUSH.md"), SkillsDir: batchAJoin(".config/crush/skills"),
		Sidecars: map[string]string{"crush.json": batchAJoin(".config/crush/crush.json")},
	}
	assert.Equal(t, want, batchAGlobal(t, "crush", nil))
}
