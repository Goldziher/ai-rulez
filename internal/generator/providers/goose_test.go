package providers_test

import (
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/generator/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGoose_Generate(t *testing.T) {
	t.Parallel()

	outputs := batchAGenerate(t, "goose", batchAConfig())

	for _, path := range []string{
		".goosehints", ".agents/skills/demo/SKILL.md", ".goose/agents/scout.md", ".agents/plugins/ai-rulez/.mcp.json",
	} {
		_, ok := outputByPath(outputs, path)
		assert.True(t, ok, path)
	}
	// Recipes are not slash commands and goose has no rules folder.
	for _, path := range []string{".goose/recipes/ship.yaml", ".goose/rules/tsx.md"} {
		_, ok := outputByPath(outputs, path)
		assert.False(t, ok, path)
	}

	hints, _ := outputByPath(outputs, ".goosehints")
	assert.Contains(t, hints.Content, "TSX_RULE")

	agent, _ := outputByPath(outputs, ".goose/agents/scout.md")
	assert.Equal(t, "scout", frontmatterValue(agent.Content, "name"))
	assert.Equal(t, "Fast recon", frontmatterValue(agent.Content, "description"))
	assert.Equal(t, "fast-model", frontmatterValue(agent.Content, "model"))
	assert.NotContains(t, agent.Content, "tools:", "goose agents take only name, description and model")
}

func TestGoose_MCP(t *testing.T) {
	t.Parallel()

	outputs := batchAGenerate(t, "goose", batchAConfig())
	mcp, ok := outputByPath(outputs, ".agents/plugins/ai-rulez/.mcp.json")
	require.True(t, ok)
	servers := batchAMCPServers(t, mcp, "mcpServers")
	assert.Equal(t, map[string]any{
		"command": "npx", "args": []any{"-y", "pkg"}, "env": map[string]any{"K": "v"},
	}, servers["local"])
	// A remote entry makes goose skip the whole plugin document.
	assert.Len(t, servers, 1, "the plugin .mcp.json is stdio-only")
}

func TestGoose_Global(t *testing.T) {
	t.Parallel()

	want := providers.GlobalPaths{
		RootFile: batchAJoin(".config/goose/.goosehints"), SkillsDir: batchAJoin(".agents/skills"),
		AgentsDir: batchAJoin(".config/goose/agents"),
		Sidecars:  map[string]string{".agents/plugins/ai-rulez/hooks/hooks.json": batchAJoin(".agents/plugins/ai-rulez/hooks/hooks.json")},
	}
	assert.Equal(t, want, batchAGlobal(t, "goose", nil))
}
