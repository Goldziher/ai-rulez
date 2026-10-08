package conformance

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The MCP server configuration ai-rulez publishes is the mcp.json of an Agent
// Plugins package, checked against the official mcp.schema.json in
// agentplugins_test.go. This test pins the shape of each transport. No schema
// is vendored for an MCP server card yet; docs/standards.md lists it as planned.
func TestMCPServerEntriesUseTheSchemaTransportTypes(t *testing.T) {
	// Arrange
	dir := project(t, map[string]string{
		".ai-rulez/config.toml": `version = "5.0"
name = "conf"
presets = ["claude"]

[plugin]
name = "acme.tools"
version = "1.0.0"
description = "Acme tooling."
runtimes = ["agent-plugins"]

[[mcp_servers]]
name = "tools"
command = "npx"
args = ["-y", "@acme/mcp"]

[[mcp_servers]]
name = "remote"
transport = "http"
url = "https://mcp.acme.example/mcp"
`,
	})

	// Act
	run(t, dir, "generate", "--yes", "--plugin")

	// Assert
	var doc struct {
		Servers map[string]struct {
			Type    string `json:"type"`
			Command string `json:"command"`
			URL     string `json:"url"`
		} `json:"mcpServers"`
	}
	require.NoError(t, json.Unmarshal(read(t, dir, "mcp.json"), &doc))
	assert.Equal(t, "stdio", doc.Servers["tools"].Type)
	assert.Equal(t, "npx", doc.Servers["tools"].Command)
	assert.Equal(t, "streamable-http", doc.Servers["remote"].Type)
	assert.Equal(t, "https://mcp.acme.example/mcp", doc.Servers["remote"].URL)
}
