package config

import "strings"

// ShowMCPLine reports whether generated headers may carry the MCP server line.
func (h *HeaderConfig) ShowMCPLine() bool {
	return h == nil || h.MCPLine == nil || *h.MCPLine
}

// HeaderMCPCommand returns the command the header tells agents to run as the
// ai-rulez MCP server, derived from the same [mcp] settings that write the
// .mcp.json entry (self_server_command, self_server_version, and the
// --serve-skills flag when skills are served). show is false when the header
// must not mention a server: [header] mcp_line = false, or [mcp] self_server =
// false, where no ai-rulez server is configured to point at.
func (c *Config) HeaderMCPCommand() (cmd string, show bool) {
	if c == nil || !c.Header.ShowMCPLine() || !c.HasSelfServer() {
		return "", false
	}
	entry := c.SelfMCPServerEntry()
	parts := []string{}
	if command, ok := entry[mcpKeyCommand].(string); ok {
		parts = append(parts, command)
	}
	if args, ok := entry["args"].([]string); ok {
		parts = append(parts, args...)
	}
	return strings.Join(parts, " "), true
}

// Header modes accepted by [header] nested.
const (
	HeaderNestedCompact = "compact"
	HeaderNestedOmit    = "omit"
	HeaderNestedInherit = "inherit"
)

// NestedHeaderMode returns how this root's header differs from the outer
// root's: "inherit" (the full header of the configured style), "compact" (one
// line) or "omit". A root that no other root encloses always inherits. For a
// nested root the default is compact, except that a custom [header] text is kept
// unless [header] nested is set explicitly.
func (c *Config) NestedHeaderMode() string {
	if c == nil || !c.NestedRoot {
		return HeaderNestedInherit
	}
	if c.Header != nil && c.Header.Nested != "" {
		return c.Header.Nested
	}
	if c.Header.GetCustomHeader() != "" {
		return HeaderNestedInherit
	}
	return HeaderNestedCompact
}
