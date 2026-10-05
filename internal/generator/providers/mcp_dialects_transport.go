package providers

import (
	"fmt"
	"os"
	"sort"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
	toml "github.com/pelletier/go-toml/v2"
)

const (
	// MCPDialectTransport is the mcpServers shape whose transport is the
	// `transport` member (stdio|http|sse) rather than `type` (Kimi Code):
	// command/args/env or url/headers, `enabled = false` for a disabled server.
	MCPDialectTransport = "transport"
	// MCPDialectVibe is Mistral Vibe's [[mcp_servers]] array of tables: each
	// element is named by `name`, takes transport stdio|http (SSE is served by
	// http) and `disabled = true` for a disabled server.
	MCPDialectVibe = "vibe"
)

// vibeUserManagedFields are keys Vibe's /mcp panel writes into a server it
// does not own; they are carried over from a same-named existing element.
var vibeUserManagedFields = []string{"prompt", "sampling_enabled", "disabled", "disabled_tools"}

func transportMCPEntry(server *config.MCPServer) map[string]any {
	entry := map[string]any{}
	if isRemote(server) {
		entry["transport"] = server.GetTransport()
		entry["url"] = server.URL
		if len(server.Headers) > 0 {
			entry["headers"] = server.Headers
		}
	} else {
		entry["transport"] = "stdio"
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

func vibeMCPEntry(server *config.MCPServer) map[string]any {
	entry := map[string]any{}
	if isRemote(server) {
		entry["transport"] = "http"
		entry["url"] = server.URL
		if len(server.Headers) > 0 {
			entry["headers"] = server.Headers
		}
	} else {
		entry["transport"] = "stdio"
		entry["command"] = server.Command
		if len(server.Args) > 0 {
			entry["args"] = server.Args
		}
		if len(server.Env) > 0 {
			entry["env"] = server.Env
		}
	}
	if !server.IsEnabled() {
		entry["disabled"] = true
	}
	return entry
}

// arrayOwnedKey builds the owned key of an array-of-tables dialect. The array
// keeps every existing element whose name is not one of ours (servers added with
// `vibe mcp add`) and claims only the elements ai-rulez writes.
func arrayOwnedKey(sc *SidecarSpec, d mcpDialect, cfg *config.Config, outputPath string) (jsonmerge.OwnedKey, error) {
	if sc.DocFormat() != DocFormatTOML {
		return jsonmerge.OwnedKey{}, fmt.Errorf("dialect with an array layout needs a toml document, got %q", sc.DocFormat())
	}
	path := sc.ownedKeyPath(d)
	entries := mcpDialectEntries(d, cfg)
	existing, err := existingArrayElements(outputPath, path, d.arrayKey)
	if err != nil {
		return jsonmerge.OwnedKey{}, err
	}
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)

	ours := make([]any, 0, len(names))
	var kept []any
	byName := make(map[string]map[string]any, len(existing))
	for _, el := range existing {
		if name, _ := el[d.arrayKey].(string); name != "" {
			byName[name] = el
		}
	}
	for _, el := range existing {
		if name, _ := el[d.arrayKey].(string); entries[name] == nil {
			kept = append(kept, el)
		}
	}
	for _, name := range names {
		entry := map[string]any{d.arrayKey: name}
		for k, v := range entries[name].(map[string]any) {
			entry[k] = v
		}
		if prev := byName[name]; prev != nil {
			for _, field := range vibeUserManagedFields {
				if _, set := entry[field]; !set && prev[field] != nil {
					entry[field] = prev[field]
				}
			}
		}
		ours = append(ours, entry)
	}
	return jsonmerge.OwnedKey{Path: path, Value: append(kept, ours...), Elements: ours}, nil
}

// existingArrayElements reads the array of tables at path from the TOML file, or
// nil when the file or the member is absent.
func existingArrayElements(file string, path []string, nameKey string) ([]map[string]any, error) {
	raw, err := os.ReadFile(file)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var tree map[string]any
	if err := toml.Unmarshal(raw, &tree); err != nil {
		return nil, fmt.Errorf("parse %s: %w", file, err)
	}
	var node any = tree
	for _, seg := range path {
		m, ok := node.(map[string]any)
		if !ok {
			return nil, nil
		}
		node = m[seg]
	}
	list, ok := node.([]any)
	if !ok {
		return nil, nil
	}
	out := make([]map[string]any, 0, len(list))
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out, nil
}
