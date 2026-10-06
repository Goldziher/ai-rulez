package importer

import (
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/tailscale/hujson"
)

// rulesyncGenerationKeys are rulesync.jsonc settings that steer how rulesync
// writes its outputs; ai-rulez has no counterpart.
var rulesyncGenerationKeys = map[string]string{
	"outputRoots":                   "rulesync output directories",
	"delete":                        "rulesync output cleanup",
	"verbose":                       "rulesync logging",
	"silent":                        "rulesync logging",
	"dryRun":                        "rulesync run mode",
	"check":                         "rulesync run mode",
	"simulateCommands":              "rulesync simulation of commands for tools without them",
	"simulateSubagents":             "rulesync simulation of subagents for tools without them",
	"simulateSkills":                "rulesync simulation of skills for tools without them",
	"preserveUnownedHooks":          "rulesync hook merging",
	"deriveSubprojectPathFromGlobs": "rulesync nested AGENTS.md placement",
	"flattenedCommandNaming":        "rulesync command file naming",
	"gitignoreTargetsOnly":          "rulesync gitignore generation",
	"gitignoreDestination":          "rulesync gitignore generation",
}

func (b *rulesyncPlanner) importConfig(cfg *rulesyncConfig) {
	if cfg == nil {
		return
	}
	keys := make([]string, 0, len(cfg.raw))
	for k := range cfg.raw {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	const src = rulesyncConfigFile
	for _, k := range keys {
		raw := cfg.raw[k]
		switch k {
		case "$schema", "inputRoots", "inputRoot":
		case "targets":
			b.importTargets(raw)
		case "features":
			b.importFeatures(raw)
		case "sources":
			b.importSources(raw)
		case "language":
			b.p.add(newFinding(StatusDropped, src, k, "",
				"ai-rulez has no response-language setting; add the instruction as a rule if you need it"))
		case "global":
			b.p.add(newFinding(StatusDropped, src, k, "",
				"user-scope generation is not a project setting; use `ai-rulez generate --user` with a user config"))
		default:
			if what, ok := rulesyncGenerationKeys[k]; ok {
				b.p.add(newFinding(StatusDropped, src, k, "", what+"; ai-rulez has no equivalent setting"))
			} else {
				b.p.add(newFinding(StatusDropped, src, k, "", "unrecognised rulesync.jsonc key"))
			}
		}
	}
}

// importTargets maps the tools of rulesync.jsonc to presets.
func (b *rulesyncPlanner) importTargets(raw json.RawMessage) {
	const src = rulesyncConfigFile
	var names []string
	perTarget := map[string]json.RawMessage{}
	var list []string
	if json.Unmarshal(raw, &list) == nil {
		names = list
	} else if json.Unmarshal(raw, &perTarget) == nil {
		for n := range perTarget {
			names = append(names, n)
		}
	} else {
		b.p.add(newFinding(StatusUnsupported, src, "targets", "", "targets is neither a list nor an object"))
		return
	}
	sort.Strings(names)
	for _, n := range names {
		field := "targets." + n
		switch preset, ok := rulesyncPresets[n]; {
		case n == "*":
			b.p.add(newFinding(StatusNeedsAction, src, field, "presets",
				"* means every rulesync tool; set `presets` in config.toml to the tools you use"))
		case ok:
			b.p.Presets = append(b.p.Presets, preset)
			b.p.add(newFinding(StatusMapped, src, field, "presets."+preset, ""))
		default:
			reason := rulesyncUnsupported[n]
			if reason == "" {
				reason = "unknown rulesync target"
			}
			b.p.add(newFinding(StatusUnsupported, src, field, "", reason))
		}
		if v, ok := perTarget[n]; ok && !trivialFeatures(v) {
			b.p.add(newFinding(StatusApproximated, src, field, "",
				"per-tool feature selection and options are not carried; the preset writes every kind the tool supports"))
		}
	}
}

// trivialFeatures reports whether a per-target value enables everything
// (true, "*", or ["*"]).
func trivialFeatures(raw json.RawMessage) bool {
	var on bool
	if json.Unmarshal(raw, &on) == nil {
		return on
	}
	var list []string
	if json.Unmarshal(raw, &list) == nil {
		return len(list) == 1 && list[0] == "*"
	}
	return false
}

func (b *rulesyncPlanner) importFeatures(raw json.RawMessage) {
	if trivialFeatures(raw) {
		return
	}
	b.p.add(newFinding(StatusApproximated, rulesyncConfigFile, "features", "",
		"restricting rulesync to some features is not carried; ai-rulez presets write every kind the tool supports"))
}

// readJSONC reads a JSON or JSONC input file; a file that cannot be parsed is
// reported and skipped.
func (b *rulesyncPlanner) readJSONC(file string) (map[string]json.RawMessage, bool) {
	data, err := b.r.read(file)
	if err != nil {
		b.p.add(newFinding(StatusDropped, file, "", "", skipReasonOr(err)))
		return nil, false
	}
	std, err := hujson.Standardize(data)
	var doc map[string]json.RawMessage
	if err == nil {
		err = json.Unmarshal(std, &doc)
	}
	if err != nil {
		b.p.add(newFinding(StatusUnsupported, file, "", "", "not valid JSON: "+err.Error()))
		return nil, false
	}
	return doc, true
}

// firstExisting returns the first of the names below root that exists, and
// reports the others as shadowed.
func (b *rulesyncPlanner) firstExisting(root string, names ...string) string {
	found := ""
	for _, n := range names {
		file := path.Join(root, n)
		if _, ok := b.r.exists(file); !ok {
			continue
		}
		if found == "" {
			found = file
			continue
		}
		b.p.add(newFinding(StatusDropped, file, "", "", path.Base(found)+" takes precedence and is the one imported"))
	}
	return found
}

// rulesyncServerOnly are per-server keys rulesync reads that ai-rulez cannot express.
var rulesyncServerOnly = map[string]string{
	"targets":       "per-server targets are not carried; the server is generated for every preset",
	"enabledTools":  "per-tool filtering has no ai-rulez equivalent",
	"disabledTools": "per-tool filtering has no ai-rulez equivalent",
}

func (b *rulesyncPlanner) importMCP(root string) {
	file := b.firstExisting(root, "mcp.jsonc", "mcp.json", ".mcp.json")
	if file == "" {
		return
	}
	doc, ok := b.readJSONC(file)
	if !ok {
		return
	}
	keys := make([]string, 0, len(doc))
	for k := range doc {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		switch k {
		case "$schema":
		case "mcpServers":
			b.importServers(file, doc[k])
		default:
			var scoped struct {
				Servers map[string]json.RawMessage `json:"mcpServers"`
			}
			if json.Unmarshal(doc[k], &scoped) != nil || scoped.Servers == nil {
				b.p.add(newFinding(StatusDropped, file, k, "", "unrecognised key"))
				continue
			}
			names := make([]string, 0, len(scoped.Servers))
			for n := range scoped.Servers {
				names = append(names, n)
			}
			sort.Strings(names)
			b.p.add(newFinding(StatusDropped, file, k+".mcpServers", "", fmt.Sprintf(
				"servers scoped to %s (%s) are not imported; ai-rulez MCP servers are not scoped per tool, add them by hand if every preset may use them",
				k, strings.Join(names, ", "))))
		}
	}
}

func (b *rulesyncPlanner) importServers(file string, raw json.RawMessage) {
	var table map[string]json.RawMessage
	if err := json.Unmarshal(raw, &table); err != nil {
		b.p.add(newFinding(StatusUnsupported, file, "mcpServers", "", "server table is not an object"))
		return
	}
	names := make([]string, 0, len(table))
	for n := range table {
		if n != "$schema" { // the editor schema reference some files put inside the table
			names = append(names, n)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		field := "mcpServers." + name
		var entry map[string]any
		if err := json.Unmarshal(table[name], &entry); err != nil || entry == nil {
			b.p.add(newFinding(StatusUnsupported, file, field, "", "server is not an object"))
			continue
		}
		for _, k := range []string{"targets", "enabledTools", "disabledTools"} {
			if v, ok := entry[k]; ok {
				if k == "targets" && !onlyWildcard(v) {
					b.p.add(newFinding(StatusApproximated, file, field+"."+k, "", rulesyncServerOnly[k]))
				} else if k != "targets" {
					b.p.add(newFinding(StatusDropped, file, field+"."+k, "", rulesyncServerOnly[k]))
				}
				delete(entry, k)
			}
		}
		if t := strings.ToLower(stringOf(entry["type"])); t == "local" {
			entry["type"] = "stdio"
		}
		if t := strings.ToLower(stringOf(entry["transport"])); t == "local" {
			entry["transport"] = "stdio"
		}
		srv, ok := mcpServerFrom(b.p, file, name, entry)
		if !ok {
			continue
		}
		b.p.MCPServers = append(b.p.MCPServers, srv)
		b.p.add(newFinding(StatusMapped, file, field, "mcp_servers."+name, ""))
	}
}

func onlyWildcard(v any) bool {
	l := listOf(v)
	return len(l) == 0 || (len(l) == 1 && l[0] == "*")
}

// importHooksAndPermissions reads hooks.jsonc and permissions.jsonc. What they
// declare lands in the plan as [[hooks]] and [permissions]; convert writes both
// disabled unless asked (see hooks.go).
func (b *rulesyncPlanner) importHooksAndPermissions(root string) {
	if file := b.firstExisting(root, "hooks.jsonc", "hooks.json"); file != "" {
		b.importHooks(file)
	}
	if file := b.firstExisting(root, "permissions.jsonc", "permissions.json"); file != "" {
		b.importPermissions(file)
	}
}
