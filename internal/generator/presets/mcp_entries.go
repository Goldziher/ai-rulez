package presets

import (
	"regexp"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/docmerge"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
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
// `enabled = false` for a disabled one. A value that came from a ${VAR}
// placeholder is handed over as a reference Codex resolves itself, so the secret
// stays out of the file: a bearer Authorization header is bearer_token_env_var,
// another whole-value header is env_http_headers, and an env value named like the
// variable it reads is forwarded through env_vars.
func CodexMCPEntry(server *config.MCPServer) map[string]any {
	if server == nil {
		return nil
	}
	entry := map[string]any{}
	if isRemoteServer(server) {
		entry[keyURL] = server.URL
		codexHeaders(entry, server)
	} else {
		entry[keyCommand] = server.Command
		if len(server.Args) > 0 {
			entry[keyArgs] = server.Args
		}
		codexEnv(entry, server)
	}
	if !server.IsEnabled() {
		entry["enabled"] = false
	}
	return entry
}

var (
	codexBearerRef = regexp.MustCompile(`^Bearer \$\{([A-Za-z_][A-Za-z0-9_]*)\}$`)
	codexWholeRef  = regexp.MustCompile(`^\$\{([A-Za-z_][A-Za-z0-9_]*)\}$`)
)

// codexHeaders writes the headers of a remote server, as env references where the
// header came from a placeholder Codex can resolve.
func codexHeaders(entry map[string]any, server *config.MCPServer) {
	literal := map[string]string{}
	envHeaders := map[string]string{}
	for name, value := range server.Headers {
		raw, referenced := server.HeaderRefs[name]
		if m := codexBearerRef.FindStringSubmatch(raw); referenced && strings.EqualFold(name, "Authorization") && m != nil {
			entry["bearer_token_env_var"] = m[1]
		} else if m := codexWholeRef.FindStringSubmatch(raw); referenced && m != nil {
			envHeaders[name] = m[1]
		} else {
			literal[name] = value
		}
	}
	if len(literal) > 0 {
		entry[keyHTTPHeaders] = literal
	}
	if len(envHeaders) > 0 {
		entry["env_http_headers"] = envHeaders
	}
}

// codexEnv writes the env of a stdio server: a value that is exactly ${KEY} for
// its own key is forwarded by name (env_vars), the rest is written as env.
func codexEnv(entry map[string]any, server *config.MCPServer) {
	literal := map[string]string{}
	var forwarded []string
	for key, value := range server.Env {
		if m := codexWholeRef.FindStringSubmatch(server.EnvRefs[key]); m != nil && m[1] == key {
			forwarded = append(forwarded, key)
			continue
		}
		literal[key] = value
	}
	if len(literal) > 0 {
		entry[keyEnv] = literal
	}
	if len(forwarded) > 0 {
		sort.Strings(forwarded)
		entry["env_vars"] = forwarded
	}
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

var envRefPlaceholder = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// ApplyEnvRefs rewrites, in the env, environment and headers maps of an entry, the
// values that held a ${VAR} placeholder into the tool's own reference syntax
// (format receives the variable name), so a tool that expands references itself
// is given the reference instead of the resolved secret. The maps are copied: they
// are the server's own.
func ApplyEnvRefs(entry map[string]any, server *config.MCPServer, format func(name string) string) {
	applyEnvRefs(entry, server, format, false)
}

// ApplyWholeEnvRefs is ApplyEnvRefs for a tool whose reference syntax has no
// delimiter ($NAME): only a value that is exactly one placeholder is rewritten,
// since a reference inside a longer string could bind to a different variable
// ("$TOKEN_v2"). Every other value keeps its resolved form.
func ApplyWholeEnvRefs(entry map[string]any, server *config.MCPServer, format func(name string) string) {
	applyEnvRefs(entry, server, format, true)
}

func applyEnvRefs(entry map[string]any, server *config.MCPServer, format func(name string) string, wholeOnly bool) {
	if entry == nil || server == nil || format == nil {
		return
	}
	for key, refs := range map[string]map[string]string{
		"env": server.EnvRefs, "environment": server.EnvRefs, "headers": server.HeaderRefs,
	} {
		current, ok := entry[key].(map[string]string)
		if !ok || len(refs) == 0 {
			continue
		}
		rewritten := make(map[string]string, len(current))
		for k, v := range current {
			if raw, referenced := refs[k]; referenced && (!wholeOnly || codexWholeRef.MatchString(raw)) {
				v = envRefPlaceholder.ReplaceAllStringFunc(raw, func(m string) string { return format(m[2 : len(m)-1]) })
			}
			rewritten[k] = v
		}
		entry[key] = rewritten
	}
}

// cursorMCPEntry is the .cursor/mcp.json entry: the shared native entry with
// ${VAR} placeholders written as ${env:VAR}, which Cursor expands itself.
func cursorMCPEntry(server *config.MCPServer) map[string]any {
	entry := nativeMCPEntry(server)
	ApplyEnvRefs(entry, server, func(name string) string { return "${env:" + name + "}" })
	return entry
}
