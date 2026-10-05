package presets

import (
	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/docmerge"
	"github.com/Goldziher/ai-rulez/internal/generator/jsonmerge"
)

const (
	keyURL         = "url"
	keyEnv         = "env"
	keyType        = "type"
	keyHTTPHeaders = "http_headers"
)

// isRemoteServer reports whether the server speaks a remote transport.
func isRemoteServer(server *config.MCPServer) bool {
	t := server.GetTransport()
	return t == config.TransportHTTP || t == config.TransportSSE
}

// VSCodeMCPEntry is the .vscode/mcp.json entry (`servers.<name>`) with type
// stdio|http|sse. VS Code cannot switch a server off, so a disabled server yields
// nil and is left out of the document.
func VSCodeMCPEntry(server *config.MCPServer) map[string]any {
	if server == nil || !server.IsEnabled() {
		return nil
	}
	entry := map[string]any{}
	if t := server.GetTransport(); isRemoteServer(server) {
		entry[keyType] = t
		entry[keyURL] = server.URL
		if len(server.Headers) > 0 {
			entry[keyHeaders] = server.Headers
		}
		return entry
	}
	entry[keyType] = "stdio"
	entry[keyCommand] = server.Command
	if len(server.Args) > 0 {
		entry[keyArgs] = server.Args
	}
	if len(server.Env) > 0 {
		entry[keyEnv] = server.Env
	}
	return entry
}

// CodexMCPEntry is the [mcp_servers.<name>] table of a Codex config.toml:
// command/args/env, or url with http_headers for a remote server, and
// `enabled = false` for a disabled one.
func CodexMCPEntry(server *config.MCPServer) map[string]any {
	if server == nil {
		return nil
	}
	entry := map[string]any{}
	if isRemoteServer(server) {
		entry[keyURL] = server.URL
		if len(server.Headers) > 0 {
			entry[keyHTTPHeaders] = server.Headers
		}
	} else {
		entry[keyCommand] = server.Command
		if len(server.Args) > 0 {
			entry[keyArgs] = server.Args
		}
		if len(server.Env) > 0 {
			entry[keyEnv] = server.Env
		}
	}
	if !server.IsEnabled() {
		entry["enabled"] = false
	}
	return entry
}

// nativeMCPEntry is the shared mcpServers entry (command/args/env or url/headers)
// of the tools that read it natively, without the description they ignore. A
// disabled server yields nil: none of these tools has a switch for it.
func nativeMCPEntry(server *config.MCPServer) map[string]any {
	if server == nil || !server.IsEnabled() {
		return nil
	}
	entry := MCPServerEntry(server)
	delete(entry, keyDescription)
	return entry
}

// mcpEntries renders every complete, expressible server through build, keyed by name.
func mcpEntries(cfg *config.Config, build func(*config.MCPServer) map[string]any) map[string]any {
	entries := map[string]any{}
	if cfg == nil {
		return entries
	}
	for name, server := range cfg.MCPServers {
		if server == nil {
			continue
		}
		if isRemoteServer(server) && server.URL == "" || !isRemoteServer(server) && server.Command == "" {
			continue
		}
		if entry := build(server); entry != nil {
			entries[name] = entry
		}
	}
	return entries
}

// renderMergedMCP merges the servers into the document at path under the key
// path keyPath, leaving every other member alone.
func renderMergedMCP(path string, format docmerge.Format, keyPath []string, entries map[string]any) (jsonmerge.Result, error) {
	return applyMergedDocumentAs(path, format, []jsonmerge.OwnedKey{{Path: keyPath, Value: entries, Members: true}})
}

// mergedOutput is the OutputFile of a merged document render.
func mergedOutput(path string, res jsonmerge.Result) config.OutputFile {
	return config.OutputFile{Path: path, Content: res.Body, PartiallyOwned: res.PartiallyOwned, MergeClaims: res.Claims}
}
