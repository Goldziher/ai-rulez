package importer

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

type mcpFile struct {
	Path string
	// Keys are the JSON keys that hold the server table, in order of preference.
	Keys []string
}

// mcpFiles are the project MCP files of the supported tools.
var mcpFiles = []mcpFile{
	{Path: ".mcp.json", Keys: []string{"mcpServers"}},
	{Path: ".cursor/mcp.json", Keys: []string{"mcpServers"}},
	{Path: ".vscode/mcp.json", Keys: []string{"servers", "mcpServers"}},
	{Path: ".kiro/settings/mcp.json", Keys: []string{"mcpServers"}},
	{Path: ".roo/mcp.json", Keys: []string{"mcpServers"}},
	{Path: ".gemini/settings.json", Keys: []string{"mcpServers"}},
	{Path: ".qwen/settings.json", Keys: []string{"mcpServers"}},
}

var mcpKnownKeys = map[string]bool{
	"command": true, "args": true, "env": true, "url": true, "serverUrl": true, "type": true,
	"transport": true, "headers": true, "disabled": true, "enabled": true, "description": true,
}

func importMCP(p *Plan, r *reader) {
	seen := map[string]config.MCPServer{}
	for _, f := range mcpFiles {
		if _, ok := r.exists(f.Path); !ok {
			continue
		}
		data, err := r.read(f.Path)
		if err != nil {
			p.add(newFinding(StatusDropped, f.Path, "", "", skipReasonOr(err)))
			continue
		}
		var doc map[string]json.RawMessage
		if err := json.Unmarshal(stripJSONC(data), &doc); err != nil {
			p.add(newFinding(StatusUnsupported, f.Path, "", "", "not valid JSON: "+err.Error()))
			continue
		}
		var table map[string]map[string]any
		for _, key := range f.Keys {
			raw, ok := doc[key]
			if !ok {
				continue
			}
			if err := json.Unmarshal(raw, &table); err != nil {
				p.add(newFinding(StatusUnsupported, f.Path, key, "", "server table is not an object"))
				table = nil
			}
			break
		}
		names := make([]string, 0, len(table))
		for name := range table {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			srv, ok := mcpServerFrom(p, f.Path, name, table[name])
			if !ok {
				continue
			}
			if prev, dup := seen[name]; dup {
				if !reflect.DeepEqual(prev, srv) {
					p.add(newFinding(StatusDropped, f.Path, "mcpServers."+name, "",
						"a different definition of this server was already imported from another file"))
				}
				continue
			}
			seen[name] = srv
			p.MCPServers = append(p.MCPServers, srv)
			p.add(newFinding(StatusMapped, f.Path, "mcpServers."+name, "mcp_servers."+name, ""))
		}
	}
}

// stripJSONC removes whole-line // comments, which editors allow in MCP files.
func stripJSONC(data []byte) []byte {
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		out = append(out, line)
	}
	return []byte(strings.Join(out, "\n"))
}

func mcpServerFrom(p *Plan, file, name string, raw map[string]any) (config.MCPServer, bool) {
	field := "mcpServers." + name
	srv := config.MCPServer{Name: name}
	if name == "" {
		return srv, false
	}
	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !mcpKnownKeys[k] {
			p.add(newFinding(StatusDropped, file, field+"."+k, "", "key has no ai-rulez equivalent"))
		}
	}
	srv.Command, _ = raw["command"].(string)
	if args, ok := raw["args"].([]any); ok {
		for _, a := range args {
			srv.Args = append(srv.Args, fmt.Sprint(a))
		}
	}
	srv.URL, _ = raw["url"].(string)
	if srv.URL == "" {
		srv.URL, _ = raw["serverUrl"].(string)
	}
	srv.Description, _ = raw["description"].(string)
	if isOwnServer(srv) {
		p.add(newFinding(StatusDropped, file, field, "", "this is ai-rulez's own MCP server, which generate adds itself"))
		return srv, false
	}
	if srv.Command == "" && srv.URL == "" {
		p.add(newFinding(StatusUnsupported, file, field, "", "server has neither command nor url"))
		return srv, false
	}

	typ, _ := raw["type"].(string)
	if typ == "" {
		typ, _ = raw["transport"].(string)
	}
	switch strings.ToLower(typ) {
	case "", "stdio":
		if srv.URL != "" {
			srv.Transport = config.TransportHTTP
			p.add(newFinding(StatusApproximated, file, field+".type", "mcp_servers."+name+".transport",
				"remote server without a type is imported as http"))
		}
	case "http", "streamable-http", "streamablehttp", "streamable_http":
		srv.Transport = config.TransportHTTP
	case "sse":
		srv.Transport = config.TransportSSE
	default:
		p.add(newFinding(StatusUnsupported, file, field+".type", "", "transport "+typ+" is not supported"))
		return srv, false
	}
	if d, ok := raw["disabled"].(bool); ok && d {
		off := false
		srv.Enabled = &off
	}
	if e, ok := raw["enabled"].(bool); ok && !e {
		off := false
		srv.Enabled = &off
	}
	c := credentialScan{plan: p, file: file, field: field, server: name}
	srv.Args = c.args(srv.Args)
	srv.URL = c.url(field+".url", srv.URL)
	srv.Env = c.values(".env", stringMap(raw["env"]), false)
	srv.Headers = c.values(".headers", stringMap(raw["headers"]), true)
	if len(srv.Headers) > 0 && srv.Transport == "" {
		p.add(newFinding(StatusDropped, file, field+".headers", "", "headers only apply to http and sse servers"))
		srv.Headers = nil
	}
	return srv, true
}

func isOwnServer(s config.MCPServer) bool {
	if strings.Contains(s.Command, "ai-rulez") {
		return true
	}
	for _, a := range s.Args {
		if a == "ai-rulez" || strings.HasPrefix(a, "ai-rulez@") {
			return true
		}
	}
	return false
}

func stringMap(v any) map[string]string {
	m, ok := v.(map[string]any)
	if !ok || len(m) == 0 {
		return nil
	}
	out := map[string]string{}
	for k, val := range m {
		out[k] = fmt.Sprint(val)
	}
	return out
}

// importClaudeSettings reports hooks and permissions in .claude/settings.json:
// importing them is a later phase and imported hooks are never auto-enabled.
func importClaudeSettings(p *Plan, r *reader) {
	const file = ".claude/settings.json"
	if _, ok := r.exists(file); !ok {
		return
	}
	data, err := r.read(file)
	if err != nil {
		p.add(newFinding(StatusDropped, file, "", "", skipReasonOr(err)))
		return
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		p.add(newFinding(StatusDropped, file, "", "", "not valid JSON, so its hooks and permissions were not read: "+err.Error()))
		return
	}
	for _, key := range []string{"hooks", "permissions"} {
		if _, ok := doc[key]; ok {
			p.add(newFinding(StatusNeedsAction, file, key, "",
				"not imported yet; add it to config.toml by hand and review it (imported hooks are never enabled automatically)"))
		}
	}
}
