package providers

import (
	"github.com/Goldziher/ai-rulez/internal/config"
)

const (
	// MCPDialectBob is IBM Bob's mcpServers shape: command/args/env for a local
	// server, `type: "streamable-http"` plus url/headers for an HTTP one, a bare
	// url for legacy SSE, and `disabled: true` for a disabled server.
	MCPDialectBob = "bob"
	// MCPDialectZcode is ZCode's mcp.servers shape: command/args/env, or
	// `type` (http|sse) with url/headers, and `enable: false` for a disabled
	// server.
	MCPDialectZcode = "zcode"
	// MCPDialectGrok is Grok Build's [mcp_servers.<name>] shape: command/args/env
	// or url/headers, and `enabled = false` for a disabled server.
	MCPDialectGrok = "grok"
)

// bobMCPEntry is the .bob/mcp.json entry.
func bobMCPEntry(server *config.MCPServer) map[string]any {
	entry := standardMCPEntry(server)
	if entry == nil {
		return nil
	}
	delete(entry, "description")
	if server.GetTransport() == config.TransportHTTP {
		entry["type"] = "streamable-http"
	}
	return entry
}

// zcodeMCPEntry is the entry under mcp.servers of .zcode/config.json.
func zcodeMCPEntry(server *config.MCPServer) map[string]any {
	entry := claudeMCPEntry(server)
	delete(entry, "disabled")
	if !server.IsEnabled() {
		entry["enable"] = false
	}
	return entry
}

// grokMCPEntry is the [mcp_servers.<name>] table of .grok/config.toml.
func grokMCPEntry(server *config.MCPServer) map[string]any {
	entry := standardMCPEntry(server)
	if entry == nil {
		return nil
	}
	delete(entry, "description")
	delete(entry, "disabled")
	if !server.IsEnabled() {
		entry["enabled"] = false
	}
	return entry
}
