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
	expectedTools := []string{
		// Project tools
		"generate_outputs",
		"validate_config",
		"init_project",
		// Utility tools
		"get_version",
		// Domain tools
		"create_domain",
		"delete_domain",
		"list_domains",
		// Rule tools
		"create_rule",
		"update_rule",
		"delete_rule",
		"list_rules",
		// Context tools
		"create_context",
		"update_context",
		"delete_context",
		"list_context",
		// Skill tools
		"create_skill",
		"update_skill",
		"delete_skill",
		"list_skills",
		// Include tools
		"add_include",
		"remove_include",
		"list_includes",
		// Profile tools
		"add_profile",
		"remove_profile",
		"set_default_profile",
		"list_profiles",
	}

	assert.Equal(t, 26, len(expectedTools), "Should have 26 MCP tools")
}
