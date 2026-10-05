package providers_test

import (
	"testing"

	"github.com/Goldziher/ai-rulez/v5/internal/generator/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReasonix_Generate(t *testing.T) {
	t.Parallel()

	outputs := batchAGenerate(t, "reasonix", batchAConfig())

	for _, path := range []string{
		"REASONIX.md", ".reasonix/skills/demo/SKILL.md", ".reasonix/skills/scout/SKILL.md",
		".reasonix/commands/ship.md", ".mcp.json",
	} {
		_, ok := outputByPath(outputs, path)
		assert.True(t, ok, path)
	}

	// A subagent is a manual-invocation skill that runs as a subagent.
	agent, _ := outputByPath(outputs, ".reasonix/skills/scout/SKILL.md")
	assert.Equal(t, "scout", frontmatterValue(agent.Content, "name"))
	assert.Equal(t, "Fast recon", frontmatterValue(agent.Content, "description"))
	assert.Equal(t, "manual", frontmatterValue(agent.Content, "invocation"))
	assert.Equal(t, "subagent", frontmatterValue(agent.Content, "runAs"))
	assert.Equal(t, "fast-model", frontmatterValue(agent.Content, "model"))
	assert.Equal(t, "high", frontmatterValue(agent.Content, "effort"))

	skill, _ := outputByPath(outputs, ".reasonix/skills/demo/SKILL.md")
	assert.Empty(t, frontmatterValue(skill.Content, "runAs"), "a plain skill is not a subagent")

	command, _ := outputByPath(outputs, ".reasonix/commands/ship.md")
	assert.Equal(t, "Ship the change", frontmatterValue(command.Content, "description"))

	root, _ := outputByPath(outputs, "REASONIX.md")
	assert.Contains(t, root.Content, "TSX_RULE", "Reasonix has no rules folder, so rules inline")
}

func TestReasonix_MCP(t *testing.T) {
	t.Parallel()

	outputs := batchAGenerate(t, "reasonix", batchAConfig())
	mcp, ok := outputByPath(outputs, ".mcp.json")
	require.True(t, ok)
	servers := batchAMCPServers(t, mcp, "mcpServers")
	assert.Equal(t, "npx", servers["local"]["command"])
	assert.Equal(t, "https://x.test/mcp", servers["http"]["url"])
}

func TestReasonix_Global(t *testing.T) {
	t.Parallel()

	want := providers.GlobalPaths{
		RootFile: batchAJoin(".reasonix/REASONIX.md"), SkillsDir: batchAJoin(".reasonix/skills"),
		CommandsDir: batchAJoin(".reasonix/commands"),
		Sidecars:    map[string]string{".reasonix/settings.json": batchAJoin(".reasonix/settings.json")},
	}
	assert.Equal(t, want, batchAGlobal(t, "reasonix", nil))
}
