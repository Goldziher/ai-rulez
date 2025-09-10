package cli

import (
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/samber/oops"
)

// ClaudeIntegrator handles Claude CLI MCP server configuration
type ClaudeIntegrator struct{}

// ToolName returns the tool name for targeting
func (c *ClaudeIntegrator) ToolName() string {
	return "claude"
}

// IsAvailable checks if Claude CLI is installed and accessible
func (c *ClaudeIntegrator) IsAvailable() bool {
	return isToolAvailable("claude")
}

// ConfigureServer configures an MCP server using Claude CLI
func (c *ClaudeIntegrator) ConfigureServer(server *config.MCPServer) error {
	cmd := []string{"claude", "mcp", "add"}

	// Add scope flag (default to project)
	cmd = append(cmd, "-s", "project")

	// Add transport flag
	transport := server.GetTransport()
	cmd = append(cmd, "-t", transport)

	// Add environment variables using -e flag
	if len(server.Env) > 0 {
		envFlags := buildEnvFlags(server.Env, "-e")
		cmd = append(cmd, envFlags...)
	}

	// Add server name
	cmd = append(cmd, server.Name)

	// Handle different transport types
	switch transport {
	case "stdio":
		if server.Command == "" {
			return oops.
				With("server_name", server.Name).
				With("transport", transport).
				Errorf("command is required for stdio transport")
		}

		// Add command and arguments
		cmd = append(cmd, server.Command)
		cmd = append(cmd, server.Args...)

	case "http", "sse":
		if server.URL == "" {
			return oops.
				With("server_name", server.Name).
				With("transport", transport).
				Errorf("url is required for %s transport", transport)
		}

		// For HTTP/SSE, use URL instead of command
		cmd = append(cmd, server.URL)

	default:
		return oops.
			With("server_name", server.Name).
			With("transport", transport).
			Errorf("unsupported transport type: %s", transport)
	}

	// Execute the command
	if err := executeCommand(cmd); err != nil {
		return oops.
			With("server_name", server.Name).
			With("command", strings.Join(cmd, " ")).
			Wrapf(err, "failed to configure Claude MCP server")
	}

	return nil
}

// RemoveServer removes an MCP server configuration from Claude CLI
func (c *ClaudeIntegrator) RemoveServer(serverName string) error {
	cmd := []string{"claude", "mcp", "remove", serverName}

	if err := executeCommand(cmd); err != nil {
		return oops.
			With("server_name", serverName).
			With("command", strings.Join(cmd, " ")).
			Wrapf(err, "failed to remove Claude MCP server")
	}

	return nil
}

// GetClaudeScope returns the appropriate scope for Claude MCP configuration
func GetClaudeScope(preferProject bool) string {
	if preferProject {
		return "project"
	}
	return "local"
}

// ValidateClaudeConfig validates Claude-specific MCP server configuration
func ValidateClaudeConfig(server *config.MCPServer) error {
	if server.Name == "" {
		return oops.Errorf("server name is required")
	}

	transport := server.GetTransport()
	switch transport {
	case "stdio":
		if server.Command == "" {
			return oops.
				With("server_name", server.Name).
				Errorf("command is required for stdio transport")
		}
	case "http", "sse":
		if server.URL == "" {
			return oops.
				With("server_name", server.Name).
				Errorf("url is required for %s transport", transport)
		}
	default:
		return oops.
			With("transport", transport).
			Errorf("unsupported transport type for Claude: %s", transport)
	}

	return nil
}
