package providers_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReplit_Generate(t *testing.T) {
	t.Parallel()

	outputs := batchAGenerate(t, "replit", batchAConfig())

	for _, path := range []string{"replit.md", ".agents/skills/demo/SKILL.md"} {
		_, ok := outputByPath(outputs, path)
		assert.True(t, ok, path)
	}
	// Replit has no rules folder, subagent, command or file-based MCP format.
	for _, path := range []string{".mcp.json", ".replit/mcp.json", ".agents/agents/scout.md", "replit.local.md"} {
		_, ok := outputByPath(outputs, path)
		assert.False(t, ok, path)
	}

	root, _ := outputByPath(outputs, "replit.md")
	assert.Contains(t, root.Content, "TSX_RULE")
	assert.Contains(t, root.Content, "LAYOUT_CTX")

	skill, _ := outputByPath(outputs, ".agents/skills/demo/SKILL.md")
	assert.Equal(t, "demo", frontmatterValue(skill.Content, "name"))
}
