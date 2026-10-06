package policy

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
)

// MCP governs the repository's [[mcp_servers]].
type MCP struct {
	// AllowedCommands, when set, is the only list of commands a stdio server may
	// run (compared as written, so "npx" does not cover "/usr/bin/npx").
	AllowedCommands List
	// DenyTransports lists transports no server may use ("stdio", "http", "sse").
	DenyTransports []string
}

// Hooks governs the repository's [[hooks]].
type Hooks struct {
	// Forbidden makes any [[hooks]] group a violation (allow = false).
	Forbidden bool
}

type fileMCP struct {
	AllowedCommands *[]string `toml:"allowed_commands"`
	DenyTransports  []string  `toml:"deny_transports"`
}

type fileHooks struct {
	Allow *bool `toml:"allow"`
}

var mcpTransports = []string{config.TransportStdio, config.TransportHTTP, config.TransportSSE}

func (m *MCP) fromDoc(d *fileMCP) error {
	if d == nil {
		return nil
	}
	if d.AllowedCommands != nil {
		items := make([]string, 0, len(*d.AllowedCommands))
		for _, c := range *d.AllowedCommands {
			if c = strings.TrimSpace(c); c == "" || strings.ContainsAny(c, "\r\n\x00") {
				return fmt.Errorf("mcp.allowed_commands: %q is not a command", c)
			}
			items = append(items, c)
		}
		m.AllowedCommands = List{Set: true, Items: sortedUnique(items)}
	}
	for _, t := range d.DenyTransports {
		t = strings.ToLower(strings.TrimSpace(t))
		if !slices.Contains(mcpTransports, t) {
			return fmt.Errorf("mcp.deny_transports: %q is not a transport (use %s)", t, strings.Join(mcpTransports, ", "))
		}
		m.DenyTransports = append(m.DenyTransports, t)
	}
	m.DenyTransports = sortedUnique(m.DenyTransports)
	return nil
}

func (h *Hooks) fromDoc(d *fileHooks) error {
	if d != nil && d.Allow != nil {
		h.Forbidden = !*d.Allow
	}
	return nil
}

func mergeMCP(a, b MCP) MCP {
	return MCP{
		AllowedCommands: intersectExact(a.AllowedCommands, b.AllowedCommands),
		DenyTransports:  union(a.DenyTransports, b.DenyTransports),
	}
}

func (m MCP) addTo(table func(path ...string) map[string]any) {
	if m.AllowedCommands.Set {
		table("mcp")["allowed_commands"] = nonNil(m.AllowedCommands.Items)
	}
	if len(m.DenyTransports) > 0 {
		table("mcp")["deny_transports"] = m.DenyTransports
	}
}

// mcpServers drops the repository's servers the policy forbids: a transport on
// the deny list, or a stdio command outside allowed_commands. A dropped server
// is reported, since the repository asked for something the policy bars.
func (a *applier) mcpServers() {
	pol := a.res.Policy.MCP
	if !pol.AllowedCommands.Set && len(pol.DenyTransports) == 0 {
		return
	}
	a.cfg.MCPServersRaw = filterSlice(a.cfg.MCPServersRaw, func(s config.MCPServer) bool {
		transport := s.GetTransport()
		if slices.Contains(pol.DenyTransports, transport) {
			a.violate(lint.CodeCapabilityNotAllowed, "mcp.deny_transports", s.Name,
				"[[mcp_servers]] %q uses the %s transport, which the policy denies (origin: %s); the server is not loaded", s.Name, transport, a.origin("mcp.deny_transports"))
			return false
		}
		if transport == config.TransportStdio && pol.AllowedCommands.Set && !slices.Contains(pol.AllowedCommands.Items, s.Command) {
			a.violate(lint.CodeCapabilityNotAllowed, "mcp.allowed_commands", s.Name,
				"[[mcp_servers]] %q runs %q, which is not in mcp.allowed_commands %s (origin: %s); the server is not loaded", s.Name, s.Command, quoteList(pol.AllowedCommands.Items), a.origin("mcp.allowed_commands"))
			return false
		}
		return true
	})
}

// hooks removes the repository's hook groups when the policy forbids hooks.
func (a *applier) hooks() {
	if !a.res.Policy.Hooks.Forbidden {
		return
	}
	for _, g := range a.cfg.Hooks {
		a.violate(lint.CodeCapabilityNotAllowed, "hooks.allow", g.Event,
			"[[hooks]] group %q is declared, but the policy forbids hooks (origin: %s); the group is not loaded", g.Event, a.origin("hooks.allow"))
	}
	a.cfg.Hooks = nil
}
