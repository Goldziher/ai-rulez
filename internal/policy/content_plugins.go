package policy

import (
	"fmt"
	"slices"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
)

// pluginSection is the configuration table the plugin violations are reported at.
const pluginSection = "installed_plugins"

// plugins bounds the hooks and MCP servers installed plugins delivered by the
// same [hooks] and [mcp] policy that bounds config.toml itself. A hook group or
// server the policy forbids is removed from the loaded configuration and
// reported (AR748), like the same declaration in an imported agent's frontmatter.
func (c *contentApplier) plugins() {
	if c.forbidHooks {
		kept := c.cfg.Hooks[:0:0]
		for _, g := range c.cfg.Hooks {
			if g.Builtin != config.HookBuiltinPlugin {
				kept = append(kept, g)
				continue
			}
			c.reportPlugin("hooks.allow", "an installed plugin delivers a %s hook, but the policy forbids hooks (origin: %s); it is not loaded",
				g.Event, c.res.Provenance["hooks.allow"])
		}
		c.cfg.Hooks = kept
	}
	if !c.boundMCP {
		return
	}
	pol := c.res.Policy.MCP
	for _, name := range c.cfg.PluginMCPServers {
		s, ok := c.cfg.MCPServers[name]
		if !ok {
			continue
		}
		transport := serverTransport(s)
		switch {
		case slices.Contains(pol.DenyTransports, transport):
			c.reportPlugin("mcp.deny_transports", "an installed plugin delivers MCP server %q on the %s transport, which the policy denies (origin: %s); it is not loaded",
				name, transport, c.res.Provenance["mcp.deny_transports"])
		case transport == config.TransportStdio && pol.AllowedCommands.Set && !slices.Contains(pol.AllowedCommands.Items, s.Command):
			c.reportPlugin("mcp.allowed_commands", "an installed plugin delivers MCP server %q running %q, which is not in mcp.allowed_commands %s (origin: %s); it is not loaded",
				name, s.Command, quoteList(pol.AllowedCommands.Items), c.res.Provenance["mcp.allowed_commands"])
		default:
			continue
		}
		delete(c.cfg.MCPServers, name)
	}
}

func serverTransport(s *config.MCPServer) string {
	switch {
	case s.Type != "":
		return s.Type
	case s.Transport != "":
		return s.Transport
	case s.Command != "":
		return config.TransportStdio
	case s.URL != "":
		return config.TransportHTTP
	}
	return config.TransportStdio
}

func (c *contentApplier) reportPlugin(key, format string, args ...any) {
	c.found = append(c.found, config.PolicyViolation{
		Code: lint.CodeCapabilityNotAllowed, Key: key, File: pluginSection, Line: 1,
		Message: fmt.Sprintf(format, args...), Origin: c.res.Provenance[key],
	})
}
