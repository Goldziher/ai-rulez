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
	assert.Contains(t, configContent, "path: \".continue/rules/01-main.md\"")

	assert.Contains(t, configContent, "ai-rules-v2.schema.json")

	assert.Contains(t, configContent, "# agents:")
	assert.Contains(t, configContent, "# rules:")
	assert.Contains(t, configContent, "# sections:")
	assert.Contains(t, configContent, "# commands:")
	assert.Contains(t, configContent, "# mcp_servers:")

	assert.Contains(t, configContent, `#   - name: "architect"`)
	assert.Contains(t, configContent, `#   - name: "swe"`)
	assert.Contains(t, configContent, `#   - name: "reviewer"`)
}

func TestGenerateConfigTemplate_ContinueDevAndClaude(t *testing.T) {
	projectName := "TestProject"
	providers := templates.ProviderConfig{
		ContinueDev: true,
		Claude:      true,
	}

	configContent := templates.GenerateConfigTemplate(projectName, providers)

	assert.Contains(t, configContent, "path: \"CLAUDE.md\"")

	assert.Contains(t, configContent, "# - path: \".cursorrules\"")
	assert.Contains(t, configContent, "# - path: \"GEMINI.md\"")

	assert.Contains(t, configContent, "# agents:")

	assert.Equal(t, 1, strings.Count(configContent, "# agents:"), "There should be only one commented 'agents:' section")
}

func TestGenerateConfigTemplate_OpenCode(t *testing.T) {
	projectName := "TestProject"
	providers := templates.ProviderConfig{
		Opencode: true,
	}

	configContent := templates.GenerateConfigTemplate(projectName, providers)

	assert.Contains(t, configContent, "name: \"TestProject\"")
	assert.Contains(t, configContent, "path: \"AGENTS.md\"")
	assert.Contains(t, configContent, "ai-rules-v2.schema.json")

	assert.Contains(t, configContent, "# agents:")
	assert.Contains(t, configContent, "# rules:")
	assert.Contains(t, configContent, "# sections:")
	assert.Contains(t, configContent, "# commands:")
	assert.Contains(t, configContent, "# mcp_servers:")

	assert.Contains(t, configContent, `#   - name: "architect"`)
	assert.Contains(t, configContent, `#   - name: "swe"`)
	assert.Contains(t, configContent, `#   - name: "reviewer"`)
}

func TestProviderConfig_HasAny_OpenCode(t *testing.T) {
	providers := templates.ProviderConfig{
		Opencode: true,
	}

	assert.True(t, providers.HasAny())
}

func TestProviderConfig_HasAny_OpenCodeAndClaude(t *testing.T) {
	providers := templates.ProviderConfig{
		Opencode: true,
		Claude:   true,
	}

	assert.True(t, providers.HasAny())
}

func TestGenerateConfigTemplate_Junie(t *testing.T) {
	projectName := "TestProject"
	providers := templates.ProviderConfig{
		Junie: true,
	}

	configContent := templates.GenerateConfigTemplate(projectName, providers)

	assert.Contains(t, configContent, "name: \"TestProject\"")
	assert.Contains(t, configContent, "path: \".junie/guidelines.md\"")
	assert.Contains(t, configContent, "ai-rules-v2.schema.json")

	assert.Contains(t, configContent, "# agents:")
	assert.Contains(t, configContent, "# rules:")
	assert.Contains(t, configContent, "# sections:")
	assert.Contains(t, configContent, "# commands:")
	assert.Contains(t, configContent, "# mcp_servers:")
}

func TestProviderConfig_HasAny_Junie(t *testing.T) {
	providers := templates.ProviderConfig{
		Junie: true,
	}

	assert.True(t, providers.HasAny())
}
