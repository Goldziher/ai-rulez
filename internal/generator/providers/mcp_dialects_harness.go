package providers

import (
	"github.com/Goldziher/ai-rulez/v5/internal/config"
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
	// MCPDialectRoo is the Roo Code family's mcpServers shape (Zoo Code): command/
	// args/env for a local server, `type` streamable-http or sse plus url/headers
	// for a remote one, and `disabled: true` for a disabled server.
	MCPDialectRoo = "roo"
	// MCPDialectCodewhale is CodeWhale's servers shape: command/args/env or
	// url/headers, `transport: "sse"` for a legacy SSE endpoint (a bare url is
	// streamable HTTP), and `disabled: true` for a disabled server.
	MCPDialectCodewhale = "codewhale"
)

// codewhaleMCPEntry is the .codewhale/mcp.json entry.
func codewhaleMCPEntry(server *config.MCPServer) map[string]any {
	entry := standardMCPEntry(server)
	if entry == nil {
		return nil
	}
	delete(entry, "description")
	if server.GetTransport() == config.TransportSSE {
		entry["transport"] = "sse"
	}
	return entry
}

// rooMCPEntry is the .roo/mcp.json entry. Roo needs `type` on every remote
// server: a typeless url entry is not read as streamable HTTP.
func rooMCPEntry(server *config.MCPServer) map[string]any {
	entry := standardMCPEntry(server)
	if entry == nil {
		return nil
	}
	delete(entry, "description")
	switch server.GetTransport() {
	case config.TransportHTTP:
		entry["type"] = "streamable-http"
	case config.TransportSSE:
		entry["type"] = "sse"
	}
	return entry
}

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
