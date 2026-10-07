package policy

import (
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/lint"
)

// ApplyContent bounds the hooks and MCP servers that the content delivered by
// includes and installed skills declares in its frontmatter, by the same [hooks]
// and [mcp] policy that bounds the repository's own config.toml. It runs after
// the content is loaded, since Apply runs before anything is fetched.
//
// An imported agent, skill or command whose `hooks` the policy forbids, or whose
// `mcpServers` has a server the policy denies, loses that key: it is not loaded.
// The server list is dropped whole, because one denied entry makes the file's
// own declaration untrustworthy and a server cannot be removed from it without
// rewriting the author's YAML. Every drop is reported (AR748).
func (r *Resolved) ApplyContent(cfg *config.Config) []config.PolicyViolation {
	pol := r.Policy
	forbidHooks := pol.Hooks.Forbidden
	boundMCP := pol.MCP.AllowedCommands.Set || len(pol.MCP.DenyTransports) > 0
	if cfg == nil || cfg.Content == nil || (!forbidHooks && !boundMCP) {
		return nil
	}
	c := &contentApplier{res: r, cfg: cfg, forbidHooks: forbidHooks, boundMCP: boundMCP}
	c.tree(cfg.Content)
	sort.SliceStable(c.found, func(i, j int) bool {
		x, y := c.found[i], c.found[j]
		if x.Key != y.Key {
			return x.Key < y.Key
		}
		return x.File < y.File
	})
	return c.found
}

type contentApplier struct {
	res         *Resolved
	cfg         *config.Config
	forbidHooks bool
	boundMCP    bool
	found       []config.PolicyViolation
}

func (c *contentApplier) tree(t *config.ContentTree) {
	c.files(t.Agents, t.Skills, t.Commands)
	for _, d := range t.Domains {
		if d == nil || d.Builtin {
			continue // a builtin pack ships with the binary, not with a source the policy must vet
		}
		c.files(d.Agents, d.Skills, d.Commands)
	}
}

func (c *contentApplier) files(groups ...[]config.ContentFile) {
	for _, files := range groups {
		for i := range files {
			f := &files[i]
			if f.Metadata == nil || !c.imported(f.Path) {
				continue
			}
			if c.forbidHooks {
				c.dropHooks(f)
			}
			if c.boundMCP {
				c.dropMCP(f)
			}
		}
	}
}

// imported reports whether a content file arrived through an include or an
// installed skill: it lives outside the project's own configuration directory,
// or inside the directory of a local-path include.
func (c *contentApplier) imported(path string) bool {
	if path == "" || strings.Contains(path, "://") {
		return false // a builtin pack, not a source the policy must vet
	}
	// A configuration loaded from a relative directory carries relative paths:
	// compare absolute forms so none of them reads as "not imported".
	path = absPath(path)
	if c.cfg.ConfigDir == "" || !within(absPath(c.cfg.ConfigDir), path) {
		return true
	}
	for i := range c.cfg.Includes {
		src := c.cfg.Includes[i].Source
		if isRemoteSource(src) {
			continue
		}
		root := src
		if !filepath.IsAbs(root) {
			root = filepath.Join(c.cfg.BaseDir, root)
		}
		root = absPath(root)
		if within(root, path) && !within(absPath(c.cfg.ConfigDir), root) {
			return true
		}
	}
	return false
}

func absPath(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return filepath.Clean(p)
}

func isRemoteSource(src string) bool {
	return strings.Contains(src, "://") || strings.HasPrefix(src, "git@") || strings.HasPrefix(src, "git+")
}

func within(dir, path string) bool {
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(path))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (c *contentApplier) report(f *config.ContentFile, key, format string, args ...any) {
	c.found = append(c.found, config.PolicyViolation{
		Code: lint.CodeCapabilityNotAllowed, Key: key, File: f.Path, Line: 1,
		Message: fmt.Sprintf(format, args...), Origin: c.res.Provenance[key],
	})
}

// dropHooks unloads every spelling of the hooks key (config.ExtraKeysFor reads
// the registry of executing keys the renderers share).
func (c *contentApplier) dropHooks(f *config.ContentFile) {
	keys := f.Metadata.ExtraKeysFor(config.FrontmatterHooks)
	if len(keys) == 0 {
		return
	}
	c.report(f, "hooks.allow", "%q declares hooks in its frontmatter, but the policy forbids hooks (origin: %s); they are not loaded",
		f.Name, c.res.Provenance["hooks.allow"])
	f.Metadata = f.Metadata.WithoutExtra(keys...)
}

func (c *contentApplier) dropMCP(f *config.ContentFile) {
	pol := c.res.Policy.MCP
	keys := f.Metadata.ExtraKeysFor(config.FrontmatterMCPServers)
	for _, s := range inlineMCPServers(f.Metadata, keys) {
		switch {
		case slices.Contains(pol.DenyTransports, s.transport):
			c.report(f, "mcp.deny_transports", "%q declares MCP server %q on the %s transport, which the policy denies (origin: %s); its mcpServers are not loaded",
				f.Name, s.name, s.transport, c.res.Provenance["mcp.deny_transports"])
		case s.transport == config.TransportStdio && pol.AllowedCommands.Set && !slices.Contains(pol.AllowedCommands.Items, s.command):
			c.report(f, "mcp.allowed_commands", "%q declares MCP server %q running %q, which is not in mcp.allowed_commands %s (origin: %s); its mcpServers are not loaded",
				f.Name, s.name, s.command, quoteList(pol.AllowedCommands.Items), c.res.Provenance["mcp.allowed_commands"])
		default:
			continue
		}
		// One denied server makes the whole declaration untrustworthy: every
		// spelling of the key goes. The metadata is copied, never edited: it may be
		// shared with a load that runs without this policy.
		f.Metadata = f.Metadata.WithoutExtra(keys...)
		return
	}
}

// inlineServer is one MCP server an agent, skill or command defines inline.
type inlineServer struct {
	name, transport, command string
}

// inlineMCPServers decodes the `mcpServers` frontmatter in both documented
// shapes: a mapping of name to definition, and a list whose entries are a name
// (a server defined elsewhere, skipped) or a one-key mapping.
func inlineMCPServers(m *config.Metadata, keys []string) []inlineServer {
	defs := map[string]map[string]any{}
	for _, key := range keys {
		collectInlineServers(m, key, defs)
	}
	return sortedInlineServers(defs)
}

// collectInlineServers adds the servers the frontmatter key declares to defs.
func collectInlineServers(m *config.Metadata, key string, defs map[string]map[string]any) {
	raw, ok := m.TypedExtra(key)
	if !ok {
		return
	}
	node, isNode := raw.(*yaml.Node)
	if !isNode {
		return
	}
	var v any
	if node.Decode(&v) != nil {
		return
	}
	collect := func(name string, def any) {
		if d, ok := def.(map[string]any); ok {
			defs[name] = d
		}
	}
	switch t := v.(type) {
	case map[string]any:
		for name, def := range t {
			collect(name, def)
		}
	case []any:
		for _, entry := range t {
			if mm, ok := entry.(map[string]any); ok {
				for name, def := range mm {
					collect(name, def)
				}
			}
		}
	}
}

func sortedInlineServers(defs map[string]map[string]any) []inlineServer {
	names := make([]string, 0, len(defs))
	for name := range defs {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]inlineServer, 0, len(names))
	for _, name := range names {
		def := defs[name]
		command, _ := def["command"].(string)
		out = append(out, inlineServer{name: name, transport: inlineTransport(def, command), command: command})
	}
	return out
}

// inlineTransport reads the transport of an inline definition: an explicit type,
// else stdio when it runs a command and http when it names a URL.
func inlineTransport(def map[string]any, command string) string {
	t, _ := def["type"].(string)
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "sse":
		return config.TransportSSE
	case "http", "streamable-http", "streamablehttp":
		return config.TransportHTTP
	case "stdio":
		return config.TransportStdio
	}
	if command != "" {
		return config.TransportStdio
	}
	if _, hasURL := def["url"]; hasURL {
		return config.TransportHTTP
	}
	return config.TransportStdio
}
