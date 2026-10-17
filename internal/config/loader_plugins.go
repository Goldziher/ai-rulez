package config

import (
	"context"
	"errors"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

// resolveInstalledPluginsIfNeeded resolves [[installed_marketplaces]] and
// [[installed_plugins]] and merges what the plugins deliver into the content
// tree: local content wins over a plugin's, an earlier plugin over a later one.
func resolveInstalledPluginsIfNeeded(ctx context.Context, config *Config, resolve ResolveInstalledPluginsCallback) error {
	log := logger.FromContext(ctx)
	if len(config.InstalledMarketplaces) == 0 && len(config.InstalledPlugins) == 0 {
		return nil
	}
	if resolve == nil {
		return oops.
			Hint("Installed plugins are configured but the load was given no resolver; pass config.WithResolvers (the CLI and the MCP server do).").
			Errorf("installed plugins configured but resolver is unavailable")
	}
	res, err := resolve(ctx, config)
	if err != nil {
		if errors.Is(err, ErrLockViolation) || errors.Is(err, ErrPluginUnresolved) {
			return err
		}
		log.Warn("Failed to resolve installed plugins", "error", err)
		return nil
	}
	if res == nil {
		return nil
	}
	if config.Content == nil {
		config.Content = &ContentTree{Domains: make(map[string]*Domain)}
	}
	m := &pluginMerger{cfg: config, log: log, taken: map[string]bool{}, seeded: map[string]bool{}}
	m.files(ItemKindSkill, res.Skills)
	m.files(string(OutputKindAgent), res.Agents)
	m.files(ItemKindCommand, res.Commands)
	m.files("rule", res.Rules)
	m.hooks(res.Hooks)
	m.mcp(res.MCP)
	config.ImportedScan = append(config.ImportedScan, res.Scan...)
	log.Debug("Merged installed plugins", "skills", len(res.Skills), "agents", len(res.Agents), "commands", len(res.Commands), "rules", len(res.Rules),
		"hooks", len(res.Hooks), "mcp", len(res.MCP))
	return nil
}

type pluginMerger struct {
	cfg *Config
	log logger.Logger
	// taken holds kind+name of every file placed so far, local ones included;
	// seeded notes the kinds whose existing files are already in it.
	taken, seeded map[string]bool
}

func (m *pluginMerger) existing(kind string) []ContentFile {
	t := m.cfg.Content
	switch kind {
	case ItemKindSkill:
		return t.Skills
	case string(OutputKindAgent):
		return t.Agents
	case ItemKindCommand:
		return t.Commands
	}
	return t.Rules
}

func (m *pluginMerger) domainList(kind string, d *Domain) *[]ContentFile {
	switch kind {
	case ItemKindSkill:
		return &d.Skills
	case string(OutputKindAgent):
		return &d.Agents
	case ItemKindCommand:
		return &d.Commands
	}
	return &d.Rules
}

func (m *pluginMerger) rootList(kind string) *[]ContentFile {
	t := m.cfg.Content
	switch kind {
	case ItemKindSkill:
		return &t.Skills
	case string(OutputKindAgent):
		return &t.Agents
	case ItemKindCommand:
		return &t.Commands
	}
	return &t.Rules
}

// files merges one kind. Names are unique per kind across the root and the
// domains, the scope generation keys outputs by.
func (m *pluginMerger) files(kind string, files []ContentFile) {
	if len(files) == 0 {
		return
	}
	if !m.seeded[kind] {
		m.seeded[kind] = true
		existing := m.existing(kind)
		for i := range existing {
			m.taken[kind+"\x00"+existing[i].Name] = true
		}
		for _, d := range m.cfg.Content.Domains {
			list := *m.domainList(kind, d)
			for i := range list {
				m.taken[kind+"\x00"+list[i].Name] = true
			}
		}
	}
	for i := range files {
		f := files[i]
		key := kind + "\x00" + f.Name
		if m.taken[key] {
			if m.cfg.OncePerRun("plugin-conflict\x00" + key) {
				m.log.Warn("Installed plugin "+kind+" name conflicts with an existing one, skipping", "name", f.Name)
			}
			continue
		}
		m.taken[key] = true
		if f.Domain != "" {
			if d, ok := m.cfg.Content.Domains[f.Domain]; ok {
				l := m.domainList(kind, d)
				*l = append(*l, f)
				continue
			}
			m.log.Warn("Installed plugin names a domain that does not exist; placing it in the root content", "name", f.Name, "domain", f.Domain)
		}
		l := m.rootList(kind)
		*l = append(*l, f)
	}
}

func (m *pluginMerger) hooks(groups []HookGroup) {
	for i := range groups {
		g := groups[i]
		g.Builtin = HookBuiltinPlugin
		m.cfg.Hooks = append(m.cfg.Hooks, g)
	}
}

func (m *pluginMerger) mcp(servers []MCPServer) {
	if len(servers) == 0 {
		return
	}
	if m.cfg.MCPServers == nil {
		m.cfg.MCPServers = map[string]*MCPServer{}
	}
	for i := range servers {
		s := servers[i]
		if _, dup := m.cfg.MCPServers[s.Name]; dup {
			if m.cfg.OncePerRun("plugin-mcp-conflict\x00" + s.Name) {
				m.log.Warn("Installed plugin MCP server name conflicts with an existing one, skipping", "name", s.Name)
			}
			continue
		}
		m.cfg.MCPServers[s.Name] = &s
		m.cfg.PluginMCPServers = append(m.cfg.PluginMCPServers, s.Name)
	}
}
