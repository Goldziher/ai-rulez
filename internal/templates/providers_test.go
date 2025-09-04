package templates_test

import (
	"strings"
	"testing"

	"github.com/Goldziher/ai-rulez/internal/templates"
	"github.com/stretchr/testify/assert"
)

func TestGenerateConfigTemplate_ContinueDev(t *testing.T) {
	projectName := "TestProject"
	providers := templates.ProviderConfig{
		ContinueDev: true,
	}

	configContent := templates.GenerateConfigTemplate(projectName, providers)

	// Check for metadata
	assert.Contains(t, configContent, "name: \"TestProject\"")

	// Check for Continue.dev outputs
	assert.Contains(t, configContent, "# Continue.dev prompts (custom commands in YAML format)")
	assert.Contains(t, configContent, "path: \".continue/prompts/ai_rulez_prompts.yaml\"")
	assert.Contains(t, configContent, "template: \"continuedev-prompts\"")
	assert.Contains(t, configContent, "# Continue.dev rules (directory format with frontmatter)")
	assert.Contains(t, configContent, "path: \".continue/rules/\"")

	// Check for sample agents section
	assert.Contains(t, configContent, "# AI agents (specialized sub-assistants for Continue.dev)")
	assert.Contains(t, configContent, "agents:")
	assert.Contains(t, configContent, "name: \"code-reviewer\"")
}

func TestGenerateConfigTemplate_ContinueDevAndClaude(t *testing.T) {
	projectName := "TestProject"
	providers := templates.ProviderConfig{
		ContinueDev: true,
		Claude:      true,
	}

	configContent := templates.GenerateConfigTemplate(projectName, providers)

	// Check that Claude's agent config takes precedence
	assert.Contains(t, configContent, "# AI agents (specialized sub-assistants for Claude)")
	// Claude Code doesn't support per-agent model specification, so it shouldn't be present
	assert.NotContains(t, configContent, "model:")

	// Check that Continue.dev's agent config is not present
	assert.NotContains(t, configContent, "# AI agents (specialized sub-assistants for Continue.dev)")

	// Count occurrences of "agents:"
	assert.Equal(t, 1, strings.Count(configContent, "agents:"), "There should be only one 'agents:' section")
}
