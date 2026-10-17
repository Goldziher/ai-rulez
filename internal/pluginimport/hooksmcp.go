package pluginimport

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/importer"
)

// rootRef matches the variables a plugin uses for the directory it is
// installed in (${CLAUDE_PLUGIN_ROOT}, ${CODEX_PLUGIN_ROOT}, ${PLUGIN_ROOT},
// ${extensionPath}) and for its persistent data directory. The braced form may
// carry a shell modifier (${CLAUDE_PLUGIN_ROOT:-x}, ${...#p}, ${.../a/b}), which
// is treated exactly like the plain variable.
var rootRef = sync.OnceValue(func() *regexp.Regexp {
	return regexp.MustCompile(`\$\{[A-Za-z0-9_]*PLUGIN_(?:ROOT|DATA)(?:[^A-Za-z0-9_}][^}]*)?\}|\$[A-Za-z0-9_]*PLUGIN_(?:ROOT|DATA)\b|\$\{extensionPath(?:[^A-Za-z0-9_}][^}]*)?\}`)
})

// scriptRef matches a root variable followed by a path: the file the command runs.
var scriptRef = sync.OnceValue(func() *regexp.Regexp {
	return regexp.MustCompile(`(?:\$\{[A-Za-z0-9_]*PLUGIN_ROOT(?:[^A-Za-z0-9_}][^}]*)?\}|\$[A-Za-z0-9_]*PLUGIN_ROOT\b|\$\{extensionPath(?:[^A-Za-z0-9_}][^}]*)?\})(?:\$\{/\}|/)([^\s"'|;&<>]+)`)
})

// resolveRoot rewrites the plugin-root variables in s. A plugin installed from
// a directory inside the project has a root that exists in the generated tree,
// so the variable becomes that path. Any other plugin has none: ok is false and
// the caller refuses the hook or server rather than write a command that points
// at nothing. The data directory variable is never rewritten.
func (c *loader) resolveRoot(s string) (string, bool) {
	if !rootRef().MatchString(s) && !strings.Contains(s, "${/}") {
		return s, true
	}
	if rootRef().MatchString(s) {
		if c.opt.Rewrite == "" {
			return s, false
		}
		failed := false
		s = rootRef().ReplaceAllStringFunc(s, func(m string) string {
			if strings.Contains(m, "PLUGIN_DATA") {
				failed = true
				return m
			}
			return c.opt.Rewrite
		})
		if failed {
			return s, false
		}
	}
	return strings.ReplaceAll(s, "${/}", "/"), true
}

// scriptFiles lists the plugin files a command or argument runs through a root
// variable, so the security scan reads them.
func (c *loader) scriptFiles(texts ...string) {
	for _, t := range texts {
		for _, m := range scriptRef().FindAllStringSubmatch(t, -1) {
			rel := strings.ReplaceAll(m[1], "${/}", "/")
			abs, err := SafeJoin(c.dir, rel)
			if err != nil {
				continue
			}
			c.addScan(abs)
		}
	}
}

// addScan queues a plugin file for the import-time security scan.
func (c *loader) addScan(abs string) {
	if c.scanned[abs] {
		return
	}
	c.scanned[abs] = true
	data, err := readRegular(abs, MaxContentBytes)
	if err != nil {
		return
	}
	c.res.Scan = append(c.res.Scan, config.ImportedScanFile{Path: abs, Content: string(data), Trust: c.opt.Trust})
}

// source is one document of hooks or MCP servers: a file of the plugin or an
// inline object of the manifest or the marketplace entry.
type source struct {
	name string
	abs  string // the file to scan; the manifest for an inline object
	data []byte
}

// documents gathers the hook or MCP documents of the plugin: the layout's
// default files, then what the manifest and the marketplace entry name.
func (c *loader) documents(defaults []string, field string) []source {
	var out []source
	seen := map[string]bool{}
	addFile := func(rel string) {
		abs, err := SafeJoin(c.dir, rel)
		if err != nil {
			c.note(importer.StatusDropped, c.layout.manifest, field, err.Error())
			return
		}
		if seen[abs] || !isRegular(abs) {
			return
		}
		seen[abs] = true
		data, err := readRegular(abs, MaxManifestBytes)
		if err != nil {
			c.note(importer.StatusDropped, c.rel(abs), "", err.Error())
			return
		}
		out = append(out, source{name: c.rel(abs), abs: abs, data: data})
	}
	for _, rel := range defaults {
		addFile(rel)
	}
	manifestAbs := filepath.Join(c.dir, filepath.FromSlash(c.layout.manifest))
	for _, raw := range []json.RawMessage{c.manifest.Fields[field], c.opt.Inline[field]} {
		if len(raw) == 0 {
			continue
		}
		if list, ok := stringList(raw); ok {
			for _, rel := range list {
				addFile(rel)
			}
			continue
		}
		var items []json.RawMessage
		if json.Unmarshal(raw, &items) == nil {
			for _, it := range items {
				out = append(out, source{name: c.layout.manifest + "#" + field, abs: manifestAbs, data: it})
			}
			continue
		}
		out = append(out, source{name: c.layout.manifest + "#" + field, abs: manifestAbs, data: raw})
	}
	return out
}

func (c *loader) collectHooks() {
	var groups []config.HookGroup
	var used []source
	for _, doc := range c.documents(c.layout.hooks, "hooks") {
		found, notes := importer.PluginHooks(doc.name, doc.data, c.layout.harness)
		c.res.Findings = append(c.res.Findings, notes...)
		groups = append(groups, found...)
		if len(found) > 0 {
			used = append(used, doc)
		}
	}
	if len(groups) == 0 {
		return
	}
	if !c.opt.EnableHooks {
		c.res.HooksDisabled = len(groups)
		c.note(importer.StatusNeedsAction, c.layout.manifest, "hooks", fmt.Sprintf(
			"%d hook group(s) were found and not imported: a hook runs a command on every machine that uses the project; set enable_hooks = true on the plugin to import them", len(groups)))
		return
	}
	for _, g := range groups {
		if kept, ok := c.rewriteHook(g); ok {
			c.res.Hooks = append(c.res.Hooks, kept)
		}
	}
	if len(c.res.Hooks) > 0 {
		for _, doc := range used {
			c.addScan(doc.abs)
		}
	}
}

// rewriteHook resolves the plugin-root variables of every action of a group; a
// group with an action that cannot be resolved is refused whole.
func (c *loader) rewriteHook(g config.HookGroup) (config.HookGroup, bool) {
	out := g
	out.Hooks = make([]config.HookAction, len(g.Hooks))
	for i, a := range g.Hooks {
		c.scriptFiles(append([]string{a.Command}, a.Args...)...)
		cmd, ok := c.resolveRoot(a.Command)
		args, argsOK := c.resolveAll(a.Args)
		if !ok || !argsOK {
			c.note(importer.StatusNeedsAction, c.layout.manifest, "hooks."+g.Event, rootRefusal("hook"))
			return config.HookGroup{}, false
		}
		a.Command, a.Args = cmd, args
		out.Hooks[i] = a
	}
	return out, true
}

func rootRefusal(what string) string {
	return "the " + what + " uses the plugin's root directory (${CLAUDE_PLUGIN_ROOT} or similar), which does not exist in the generated tree, so it was not imported; " +
		"install the plugin from a directory inside the project (a local source) to keep it"
}

func (c *loader) resolveAll(in []string) ([]string, bool) {
	if len(in) == 0 {
		return in, true
	}
	out := make([]string, len(in))
	for i, s := range in {
		v, ok := c.resolveRoot(s)
		if !ok {
			return nil, false
		}
		out[i] = v
	}
	return out, true
}

func (c *loader) resolveMap(in map[string]string) (map[string]string, bool) {
	if len(in) == 0 {
		return in, true
	}
	out := make(map[string]string, len(in))
	for k, s := range in {
		v, ok := c.resolveRoot(s)
		if !ok {
			return nil, false
		}
		out[k] = v
	}
	return out, true
}

func (c *loader) collectMCP() {
	taken := map[string]bool{}
	for _, doc := range c.documents(c.layout.mcp, "mcpServers") {
		var servers []config.MCPServer
		var notes []importer.Finding
		if strings.HasPrefix(doc.name, c.layout.manifest+"#") {
			servers, notes = importer.PluginMCPTable(doc.name, doc.data)
		} else {
			servers, notes = importer.PluginMCP(doc.name, doc.data)
		}
		c.res.Findings = append(c.res.Findings, notes...)
		for i := range servers {
			s := servers[i]
			if !ValidName(s.Name) {
				c.note(importer.StatusDropped, doc.name, "mcpServers."+s.Name, fmt.Sprintf("%q is not a valid server name", s.Name))
				continue
			}
			if taken[s.Name] {
				continue
			}
			resolved, ok := c.rewriteServer(s)
			if !ok {
				c.note(importer.StatusNeedsAction, doc.name, "mcpServers."+s.Name, rootRefusal("MCP server"))
				continue
			}
			taken[s.Name] = true
			resolved.Name = c.opt.NamePrefix + s.Name
			c.res.MCP = append(c.res.MCP, resolved)
			c.addScan(doc.abs)
			c.scriptFiles(append([]string{s.Command}, s.Args...)...)
		}
	}
}

func (c *loader) rewriteServer(s config.MCPServer) (config.MCPServer, bool) {
	out := s
	var ok bool
	if out.Command, ok = c.resolveRoot(s.Command); !ok {
		return out, false
	}
	if out.URL, ok = c.resolveRoot(s.URL); !ok {
		return out, false
	}
	if out.Args, ok = c.resolveAll(s.Args); !ok {
		return out, false
	}
	if out.Env, ok = c.resolveMap(s.Env); !ok {
		return out, false
	}
	if out.Headers, ok = c.resolveMap(s.Headers); !ok {
		return out, false
	}
	return out, true
}
