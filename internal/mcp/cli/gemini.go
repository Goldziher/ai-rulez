package cli

import (
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/samber/oops"
)

// GeminiIntegrator handles Gemini CLI MCP server configuration
type GeminiIntegrator struct{}

// ToolName returns the tool name for targeting
func (g *GeminiIntegrator) ToolName() string {
	return "gemini"
}

// IsAvailable checks if Gemini CLI is installed and accessible
func (g *GeminiIntegrator) IsAvailable() bool {
	return isToolAvailable("gemini")
}

// ConfigureServer configures an MCP server using Gemini CLI
func (g *GeminiIntegrator) ConfigureServer(server *config.MCPServer) error {
	// Validate configuration first
	if err := ValidateGeminiConfig(server); err != nil {
		return err
	}

	cmd := []string{"gemini", "mcp", "add"}

	// Add server name
	cmd = append(cmd, server.Name)

	// Handle different transport types
	transport := server.GetTransport()
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

	// Note: Gemini CLI doesn't have -e flag for environment variables like Claude
	// Environment variables need to be set in the shell environment before running
	// We proceed assuming env vars are set externally if they exist

	// Execute the command
	if err := executeCommand(cmd); err != nil {
		return oops.
			With("server_name", server.Name).
			With("command", strings.Join(cmd, " ")).
			Wrapf(err, "failed to configure Gemini MCP server")
	}

	return nil
}

// RemoveServer removes an MCP server configuration from Gemini CLI
func (g *GeminiIntegrator) RemoveServer(serverName string) error {
	cmd := []string{"gemini", "mcp", "remove", serverName}

	if err := executeCommand(cmd); err != nil {
		return oops.
			With("server_name", serverName).
			With("command", strings.Join(cmd, " ")).
			Wrapf(err, "failed to remove Gemini MCP server")
	}

	return nil
}

// ValidateGeminiConfig validates Gemini-specific MCP server configuration
func ValidateGeminiConfig(server *config.MCPServer) error {
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
			Errorf("unsupported transport type for Gemini: %s", transport)
	}

	// Note: Gemini doesn't support -e flag for environment variables
	// Environment variables need to be set externally if they exist

	return nil
}
