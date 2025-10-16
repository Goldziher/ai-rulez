package templates

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/config"
)

// RenderSpecialBuiltin handles structured builtin templates that are easier to produce via JSON/YAML marshaling.
// Returns (content, true, nil) when the template name is handled here; otherwise, ("" , false, nil).
func RenderSpecialBuiltin(templateName string, data *TemplateData) (content string, handled bool, err error) {
	switch templateName {
	case "windsurf-mcp":
		content, err = renderWindsurfMCP(data.MCPServers)
		return content, true, err
	case "cline-mcp":
		content, err = renderClineMCP(data.MCPServers)
		return content, true, err
	default:
		return "", false, nil
	}
}

type windsufServerConfig struct {
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	Transport string            `json:"transport,omitempty"`
	URL       string            `json:"url,omitempty"`
	Disabled  bool              `json:"disabled"`
}

func renderWindsurfMCP(servers []config.MCPServer) (string, error) {
	mcpServers := make(map[string]windsufServerConfig, len(servers))

	for i := range servers {
		server := &servers[i]
		name := strings.TrimSpace(server.Name)
		if name == "" {
			continue
		}

		entry := windsufServerConfig{
			Disabled: !server.IsEnabled(),
		}

		transport := server.GetTransport()
		if transport != "" {
			entry.Transport = transport
		}

		if server.Command != "" {
			entry.Command = server.Command
		}
		if len(server.Args) > 0 {
			entry.Args = server.Args
		}
		if len(server.Env) > 0 {
			entry.Env = server.Env
		}

		// HTTP/SSE transports require URL
		if transport == "http" || transport == "sse" {
			entry.URL = server.URL
		}

		mcpServers[name] = entry
	}

	payload := map[string]map[string]windsufServerConfig{
		"mcpServers": mcpServers,
	}

	bytes, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", fmt.Errorf("render windsurf MCP config: %w", err)
	}

	return string(bytes) + "\n", nil
}

type clineServerConfig struct {
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	URL       string            `json:"url,omitempty"`
	Transport string            `json:"transport,omitempty"`
	Disabled  bool              `json:"disabled"`
}

func renderClineMCP(servers []config.MCPServer) (string, error) {
	mcpServers := make(map[string]clineServerConfig, len(servers))

	for i := range servers {
		server := &servers[i]
		name := strings.TrimSpace(server.Name)
		if name == "" {
			continue
		}

		entry := clineServerConfig{
			Disabled: !server.IsEnabled(),
		}

		if server.Command != "" {
			entry.Command = server.Command
		}
		if len(server.Args) > 0 {
			entry.Args = server.Args
		}
		if len(server.Env) > 0 {
			entry.Env = server.Env
		}
		if server.URL != "" {
			entry.URL = server.URL
		}

		transport := server.GetTransport()
		// Explicitly state non-stdio transports; stdio is implied.
		if transport != "" && transport != "stdio" {
			entry.Transport = transport
		}

		mcpServers[name] = entry
	}

	payload := map[string]map[string]clineServerConfig{
		"mcpServers": mcpServers,
	}

	bytes, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", fmt.Errorf("render cline MCP config: %w", err)
	}

	return string(bytes) + "\n", nil
}
