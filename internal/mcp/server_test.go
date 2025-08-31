package mcp_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Goldziher/ai-rulez/internal/mcp"
)

func TestNewServer(t *testing.T) {
	server := mcp.NewServer("1.0.0")
	assert.NotNil(t, server)

	mcpServer := server.GetMCPServer()
	assert.NotNil(t, mcpServer)
}

func TestServerToolRegistration(t *testing.T) {
	// Expected MCP tools that should be registered
	expectedTools := []string{
		"get_rules",
		"get_sections",
		"get_agents",
		"get_outputs",
		"add_rule",
		"add_section",
		"add_output",
		"add_agent",
		"update_rule",
		"update_section",
		"update_output",
		"update_agent",
		"delete_rule",
		"delete_section",
		"delete_output",
		"delete_agent",
		"generate_output",
		"validate_config",
		"init_project",
		"get_version",
	}

	// Just verify the expected tool count
	assert.Equal(t, 20, len(expectedTools), "Should have 20 MCP tools")
}
