package presets

import (
	"regexp"
	"slices"
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

// OAuthJSONEntry returns the camelCase `oauth` block that a JSON MCP document
// (Claude Code's .mcp.json, VS Code's mcp.json) reads: clientId, callbackPort
// and scopes. It returns nil for a server without an oauth block, so a plain
// server renders exactly as before.
func OAuthJSONEntry(o *config.MCPOAuth) map[string]any {
	if o.IsZero() {
		return nil
	}
	entry := map[string]any{}
	if o.ClientID != "" {
		entry["clientId"] = o.ClientID
	}
	if o.CallbackPort != 0 {
		entry["callbackPort"] = o.CallbackPort
	}
	if len(o.Scopes) > 0 {
		entry["scopes"] = o.Scopes
	}
	return entry
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
		if oauth := OAuthJSONEntry(server.OAuth); oauth != nil {
			entry["oauth"] = oauth
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
		codexOAuth(entry, server)
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

// codexOAuth writes the pre-registered OAuth client of a remote server into the
// [mcp_servers.<name>.oauth] table Codex reads (client_id, callback_port), with
// the requested scopes on the server's own `scopes` key.
func codexOAuth(entry map[string]any, server *config.MCPServer) {
	if server.OAuth.IsZero() {
		return
	}
	oauth := map[string]any{}
	if server.OAuth.ClientID != "" {
		oauth["client_id"] = server.OAuth.ClientID
	}
	if server.OAuth.CallbackPort != 0 {
		oauth["callback_port"] = server.OAuth.CallbackPort
	}
	if len(oauth) > 0 {
		entry["oauth"] = oauth
	}
	if len(server.OAuth.Scopes) > 0 {
		entry["scopes"] = server.OAuth.Scopes
	}
}

// codexEnv writes the env of a stdio server: a value that is exactly ${KEY} for
// its own key is forwarded by name (env_vars), the rest is written as env.
func codexEnv(entry map[string]any, server *config.MCPServer) {
	literal := map[string]string{}
	var forwarded []string
	for key, value := range server.Env {
		if m := codexWholeRef.FindStringSubmatch(server.EnvRefs[key]); len(m) > 1 && m[1] == key {
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
// disabled server yields nil: none of these tools has a switch for it. A server
// with an oauth block also yields nil: the shared shape has no oauth, so it is
// left out (see mcpEntries) rather than written without the client it needs.
func nativeMCPEntry(server *config.MCPServer) map[string]any {
	if server == nil || !server.IsEnabled() || server.HasOAuth() {
		return nil
	}
	entry := MCPServerEntry(server)
	delete(entry, keyDescription)
	return entry
}

// selfMCPServer is ai-rulez's own server ([mcp] self_server, on by default) as a
// config entry, so a preset that renders one entry per server writes it exactly
// like a declared [[mcp_servers]] entry. It returns nil when the self server is
// off or the project already declares a server of that name, which stays the
// author's.
func selfMCPServer(cfg *config.Config) *config.MCPServer {
	if cfg == nil || !cfg.HasSelfServer() {
		return nil
	}
	if _, declared := cfg.MCPServers[config.SelfMCPServerName]; declared {
		return nil
	}
	entry := cfg.SelfMCPServerEntry()
	server := &config.MCPServer{Name: config.SelfMCPServerName, Transport: config.TransportStdio}
	server.Command, _ = entry[keyCommand].(string) //nolint:errcheck // a non-string command stays empty
	switch args := entry[keyArgs].(type) {
	case []string:
		server.Args = append([]string(nil), args...)
	case []any:
		for _, arg := range args {
			if text, ok := arg.(string); ok {
				server.Args = append(server.Args, text)
			}
		}
	}
	if env, ok := entry[keyEnv].(map[string]string); ok && len(env) > 0 {
		server.Env = env
	}
	return server
}

// mcpEntries renders every complete, expressible server through build, keyed by
// name. The self server is one of them, so a tool that reads project MCP servers
// gets ai-rulez without the author declaring it a second time. A server carrying
// an oauth block that build cannot express is skipped with a warning rather than
// written without the client it needs.
func mcpEntries(cfg *config.Config, build func(*config.MCPServer) map[string]any) map[string]any {
	entries := map[string]any{}
	if cfg == nil {
		return entries
	}
	add := func(name string, server *config.MCPServer) {
		if server == nil {
			return
		}
		if isRemoteServer(server) && server.URL == "" || !isRemoteServer(server) && server.Command == "" {
			return
		}
		entry := build(server)
		if entry == nil {
			if server.IsEnabled() && server.HasOAuth() {
				cfg.Diag.Warn("skipping MCP server that declares an oauth block: this tool has no oauth block, so a pre-registered client cannot be written", "server", name)
			}
			return
		}
		entries[name] = entry
	}
	for name, server := range cfg.MCPServers {
		add(name, server)
	}
	if self := selfMCPServer(cfg); self != nil {
		add(self.Name, self)
	}
	return entries
}

// renderMergedMCP merges the servers into the document at path under the key
// path keyPath, leaving every other member alone.
func renderMergedMCP(cfg *config.Config, path string, format docmerge.Format, keyPath []string, entries map[string]any) (jsonmerge.Result, error) {
	return applyMergedDocumentAs(cfg, path, format, []jsonmerge.OwnedKey{{Path: keyPath, Value: entries, Members: true}})
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

// bracedEnvRef is ${NAME}, the reference Claude Code, Gemini CLI, Amp and Pi expand.
func bracedEnvRef(name string) string { return "${" + name + "}" }

// BracedMCPEntry is the entry with every ${VAR} placeholder that resolved from the
// process environment written as ${VAR}, for a tool that expands that itself.
func BracedMCPEntry(entry map[string]any, server *config.MCPServer) map[string]any {
	ApplyEnvRefs(entry, server, bracedEnvRef)
	return entry
}

// opencodeEnvRef is {env:NAME}, the substitution OpenCode applies to its config.
func opencodeEnvRef(name string) string { return "{env:" + name + "}" }

// sharedMCPJSONLiteralWriters are the presets writing the root .mcp.json whose tool
// documents no environment expansion. They always render the resolved value (the
// file is owner-only); see SharedMCPJSONRefs for what that means for the others.
var sharedMCPJSONLiteralWriters = []string{"qoder"}

// sharedMCPJSONExpandingPresets are the presets whose tool reads the root
// .mcp.json and expands ${VAR} references in it.
var sharedMCPJSONExpandingPresets = []string{"claude", "codebuddy", "commandcode", "reasonix", "cursor", "copilot"}

var upperEnvName = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

// IsLiteralMCPJSONWriter reports whether the preset writes the root .mcp.json
// with every value resolved because its tool documents no ${VAR} expansion.
func IsLiteralMCPJSONWriter(preset string) bool {
	return slices.Contains(sharedMCPJSONLiteralWriters, preset)
}

// SharedMCPJSONRefs reports whether a writer that expands references writes the
// root .mcp.json with ${VAR} references. It does, except when a literal writer
// (qoder) is the only reader: then the whole file stays resolved, as qoder alone
// could not read a reference. Once a tool that expands references is also active
// it is true again, the literal writer renders differently, and generation fails
// naming both presets instead of putting a resolved secret into a file a
// reference-reading tool shares.
func SharedMCPJSONRefs(cfg *config.Config) bool {
	for _, name := range sharedMCPJSONLiteralWriters {
		if !cfg.HasBuiltInPreset(name) {
			continue
		}
		for _, other := range sharedMCPJSONExpandingPresets {
			if cfg.HasBuiltInPreset(other) {
				return true
			}
		}
		return false
	}
	return true
}

// ApplySharedMCPJSONRefs writes the ${VAR} references of a server into an entry of
// the root .mcp.json (see SharedMCPJSONRefs). CodeBuddy expands upper-case names
// only, so a placeholder naming another variable keeps its resolved value.
func ApplySharedMCPJSONRefs(entry map[string]any, server *config.MCPServer, cfg *config.Config) {
	if entry == nil || server == nil || !SharedMCPJSONRefs(cfg) {
		return
	}
	narrowed := *server
	narrowed.EnvRefs = upperCaseRefs(server.EnvRefs)
	narrowed.HeaderRefs = upperCaseRefs(server.HeaderRefs)
	BracedMCPEntry(entry, &narrowed)
}

// upperCaseRefs keeps the references that name only upper-case variables.
func upperCaseRefs(refs map[string]string) map[string]string {
	kept := make(map[string]string, len(refs))
	for key, raw := range refs {
		ok := true
		for _, m := range envRefPlaceholder.FindAllStringSubmatch(raw, -1) {
			ok = ok && upperEnvName.MatchString(m[1])
		}
		if ok {
			kept[key] = raw
		}
	}
	return kept
}

// devinMCPEntry is the .devin/mcp_config.json entry: the shared native entry with
// ${VAR} placeholders written as ${env:VAR}, which Devin expands itself.
func devinMCPEntry(server *config.MCPServer) map[string]any { return cursorMCPEntry(server) }
