// Package plugin implements the *authoring* (producer) side of plugins: it
// packages an ai-rulez project's content tree and metadata into distributable
// plugin bundles + a marketplace index for the Claude, Cursor, Codex, Gemini,
// Kimi, OpenCode, and Factory runtimes.
//
// It is deliberately separate from the per-runtime content presets in
// internal/generator/presets and internal/generator/providers, because plugin
// packaging spans all runtimes with runtime-specific manifest shapes rather
// than mirroring a single provider's output.
package plugin

import (
	"sort"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/samber/oops"
)

// Manifest is the resolved, runtime-agnostic plugin model. It is built once
// from the config + content tree and consumed by every runtime renderer.
type Manifest struct {
	Name        string
	DisplayName string
	Description string
	Version     string
	Author      *config.Author
	Homepage    string
	Repository  string
	License     string
	Category    string
	BrandColor  string
	Icon        string
	Logo        string
	Keywords    []string
	Tags        []string

	// Runtimes are the target runtimes to emit, already resolved (never empty).
	Runtimes []string

	// MCP holds the plugin's bundled MCP servers using the canonical
	// ${PLUGIN_ROOT} launch variable; renderers rewrite it per runtime.
	MCP []config.PluginMCPLaunch

	Hooks      []config.HookGroup
	Statusline *config.Statusline
	Interface  *config.PluginInterface
	Gemini     *config.GeminiExtras
	Kimi       *config.KimiExtras

	// Content bundled into the plugin (skills/, commands/, agents/).
	Skills   []config.ContentFile
	Commands []config.ContentFile
	Agents   []config.ContentFile

	// Market describes the marketplace index emitted alongside the plugin.
	Market MarketInfo

	// SourceDir is the source project's base directory, used to resolve
	// passthrough asset/script paths.
	SourceDir string
}

// MarketInfo is the resolved marketplace metadata for the emitted index.
type MarketInfo struct {
	Name        string
	Description string
	Owner       *config.Author
	// Members lists monorepo sub-project directories; empty means single-plugin.
	Members []string
}

// BuildManifest resolves a Manifest from the config's [plugin] block, the
// project's content tree, and any [marketplace] block. Returns an error if no
// [plugin] block is configured.
func BuildManifest(cfg *config.Config, content *config.ContentTree) (*Manifest, error) {
	p := cfg.Plugin
	if p == nil {
		return nil, oops.
			Hint("Add a [plugin] block to your config to author a plugin bundle").
			Errorf("no [plugin] block configured")
	}

	m := &Manifest{
		Name:        p.Name,
		DisplayName: p.DisplayName,
		Description: p.Description,
		Version:     p.Version,
		Author:      p.Author,
		Homepage:    p.Homepage,
		Repository:  p.Repository,
		License:     p.License,
		Category:    p.Category,
		BrandColor:  p.BrandColor,
		Icon:        p.Icon,
		Logo:        p.Logo,
		Keywords:    p.Keywords,
		Tags:        p.Tags,
		Runtimes:    p.ResolvedRuntimes(),
		MCP:         resolveMCP(p, cfg),
		Hooks:       p.Hooks,
		Statusline:  p.Statusline,
		Interface:   p.Interface,
		Gemini:      p.Gemini,
		Kimi:        p.Kimi,
		Market:      resolveMarket(p, cfg.Marketplace),
		SourceDir:   cfg.BaseDir,
	}

	if content != nil {
		m.Skills = content.Skills
		m.Commands = content.Commands
		m.Agents = content.Agents
	}

	return m, nil
}

// resolveMCP returns the plugin's bundled MCP servers: the explicit
// [[plugin.mcp]] entries if present, otherwise the project's resolved
// [[mcp_servers]] mapped to the canonical launch shape.
func resolveMCP(p *config.PluginAuthoring, cfg *config.Config) []config.PluginMCPLaunch {
	if len(p.MCP) > 0 {
		return p.MCP
	}
	if len(cfg.MCPServers) == 0 {
		return nil
	}
	names := make([]string, 0, len(cfg.MCPServers))
	for name := range cfg.MCPServers {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]config.PluginMCPLaunch, 0, len(names))
	for _, name := range names {
		s := cfg.MCPServers[name]
		out = append(out, config.PluginMCPLaunch{
			Name:      s.Name,
			Command:   s.Command,
			Args:      s.Args,
			Env:       s.Env,
			Transport: s.GetTransport(),
			URL:       s.URL,
		})
	}
	return out
}

// ResolveMarketInfo builds MarketInfo from a standalone [marketplace] block,
// used for a monorepo root that has no [plugin] block of its own.
func ResolveMarketInfo(mkt *config.MarketplaceAuthoring) MarketInfo {
	if mkt == nil {
		return MarketInfo{}
	}
	return MarketInfo{
		Name:        mkt.Name,
		Description: mkt.Description,
		Owner:       mkt.Owner,
		Members:     mkt.Members,
	}
}

// resolveMarket derives the marketplace metadata. When no [marketplace] block
// is present it synthesizes a single-plugin marketplace from the plugin's own
// name/description/author (matching how single-plugin repos are published).
func resolveMarket(p *config.PluginAuthoring, mkt *config.MarketplaceAuthoring) MarketInfo {
	if mkt == nil {
		return MarketInfo{
			Name:        p.Name,
			Description: p.Description,
			Owner:       p.Author,
		}
	}
	owner := mkt.Owner
	if owner == nil {
		owner = p.Author
	}
	desc := mkt.Description
	if desc == "" {
		desc = p.Description
	}
	return MarketInfo{
		Name:        mkt.Name,
		Description: desc,
		Owner:       owner,
		Members:     mkt.Members,
	}
}
