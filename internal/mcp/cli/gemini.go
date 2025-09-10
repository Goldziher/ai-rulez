package cli

import (
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/samber/oops"
)

type GeminiIntegrator struct{}

func (g *GeminiIntegrator) ToolName() string {
	return "gemini"
}

func (g *GeminiIntegrator) IsAvailable() bool {
	return isToolAvailable("gemini")
}

func (g *GeminiIntegrator) ConfigureServer(server *config.MCPServer) error {
	if err := ValidateGeminiConfig(server); err != nil {
		return err
	}

	cmd := []string{"gemini", "mcp", "add"}

	cmd = append(cmd, server.Name)

	transport := server.GetTransport()
	switch transport {
	case "stdio":
		if server.Command == "" {
			return oops.
				With("server_name", server.Name).
				With("transport", transport).
				Errorf("command is required for stdio transport")
		}

		cmd = append(cmd, server.Command)
		cmd = append(cmd, server.Args...)

	case "http", "sse":
		if server.URL == "" {
			return oops.
				With("server_name", server.Name).
				With("transport", transport).
				Errorf("url is required for %s transport", transport)
		}

		cmd = append(cmd, server.URL)

	default:
		return oops.
			With("server_name", server.Name).
			With("transport", transport).
			Errorf("unsupported transport type: %s", transport)
	}

	if err := executeCommand(cmd); err != nil {
		return oops.
			With("server_name", server.Name).
			With("command", strings.Join(cmd, " ")).
			Wrapf(err, "failed to configure Gemini MCP server")
	}

	return nil
}

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

	return nil
}
