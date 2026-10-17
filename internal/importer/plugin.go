package importer

import (
	"encoding/json"
	"slices"
	"sort"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// The functions below let the installed-plugin importer (internal/pluginimport)
// read the hook and MCP documents of a plugin with the same mapping `convert`
// applies to a project's tool files: the event tables, the credential
// redaction and the report of what could not be carried.

// PluginHooks reads the hooks document of a plugin: either a file of the shape
// {"hooks": {Event: [...]}} (Claude Code's hooks/hooks.json) or the event table
// itself. harness is the ai-rulez harness the plugin belongs to; the groups are
// restricted to it, so a hook written for one tool never starts running in the
// others. file names the document in the findings.
func PluginHooks(file string, data []byte, harness string) ([]config.HookGroup, []Finding) {
	p := &Plan{}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(stripJSONC(data), &doc); err != nil {
		p.add(newFinding(StatusUnsupported, file, "", "", "not valid JSON, so its hooks were not read: "+err.Error()))
		return nil, p.Findings
	}
	table := doc
	if raw, ok := doc[litHooks]; ok {
		var inner map[string]json.RawMessage
		if err := json.Unmarshal(raw, &inner); err != nil {
			p.add(newFinding(StatusUnsupported, file, litHooks, "", "hooks is not an object keyed by event"))
			return nil, p.Findings
		}
		table = inner
	}
	delete(table, "description")
	src := hookSource{file: file, harness: harness}
	switch harness {
	case config.HarnessCursor:
		src.events = cursorHookEvents
	case config.HarnessGemini:
		src.events, src.timeoutMs = geminiHookEvents, true
	}
	b := &hookBuilder{p: p}
	b.fromEventTable(src, table, "hooks.")
	// A plugin cannot add an event the configuration would not accept: that
	// would fail validation of the whole project.
	kept := p.Hooks[:0:0]
	for _, g := range p.Hooks {
		if !slices.Contains(config.KnownHookEvents, g.Event) {
			p.add(newFinding(StatusUnsupported, file, "hooks."+g.Event, "", "the event is not one ai-rulez hooks support, so the group was not imported"))
			continue
		}
		kept = append(kept, g)
	}
	sort.SliceStable(kept, func(i, j int) bool { return hookLess(kept[i], kept[j]) })
	return kept, p.Findings
}

// PluginMCP reads the MCP servers of a plugin file: {"mcpServers": {...}}, or
// the table of servers itself (Claude Code's plugin .mcp.json accepts both).
// Literal credentials are replaced by ${VAR} references and reported.
func PluginMCP(file string, data []byte) ([]config.MCPServer, []Finding) {
	p := &Plan{}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(stripJSONC(data), &doc); err != nil {
		p.add(newFinding(StatusUnsupported, file, "", "", "not valid JSON: "+err.Error()))
		return nil, p.Findings
	}
	raw := data
	for _, key := range []string{litMCPServers, "servers"} {
		if inner, ok := doc[key]; ok {
			raw = inner
			break
		}
	}
	return pluginMCPTable(p, file, raw)
}

// PluginMCPTable is PluginMCP for a table of servers already extracted from a
// manifest (gemini-extension.json, plugin.json inline mcpServers).
func PluginMCPTable(file string, table json.RawMessage) ([]config.MCPServer, []Finding) {
	return pluginMCPTable(&Plan{}, file, table)
}

func pluginMCPTable(p *Plan, file string, raw []byte) ([]config.MCPServer, []Finding) {
	var table map[string]map[string]any
	if err := json.Unmarshal(stripJSONC(raw), &table); err != nil {
		p.add(newFinding(StatusUnsupported, file, litMCPServers, "", "server table is not an object"))
		return nil, p.Findings
	}
	names := make([]string, 0, len(table))
	for name := range table {
		names = append(names, name)
	}
	sort.Strings(names)
	var out []config.MCPServer
	for _, name := range names {
		srv, ok := mcpServerFrom(p, file, name, table[name])
		if !ok {
			continue
		}
		out = append(out, srv)
		p.add(newFinding(StatusMapped, file, "mcpServers."+name, "mcp_servers."+name, ""))
	}
	return out, p.Findings
}
