package providers_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/generator/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestZed_Generate(t *testing.T) {
	t.Parallel()

	outputs := batchAGenerate(t, "zed", batchAConfig())

	for _, path := range []string{".rules", ".agents/skills/demo/SKILL.md", ".zed/settings.json"} {
		_, ok := outputByPath(outputs, path)
		assert.True(t, ok, path)
	}
	for _, path := range []string{".agents/agents/scout.md", ".zed/rules/tsx.md"} {
		_, ok := outputByPath(outputs, path)
		assert.False(t, ok, path)
	}

	rules, _ := outputByPath(outputs, ".rules")
	assert.Contains(t, rules.Content, "TSX_RULE")
	assert.Contains(t, rules.Content, "LAYOUT_CTX")
}

// TestZed_MCP pins context_servers: command/args/env or url/headers, no SSE.
func TestZed_MCP(t *testing.T) {
	t.Parallel()

	outputs := batchAGenerate(t, "zed", batchAConfig())
	settings, ok := outputByPath(outputs, ".zed/settings.json")
	require.True(t, ok)
	servers := batchAMCPServers(t, settings, "context_servers")

	assert.Equal(t, map[string]any{
		"command": "npx", "args": []any{"-y", "pkg"}, "env": map[string]any{"K": "v"},
	}, servers["local"])
	assert.Equal(t, map[string]any{
		"url": "https://x.test/mcp", "headers": map[string]any{"Authorization": "Bearer t"},
	}, servers["http"])
	assert.NotContains(t, servers, "sse", "Zed has no SSE transport")
}

// TestZed_MCPMergesIntoJSONC keeps the comments and settings of a hand-written
// settings.json, which Zed reads as JSONC.
func TestZed_MCPMergesIntoJSONC(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	path := filepath.Join(baseDir, ".zed", "settings.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("{\n  // my theme\n  \"theme\": \"One Dark\",\n}\n"), 0o644))

	gen, err := providers.LoadBuiltin("zed")
	require.NoError(t, err)
	outputs, err := gen.Generate(batchAContent(), baseDir, batchAConfig())
	require.NoError(t, err)

	settings, ok := outputByPath(outputs, ".zed/settings.json")
	require.True(t, ok)
	assert.Contains(t, settings.Content, "// my theme")
	assert.Contains(t, settings.Content, `"theme": "One Dark"`)
	assert.Contains(t, settings.Content, `"context_servers"`)
}

func TestZed_Global(t *testing.T) {
	t.Parallel()

	want := providers.GlobalPaths{
		RootFile: batchAJoin(".config/zed/AGENTS.md"), SkillsDir: batchAJoin(".agents/skills"),
		Sidecars: map[string]string{".zed/settings.json": batchAJoin(".config/zed/settings.json")},
	}
	assert.Equal(t, want, batchAGlobal(t, "zed", nil))
}
