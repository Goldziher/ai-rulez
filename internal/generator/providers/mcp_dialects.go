package providers

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/presets"
)

// MCP entry dialects of a generic "mcp" sidecar. Each is the per-server shape a
// tool reads; the dialect also fixes the default owned key (see mcpDialects).
//
// Server values are read from cfg.MCPServers after placeholder resolution
// (internal/generator/mcp_env.go), so a dialect never touches ${VAR} syntax, and
// the generator's sensitive-output scan sees the same resolved values it sees for
// every other MCP document.
const (
	// MCPDialectStandard is the shared mcpServers shape: command/args/env for a
	// local server, url/headers for a remote one.
	MCPDialectStandard = "standard"
	// MCPDialectClaude keys remote transport on `type` (http or sse).
	MCPDialectClaude = "claude"
	// MCPDialectGemini keys remote transport on the URL field: httpUrl or url.
	MCPDialectGemini = "gemini"
	// MCPDialectOpencode is mcp.<name> with type local|remote, a command array,
	// `environment` and `enabled`.
	MCPDialectOpencode = "opencode"
	// MCPDialectVSCode is servers.<name> with type stdio|http|sse.
	MCPDialectVSCode = "vscode"
	// MCPDialectZed is context_servers.<name>: command/args/env or url/headers.
	MCPDialectZed = "zed"
	// MCPDialectCodex is [mcp_servers.<name>]: command/args/env or url with
	// http_headers, `enabled = false` for a disabled server.
	MCPDialectCodex = "codex"
	// MCPDialectAmp is the standard shape under the flat key "amp.mcpServers".
	MCPDialectAmp = "amp"
	// MCPDialectYAMLStandard is Poolside's mcp_servers: command/args/env, and a
	// transport block (type, url, "Name: value" header list) for a remote server.
	MCPDialectYAMLStandard = "yaml-standard"
)

// mcpDialect couples an entry builder with the member the entries live under by
// default. build returns nil for a server the dialect cannot express, which is
// then left out of the document.
type mcpDialect struct {
	defaultKey []string
	build      func(server *config.MCPServer) map[string]any
	// jsoncDefault makes a ".json" path of the dialect JSONC unless the sidecar
	// names a format: the files of these tools (Zed settings, VS Code mcp.json)
	// routinely carry comments, which a strict JSON merge would not preserve.
	jsoncDefault bool
	// arrayKey, when set, makes the owned member an array of tables rather than a
	// map: each server becomes one element carrying its name under this key
	// ([[mcp_servers]] with name = "..."). TOML documents only.
	arrayKey string
}

// mcpDialects is one map literal, so every dialect exists before any init() (the
// builtin specs are validated against it in hints.go and init.go) and the
// registration order cannot depend on file names.
var mcpDialects = map[string]mcpDialect{
	MCPDialectStandard:     {defaultKey: []string{"mcpServers"}, build: standardMCPEntry},
	MCPDialectClaude:       {defaultKey: []string{"mcpServers"}, build: claudeMCPEntry},
	MCPDialectGemini:       {defaultKey: []string{"mcpServers"}, build: geminiMCPEntry},
	MCPDialectOpencode:     {defaultKey: []string{"mcp"}, build: opencodeMCPEntry},
	MCPDialectVSCode:       {defaultKey: []string{"servers"}, build: vscodeMCPEntry, jsoncDefault: true},
	MCPDialectZed:          {defaultKey: []string{"context_servers"}, build: zedMCPEntry, jsoncDefault: true},
	MCPDialectCodex:        {defaultKey: []string{"mcp_servers"}, build: codexMCPEntry},
	MCPDialectAmp:          {defaultKey: []string{"amp.mcpServers"}, build: ampMCPEntry},
	MCPDialectYAMLStandard: {defaultKey: []string{"mcp_servers"}, build: yamlStandardMCPEntry},
	MCPDialectCodewhale:    {defaultKey: []string{"servers"}, build: codewhaleMCPEntry},
	MCPDialectRoo:          {defaultKey: []string{"mcpServers"}, build: rooMCPEntry},
	MCPDialectBob:          {defaultKey: []string{"mcpServers"}, build: bobMCPEntry},
	MCPDialectZcode:        {defaultKey: []string{"mcp", "servers"}, build: zcodeMCPEntry},
	MCPDialectGrok:         {defaultKey: []string{"mcp_servers"}, build: grokMCPEntry},
	MCPDialectTransport:    {defaultKey: []string{"mcpServers"}, build: transportMCPEntry},
	MCPDialectVibe:         {defaultKey: []string{"mcp_servers"}, build: vibeMCPEntry, arrayKey: "name"},
}

// IsMCPDialect reports whether name is a known MCP entry dialect.
func IsMCPDialect(name string) bool {
	_, ok := mcpDialects[name]
	return ok
}

// MCPDialectNames lists the known dialects, sorted.
func MCPDialectNames() []string {
	names := make([]string, 0, len(mcpDialects))
	for name := range mcpDialects {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// mcpDialectFor resolves a dialect name, with "" meaning standard.
func mcpDialectFor(name string) (mcpDialect, error) {
	if name == "" {
		name = MCPDialectStandard
	}
	d, ok := mcpDialects[name]
	if !ok {
		return mcpDialect{}, fmt.Errorf("unknown mcp dialect %q (known: %s)", name, strings.Join(MCPDialectNames(), ", "))
	}
	return d, nil
}

// mcpDialectEntries renders every configured server in the dialect, keyed by
// server name.
func mcpDialectEntries(d mcpDialect, cfg *config.Config) map[string]any {
	return mcpDialectEntriesFor(d, cfg, nil)
}

// mcpEntryOpts narrows and adapts the entries of a generic mcp sidecar.
type mcpEntryOpts struct {
	// transports limits the servers to these transports (stdio, http, sse); empty
	// means every server.
	transports []string
	// refSyntax is how an environment reference is written (EnvRefSyntax*); empty
	// writes the resolved value.
	refSyntax string
}

// mcpDialectEntriesFor is mcpDialectEntries with the options of a sidecar.
func mcpDialectEntriesFor(d mcpDialect, cfg *config.Config, opts *mcpEntryOpts) map[string]any {
	entries := make(map[string]any)
	if cfg == nil {
		return entries
	}
	if opts == nil {
		opts = &mcpEntryOpts{}
	}
	for name, server := range cfg.MCPServers {
		if server == nil {
			continue
		}
		if len(opts.transports) > 0 && !slices.Contains(opts.transports, server.GetTransport()) {
			continue
		}
		if why := incompleteMCPServer(server); why != "" {
			cfg.Warn("skipping MCP server that cannot be written: "+why, "server", name)
			continue
		}
		if entry := d.build(server); entry != nil {
			applyRefSyntax(entry, server, opts.refSyntax)
			entries[name] = entry
		}
	}
	return entries
}

// Environment reference syntaxes of a sidecar's env_ref_syntax: how a value that
// came from a ${VAR} placeholder is written, for a tool that expands references
// itself, so the secret stays out of the file.
const (
	// EnvRefSyntaxDollar is $NAME (Codebuff). Codebuff documents no braced form, so
	// only a value that is exactly one placeholder is written as a reference.
	EnvRefSyntaxDollar = "dollar"
	// EnvRefSyntaxEnvPrefix is ${env:NAME} (Cursor).
	EnvRefSyntaxEnvPrefix = "env_prefix"
	// EnvRefSyntaxBraced is ${NAME} (Claude Code, Amp, Pi, Factory and the tools that
	// read the shared root .mcp.json). Its delimiters make a reference inside a longer
	// string safe.
	EnvRefSyntaxBraced = "braced"
	// EnvRefSyntaxOpencodeEnv is {env:NAME} (OpenCode and Kilo).
	EnvRefSyntaxOpencodeEnv = "opencode_env"
)

// IsEnvRefSyntax reports whether name is a known env_ref_syntax.
func IsEnvRefSyntax(name string) bool {
	switch name {
	case EnvRefSyntaxDollar, EnvRefSyntaxEnvPrefix, EnvRefSyntaxBraced, EnvRefSyntaxOpencodeEnv:
		return true
	}
	return false
}

// applyRefSyntax rewrites the values of an entry that held a placeholder to the
// tool's reference syntax; see presets.ApplyEnvRefs.
func applyRefSyntax(entry map[string]any, server *config.MCPServer, syntax string) {
	if syntax == "" {
		return
	}
	format := func(name string) string { return formatEnvRef(syntax, name) }
	if syntax == EnvRefSyntaxDollar {
		presets.ApplyWholeEnvRefs(entry, server, format)
		return
	}
	presets.ApplyEnvRefs(entry, server, format)
}

// formatEnvRef is a reference to the environment variable name in syntax.
func formatEnvRef(syntax, name string) string {
	switch syntax {
	case EnvRefSyntaxEnvPrefix:
		return "${env:" + name + "}"
	case EnvRefSyntaxBraced:
		return "${" + name + "}"
	case EnvRefSyntaxOpencodeEnv:
		return "{env:" + name + "}"
	}
	return "$" + name
}

// incompleteMCPServer names what a server lacks to be written at all: the command
// of a stdio server, the url of a remote one. It returns "" for a complete one.
func incompleteMCPServer(server *config.MCPServer) string {
	switch {
	case isRemote(server) && server.URL == "":
		return "remote server has no url"
	case !isRemote(server) && server.Command == "":
		return "stdio server has no command"
	}
	return ""
}

func isRemote(server *config.MCPServer) bool {
	t := server.GetTransport()
	return t == config.TransportHTTP || t == config.TransportSSE
}

// standardMCPEntry is the canonical entry (presets.MCPServerEntry) plus
// `disabled: true` for a disabled server.
func standardMCPEntry(server *config.MCPServer) map[string]any {
	entry := presets.MCPServerEntry(server)
	if entry != nil && !server.IsEnabled() {
		entry["disabled"] = true
	}
	return entry
}

// claudeMCPEntry is the Claude Code entry, as in .claude/settings.json.
func claudeMCPEntry(server *config.MCPServer) map[string]any {
	entry := map[string]any{}
	applyMCPTransport(entry, server)
	if !server.IsEnabled() {
		entry["disabled"] = true
	}
	return entry
}

// geminiMCPEntry mirrors the .gemini/settings.json entry: streamable HTTP is
// `httpUrl`, SSE is `url`.
func geminiMCPEntry(server *config.MCPServer) map[string]any {
	entry := map[string]any{}
	switch server.GetTransport() {
	case config.TransportHTTP:
		if server.URL != "" {
			entry["httpUrl"] = server.URL
		}
	case config.TransportSSE:
		if server.URL != "" {
			entry["url"] = server.URL
		}
	default:
		entry["command"] = server.Command
		if len(server.Args) > 0 {
			entry["args"] = server.Args
		}
	}
	if len(server.Env) > 0 {
		entry["env"] = server.Env
	}
	if isRemote(server) && len(server.Headers) > 0 {
		entry["headers"] = server.Headers
	}
	if !server.IsEnabled() {
		entry["disabled"] = true
	}
	return entry
}

// opencodeMCPEntry is the OpenCode entry (mcp.<name>): local servers take the executable
// and its arguments as one `command` array and `environment` for env; remote
// servers take url/headers. `enabled` is always written.
func opencodeMCPEntry(server *config.MCPServer) map[string]any {
	entry := map[string]any{"enabled": server.IsEnabled()}
	if isRemote(server) {
		entry["type"] = "remote"
		if server.URL != "" {
			entry["url"] = server.URL
		}
		if len(server.Headers) > 0 {
			entry["headers"] = server.Headers
		}
		return entry
	}
	entry["type"] = "local"
	if server.Command != "" {
		entry["command"] = append([]string{server.Command}, server.Args...)
	}
	if len(server.Env) > 0 {
		entry["environment"] = server.Env
	}
	return entry
}

// vscodeMCPEntry is the .vscode/mcp.json entry. VS Code has no way to switch a
// server off, so a disabled server is left out of the document.
func vscodeMCPEntry(server *config.MCPServer) map[string]any {
	return presets.VSCodeMCPEntry(server)
}

// zedMCPEntry is the Zed context_servers entry; a disabled server is
// `enabled: false`. Zed has no SSE transport, so an SSE server is left out.
func zedMCPEntry(server *config.MCPServer) map[string]any {
	if server.GetTransport() == config.TransportSSE {
		return nil
	}
	entry := map[string]any{}
	if isRemote(server) {
		entry["url"] = server.URL
		if len(server.Headers) > 0 {
			entry["headers"] = server.Headers
		}
	} else {
		entry["command"] = server.Command
		if len(server.Args) > 0 {
			entry["args"] = server.Args
		}
		if len(server.Env) > 0 {
			entry["env"] = server.Env
		}
	}
	if !server.IsEnabled() {
		entry["enabled"] = false
	}
	return entry
}

// codexMCPEntry is the [mcp_servers.<name>] table of ~/.codex/config.toml.
func codexMCPEntry(server *config.MCPServer) map[string]any {
	return presets.CodexMCPEntry(server)
}

// ampMCPEntry is the standard entry without the description, which Amp does not
// read.
func ampMCPEntry(server *config.MCPServer) map[string]any {
	entry := standardMCPEntry(server)
	if entry == nil {
		return nil
	}
	delete(entry, "description")
	return entry
}

// yamlStandardMCPEntry is the Poolside settings.yaml entry. A remote server
// carries a transport block whose headers are a list of "Name: value" strings.
func yamlStandardMCPEntry(server *config.MCPServer) map[string]any {
	entry := map[string]any{}
	if isRemote(server) {
		transport := map[string]any{"type": server.GetTransport(), "url": server.URL}
		if len(server.Headers) > 0 {
			transport["headers"] = headerList(server.Headers)
		}
		entry["transport"] = transport
	} else {
		entry["command"] = server.Command
		entry["args"] = append([]string{}, server.Args...) // Poolside requires args, even empty
	}
	if len(server.Env) > 0 {
		entry["env"] = server.Env
	}
	if !server.IsEnabled() {
		entry["disabled"] = true
	}
	return entry
}

func headerList(headers map[string]string) []string {
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	list := make([]string, len(names))
	for i, name := range names {
		list[i] = name + ": " + headers[name]
	}
	return list
}
