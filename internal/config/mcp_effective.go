package config

import "sort"

// EffectiveMCPServers returns the MCP servers generation uses, as written
// (${VAR} placeholders unresolved): the [[mcp_servers]] of the configuration in
// the order they were authored, then any server set only on Config.MCPServers
// (programmatically), sorted by name, without duplicates.
//
// Config.MCPServers is the working copy: a render resolves placeholders in place
// there, so a check that must see what the author wrote reads this instead.
func (c *Config) EffectiveMCPServers() []MCPServer {
	servers := make([]MCPServer, 0, len(c.MCPServersRaw))
	seen := map[string]bool{}
	for i := range c.MCPServersRaw {
		servers = append(servers, c.MCPServersRaw[i].Clone())
		seen[c.MCPServersRaw[i].Name] = true
	}
	var extra []MCPServer
	add := func(name string, s *MCPServer) {
		if s == nil || seen[name] {
			return
		}
		seen[name] = true
		clone := s.Clone()
		clone.Name = name
		extra = append(extra, clone)
	}
	// Servers set programmatically (no file behind them) keep their current value.
	for name, s := range c.MCPServers {
		add(name, s)
	}
	sort.Slice(extra, func(i, j int) bool { return extra[i].Name < extra[j].Name })
	return append(servers, extra...)
}
