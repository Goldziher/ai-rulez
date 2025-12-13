package importer

import (
	"os"
	"path/filepath"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/logger"
	"github.com/samber/oops"
	"gopkg.in/yaml.v3"
)

// MigrateMCPServers converts V2 MCPServer configurations to V3 format and writes mcp.yaml
// V2 servers may have targets; for now all servers are placed in the root mcp.yaml
func MigrateMCPServers(v2Servers []config.MCPServer, targetDir string) error {
	if len(v2Servers) == 0 {
		logger.Debug("No MCP servers to migrate")
		return nil
	}

	log := logger.Get()
	log.Info("Starting MCP server migration", "count", len(v2Servers))

	// Convert V2 servers to V3 format
	v3Servers := make([]config.MCPServerV3, 0, len(v2Servers))
	for i := range v2Servers {
		v3Server := convertV2ToV3MCPServer(&v2Servers[i])
		v3Servers = append(v3Servers, v3Server)
		log.Debug("Converted MCP server", "name", v3Server.Name)
	}

	// Write mcp.yaml to root .ai-rulez directory
	mcpConfigPath := filepath.Join(targetDir, "mcp.yaml")
	if err := writeMCPConfig(v3Servers, mcpConfigPath); err != nil {
		return oops.
			With("path", mcpConfigPath).
			With("servers_count", len(v3Servers)).
			Wrapf(err, "write MCP configuration")
	}

	log.Info("MCP server migration completed", "path", mcpConfigPath, "count", len(v3Servers))
	return nil
}

// convertV2ToV3MCPServer converts a V2 MCPServer to V3 MCPServerV3 format
func convertV2ToV3MCPServer(v2 *config.MCPServer) config.MCPServerV3 {
	v3 := config.MCPServerV3{
		Name:        v2.Name,
		Description: v2.Description,
		Command:     v2.Command,
		Args:        v2.Args,
		Env:         v2.Env,
		Transport:   v2.Transport,
		URL:         v2.URL,
		Enabled:     v2.Enabled,
	}

	return v3
}

// writeMCPConfig writes the MCP configuration to mcp.yaml
func writeMCPConfig(servers []config.MCPServerV3, path string) error {
	// Create the MCP configuration structure
	mcpConfig := config.MCPConfigV3{
		Schema:  "https://raw.githubusercontent.com/Goldziher/ai-rulez/main/schema/mcp-config.schema.json",
		Version: "1.0",
		Servers: servers,
	}

	// Marshal to YAML
	data, err := yaml.Marshal(mcpConfig)
	if err != nil {
		return oops.
			With("config", mcpConfig).
			Wrapf(err, "marshal MCP config")
	}

	// Write to file
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return oops.
			With("path", path).
			Wrapf(err, "write MCP config file")
	}

	logger.Debug("Wrote MCP configuration file", "path", path, "servers", len(servers))
	return nil
}
