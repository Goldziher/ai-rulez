package plugin

import (
	"path/filepath"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

func init() {
	register(config.PluginRuntimeAgentPlugins, renderAgentPlugins)
}

// Agent Plugins 1.0.0 canonical schema identifiers. The standard shares one
// version between the plugin manifest and the MCP configuration.
const (
	agentPluginsVersion   = "1.0.0"
	agentPluginsSchema    = "https://agent-plugins.org/schemas/" + agentPluginsVersion + "/plugin.schema.json"
	agentPluginsMCPSchema = "https://agent-plugins.org/schemas/" + agentPluginsVersion + "/mcp.schema.json"
)

// agentPluginsManifest is the closed schema of the root plugin.json. Only the
// fields permitted by the standard are emitted.
type agentPluginsManifest struct {
	Schema      string         `json:"$schema"`
	Name        string         `json:"name"`
	Version     string         `json:"version,omitempty"`
	Description string         `json:"description,omitempty"`
	Author      *config.Author `json:"author,omitempty"`
	Homepage    string         `json:"homepage,omitempty"`
	Repository  string         `json:"repository,omitempty"`
	License     string         `json:"license,omitempty"`
	Keywords    []string       `json:"keywords,omitempty"`
	// Extensions carries client-specific manifest data keyed by reverse-domain
	// namespace. ai-rulez writes only com.openai (the Codex interface block).
	Extensions map[string]any `json:"extensions,omitempty"`
}

// agentPluginsMCPDoc is the root mcp.json shape: exactly $schema + mcpServers.
type agentPluginsMCPDoc struct {
	Schema     string                           `json:"$schema"`
	MCPServers map[string]agentPluginsMCPServer `json:"mcpServers"`
}

// agentPluginsMCPServer is one server entry. The standard's server schema is a
// closed union: stdio carries command/args/env/cwd; remote carries url/headers.
type agentPluginsMCPServer struct {
	Type    string            `json:"type"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"`
}

// renderAgentPlugins emits the portable Agent Plugins package: a root
// plugin.json, the fixed skills/ directory, and (when configured) mcp.json.
// Commands, agents, hooks, and marketplaces are outside Agent Plugins v1.
func renderAgentPlugins(m *Manifest, baseDir string) ([]config.OutputFile, error) {
	return agentPluginsCore(m, baseDir)
}

// buildAgentPluginsManifest is the one root plugin.json every runtime built on
// the standard (agent-plugins, copilot, codex in root layout) writes, so the
// runtimes can share the file. The Codex interface block is added under
// extensions.com.openai exactly when the Codex root layout is active.
func buildAgentPluginsManifest(m *Manifest) agentPluginsManifest {
	doc := agentPluginsManifest{
		Schema:      agentPluginsSchema,
		Name:        m.Name,
		Version:     m.Version,
		Description: m.Description,
		Author:      m.Author,
		Homepage:    m.Homepage,
		Repository:  m.Repository,
		License:     m.License,
		Keywords:    m.Keywords,
	}
	if codexRootLayout(m) {
		if iface := buildInterface(m.Interface); iface != nil {
			doc.Extensions = map[string]any{"com.openai": map[string]any{"interface": iface}}
		}
	}
	return doc
}

// agentPluginsCore writes the root plugin.json, skills/ and mcp.json.
func agentPluginsCore(m *Manifest, baseDir string) ([]config.OutputFile, error) {
	manifest, err := jsonOutput(filepath.Join(baseDir, "plugin.json"), buildAgentPluginsManifest(m))
	if err != nil {
		return nil, err
	}
	outputs := []config.OutputFile{manifest}

	content, err := bundleContent(m, baseDir, contentLayout{Skills: true})
	if err != nil {
		return nil, err
	}
	outputs = append(outputs, content...)

	if servers := agentPluginsMCPServers(m); servers != nil {
		mcpFile, err := jsonOutput(filepath.Join(baseDir, "mcp.json"), agentPluginsMCPDoc{
			Schema:     agentPluginsMCPSchema,
			MCPServers: servers,
		})
		if err != nil {
			return nil, err
		}
		outputs = append(outputs, mcpFile)
	}

	return outputs, nil
}

// agentPluginsMCPServers maps the plugin's MCP servers onto the standard's
// closed variant set. stdio commands are rewritten to plugin-relative paths
// (the standard forbids placeholders in `command`); remote transports map
// http → streamable-http and keep sse. Returns nil when there are no servers.
func agentPluginsMCPServers(m *Manifest) map[string]agentPluginsMCPServer {
	if len(m.MCP) == 0 {
		return nil
	}
	servers := make(map[string]agentPluginsMCPServer, len(m.MCP))
	for _, s := range m.MCP {
		switch s.Transport {
		case config.TransportHTTP:
			servers[s.Name] = agentPluginsMCPServer{Type: "streamable-http", URL: s.URL}
		case config.TransportSSE:
			servers[s.Name] = agentPluginsMCPServer{Type: "sse", URL: s.URL}
		default:
			servers[s.Name] = agentPluginsMCPServer{
				Type:    "stdio",
				Command: agentPluginsCommand(s.Command),
				Args:    s.Args,
				Env:     s.Env,
			}
		}
	}
	return servers
}

// agentPluginsCommand rewrites a ${PLUGIN_ROOT}-rooted command to the
// plugin-relative "./..." form the standard requires for bundled executables.
// A bare executable name is passed through unchanged.
func agentPluginsCommand(command string) string {
	return rewriteRoot(command, config.PluginRuntimeAgentPlugins)
}
