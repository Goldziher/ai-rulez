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

	assert.Contains(t, configContent, "name: \"TestProject\"")

	assert.Contains(t, configContent, "# Continue.dev prompts (custom commands in YAML format)")
	assert.Contains(t, configContent, "path: \".continue/prompts/ai_rulez_prompts.yaml\"")
	assert.Contains(t, configContent, "type: builtin")
	assert.Contains(t, configContent, "value: continuedev-prompts")
	assert.Contains(t, configContent, "# Continue.dev rules (directory format with frontmatter)")
	assert.Contains(t, configContent, "path: \".continue/rules/\"")

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

	assert.Contains(t, configContent, "# AI agents (specialized sub-assistants for Claude)")
	assert.NotContains(t, configContent, "model:")

	assert.NotContains(t, configContent, "# AI agents (specialized sub-assistants for Continue.dev)")

	assert.Equal(t, 1, strings.Count(configContent, "agents:"), "There should be only one 'agents:' section")
}
