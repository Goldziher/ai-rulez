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
	assert.Contains(t, configContent, "# Continue.dev rules (directory format with frontmatter)")
	assert.Contains(t, configContent, "path: \".continue/rules/\"")
	assert.Contains(t, configContent, "# Continue.dev agents (imported into config.py)")
	assert.Contains(t, configContent, "path: \".continue/ai_rulez_agents.py\"")
	assert.Contains(t, configContent, "template: \"continuedev-agents\"")

	// Check for sample agents section
	assert.Contains(t, configContent, "# AI agents (specialized sub-assistants for Continue.dev)")
	assert.Contains(t, configContent, "agents:")
	assert.Contains(t, configContent, "name: \"code-reviewer\"")
	assert.Contains(t, configContent, "model: \"gpt-4-turbo\"")
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
	assert.Contains(t, configContent, "model: \"claude-3-opus-20240229\"")

	// Check that Continue.dev's agent config is not present
	assert.NotContains(t, configContent, "# AI agents (specialized sub-assistants for Continue.dev)")

	// Count occurrences of "agents:"
	assert.Equal(t, 1, strings.Count(configContent, "agents:"), "There should be only one 'agents:' section")
}
