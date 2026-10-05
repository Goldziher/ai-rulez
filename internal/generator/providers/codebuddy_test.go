package providers_test

import (
	"testing"

	"github.com/Goldziher/ai-rulez/internal/generator/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCodebuddy_Generate(t *testing.T) {
	t.Parallel()

	outputs := batchAGenerate(t, "codebuddy", batchAConfig())

	for _, path := range []string{
		"CODEBUDDY.md", ".codebuddy/rules/be-nice.md", ".codebuddy/rules/tsx.md", ".codebuddy/skills/demo/SKILL.md",
		".codebuddy/agents/scout.md", ".codebuddy/commands/ship.md", ".mcp.json",
	} {
		_, ok := outputByPath(outputs, path)
		assert.True(t, ok, path)
	}

	always, _ := outputByPath(outputs, ".codebuddy/rules/be-nice.md")
	assert.Equal(t, "true", frontmatterValue(always.Content, "alwaysApply"))
	assert.Empty(t, frontmatterValue(always.Content, "paths"))

	scoped, _ := outputByPath(outputs, ".codebuddy/rules/tsx.md")
	assert.Equal(t, "false", frontmatterValue(scoped.Content, "alwaysApply"))
	assert.Equal(t, "src/**/*.tsx,*.ts", frontmatterValue(scoped.Content, "paths"))

	agent, _ := outputByPath(outputs, ".codebuddy/agents/scout.md")
	assert.Equal(t, "scout", frontmatterValue(agent.Content, "name"))
	assert.Equal(t, "Fast recon", frontmatterValue(agent.Content, "description"))
	assert.Equal(t, "fast-model", frontmatterValue(agent.Content, "model"))
	assert.Contains(t, agent.Content, "tools:\n    - read\n    - grep\n")

	command, _ := outputByPath(outputs, ".codebuddy/commands/ship.md")
	assert.Equal(t, "Ship the change", frontmatterValue(command.Content, "description"))
	assert.Empty(t, frontmatterValue(command.Content, "name"))

	root, _ := outputByPath(outputs, "CODEBUDDY.md")
	assert.Contains(t, root.Content, "LAYOUT_CTX")
}

// TestCodebuddy_MCP pins the shared project-root .mcp.json that CodeBuddy reads.
func TestCodebuddy_MCP(t *testing.T) {
	t.Parallel()

	outputs := batchAGenerate(t, "codebuddy", batchAConfig())
	mcp, ok := outputByPath(outputs, ".mcp.json")
	require.True(t, ok)
	servers := batchAMCPServers(t, mcp, "mcpServers")

	assert.Equal(t, "npx", servers["local"]["command"])
	assert.Equal(t, "http", servers["http"]["type"])
	assert.Equal(t, "https://x.test/mcp", servers["http"]["url"])
	assert.Equal(t, "sse", servers["sse"]["type"])

	cfg := batchAConfig()
	cfg.MCPServers = nil
	_, ok = outputByPath(batchAGenerate(t, "codebuddy", cfg), ".mcp.json")
	assert.False(t, ok)
}

func TestCodebuddy_Global(t *testing.T) {
	t.Parallel()

	want := providers.GlobalPaths{
		RootFile: batchAJoin(".codebuddy/CODEBUDDY.md"), SkillsDir: batchAJoin(".codebuddy/skills"),
		AgentsDir: batchAJoin(".codebuddy/agents"), CommandsDir: batchAJoin(".codebuddy/commands"),
		RulesDir: batchAJoin(".codebuddy/rules"), Sidecars: map[string]string{},
	}
	assert.Equal(t, want, batchAGlobal(t, "codebuddy", nil))
}
