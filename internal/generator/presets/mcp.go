package presets

import (
	"encoding/json"
	"path/filepath"

	"github.com/Goldziher/ai-rulez/internal/config"
)

func init() {
	config.RegisterPresetV3("mcp", &MCPPresetGenerator{})
}

// MCPPresetGenerator generates MCP server configuration
type MCPPresetGenerator struct{}

func (g *MCPPresetGenerator) GetName() string {
	return "mcp"
}

func (g *MCPPresetGenerator) GetOutputPaths(baseDir string) []string {
	return []string{filepath.Join(baseDir, ".mcp.json")}
}

func (g *MCPPresetGenerator) Generate(
	content *config.ContentTreeV3,
	baseDir string,
	cfg *config.ConfigV3,
) ([]config.OutputFileV3, error) {
	// Return empty if no MCP servers
	if len(cfg.MCPServers) == 0 {
		return []config.OutputFileV3{}, nil
	}

	// Build .mcp.json structure
	mcpServers := make(map[string]interface{})

	for name, server := range cfg.MCPServers {
		entry := map[string]interface{}{
			"command":  server.Command,
			"disabled": !server.IsEnabled(),
		}

		if len(server.Args) > 0 {
			entry["args"] = server.Args
		}
		if len(server.Env) > 0 {
			entry["env"] = server.Env
		}
		if server.Transport != "" {
			entry["transport"] = server.GetTransport()
		}
		if server.URL != "" {
			entry["url"] = server.URL
		}

		mcpServers[name] = entry
	}

	payload := map[string]interface{}{
		"mcpServers": mcpServers,
	}

	jsonBytes, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, err
	}

	return []config.OutputFileV3{{
		Path:    filepath.Join(baseDir, ".mcp.json"),
		Content: string(jsonBytes) + "\n",
	}}, nil
}
