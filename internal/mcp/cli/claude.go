package cli

import (
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/samber/oops"
)

type ClaudeIntegrator struct{}

func (c *ClaudeIntegrator) ToolName() string {
	return "claude"
}

func (c *ClaudeIntegrator) IsAvailable() bool {
	return isToolAvailable("claude")
}

func (c *ClaudeIntegrator) ConfigureServer(server *config.MCPServer) error {
	cmd := []string{"claude", "mcp", "add"}

	cmd = append(cmd, "-s", "project")

	transport := server.GetTransport()
	cmd = append(cmd, "-t", transport)

	if len(server.Env) > 0 {
		envFlags := buildEnvFlags(server.Env, "-e")
		cmd = append(cmd, envFlags...)
	}

	cmd = append(cmd, server.Name)

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
			Wrapf(err, "failed to configure Claude MCP server")
	}

	return nil
}

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

func GetClaudeScope(preferProject bool) string {
	if preferProject {
		return "project"
	}
	return "local"
}

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
