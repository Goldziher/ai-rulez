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
	"context"
	"path/filepath"
	"slices"
	"sort"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/logger"
)

// Manifest is the resolved, runtime-agnostic plugin model. It is built once
// from the config + content tree and consumed by every runtime renderer.
type Manifest struct {
	// Ctx bounds the git questions bundling a skill asks. The generator sets it;
	// nil runs them unbounded.
	Ctx         context.Context
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

	// Spec is the Agent Plugins version of the standard-based bundles; empty
	// means agentplugins.DefaultSpec.
	Spec string

	// PackageManager is emitted as packageManager in the OpenCode and Pi root
	// package.json; empty omits the field.
	PackageManager string

	// OpencodeDependency overrides the "@opencode/plugin" dependency version in
	// the OpenCode root package.json; empty uses the built-in default.
	OpencodeDependency string

	// Runtimes are the target runtimes to emit, already resolved (never empty).
	Runtimes []string

	// MCP holds the plugin's bundled MCP servers using the canonical
	// ${PLUGIN_ROOT} launch variable; renderers rewrite it per runtime.
	MCP []config.PluginMCPLaunch

	Hooks      []config.HookGroup
	Statusline *config.Statusline
	Interface  *config.PluginInterface
	Codex      *config.CodexExtras
	Cursor     *config.CursorExtras
	Gemini     *config.GeminiExtras
	Kimi       *config.KimiExtras
	Hermes     *config.HermesExtras

	// Content bundled into the plugin (skills/, commands/, agents/).
	Skills   []config.ContentFile
	Commands []config.ContentFile
	Agents   []config.ContentFile

	// Include lists the [plugin] include globs (relative to the content root)
	// whose matched files are copied verbatim into the Agent Plugins package.
	Include []string

	// SharedResources lists the resolved [plugin] shared_resources directories
	// whose files are copied into every bundled skill.
	SharedResources []config.SharedResource

	// PiSkillsDir is where the Pi runtime writes each bundled skill, relative to
	// the project root (".pi/skills" by default). PiPackageJSON is "generate" or
	// "merge".
	PiSkillsDir   string
	PiPackageJSON string
	// PiPackageName is the explicit npm name ("" derives it); PiSkillsEntry is
	// "each" or "directory".
	PiPackageName string
	PiSkillsEntry string

	// ProvenanceHeaders is nil to write the AI-RULEZ generated-file header into
	// bundled comment-safe outputs (the default), or an explicit false to write
	// them byte-identical to their sources.
	ProvenanceHeaders *bool

	// IncludeEvals bundles eval cases: each skill's evals/ directory and the
	// project-level eval tree at EvalsDir.
	IncludeEvals bool
	// EvalsDir is the project-level eval tree (.ai-rulez/evals). May not exist.
	EvalsDir string
	// EvalsPerSkillOnly limits the project-level tree to the <skill-name>/
	// subdirectories of bundled skills. Set for domain plugins, whose bundle
	// must not carry another plugin's cases.
	EvalsPerSkillOnly bool

	// Market describes the marketplace index emitted alongside the plugin.
	Market MarketInfo

	// Config is the resolved project config, used for settings that resolve
	// through project defaults (such as the per-preset agent model).
	Config *config.Config

	// SourceDir is the source project's base directory, used to resolve
	// passthrough asset/script paths.
	SourceDir string
}

// log is where the renderers report: the log of the project config the manifest
// was built from (the CLI's when it has none).
func (m *Manifest) log() logger.Logger { return m.Config.Log() }

// writesProvenanceHeaders reports whether bundled comment-safe outputs carry the
// AI-RULEZ generated-file header. It defaults to true: only an explicit false
// (from [plugin] provenance_headers = false) turns the headers off.
func (m *Manifest) writesProvenanceHeaders() bool {
	return m == nil || m.ProvenanceHeaders == nil || *m.ProvenanceHeaders
}

// piSkillsDir returns the Pi skills directory, defaulting when unset.
func (m *Manifest) piSkillsDir() string {
	if m == nil || m.PiSkillsDir == "" {
		return config.DefaultPiSkillsDir
	}
	return m.PiSkillsDir
}

// mergesPiPackageJSON reports whether the Pi runtime merges the root
// package.json instead of generating it.
func (m *Manifest) mergesPiPackageJSON() bool {
	return m != nil && m.PiPackageJSON == config.PiPackageJSONMerge
}

// managedOutputs hashes runtime-owned keys in a merged package.json while
// allowing author-owned tooling to change. A combined OpenCode package also
// owns its entrypoint and dependency metadata. ~keep
func (m *Manifest) managedOutputs() map[string][]string {
	if !m.mergesPiPackageJSON() {
		return nil
	}
	keys := piOwnedKeys()
	if slices.Contains(m.Runtimes, config.PluginRuntimeOpenCode) {
		keys = append(keys, openCodeOwnedKeys()...)
	}
	return map[string][]string{"package.json": keys}
}

// openCodeOwnedKeys are the package.json keys a combined OpenCode package owns
// beyond Pi's: the only ones the OpenCode manifest may overwrite in the merged
// file. Every other key must agree between the runtimes.
func openCodeOwnedKeys() []string {
	return []string{"type", "main", "exports", "dependencies", "homepage", "license", "packageManager"}
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

	version, err := p.ResolvedVersion(cfg.BaseDir)
	if err != nil {
		return nil, err
	}
	if err := p.Pi.Validate(); err != nil {
		return nil, err
	}
	shared, err := p.ResolveSharedResources(cfg)
	if err != nil {
		return nil, err
	}

	m := &Manifest{
		Name:        p.Name,
		DisplayName: p.DisplayName,
		Description: p.Description,
		Version:     version,
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
		Spec:        p.Spec,
		Runtimes:    p.ResolvedRuntimes(),
		MCP:         resolveMCP(p, cfg, p.ResolvedRuntimes()),
		Hooks:       p.Hooks,
		Statusline:  p.Statusline,
		Interface:   p.Interface,
		Codex:       p.Codex,
		Cursor:      p.Cursor,
		Gemini:      p.Gemini,
		Kimi:        p.Kimi,
		Hermes:      p.Hermes,
		Market:      resolveMarket(p, cfg.Marketplace),
		Config:      cfg,
		SourceDir:   cfg.BaseDir,

		IncludeEvals:       p.IncludeEvals,
		EvalsDir:           evalsDir(cfg, p),
		Include:            p.Include,
		SharedResources:    shared,
		ProvenanceHeaders:  p.ProvenanceHeaders,
		PackageManager:     p.PackageManager,
		OpencodeDependency: p.OpencodeDependency,
		PiSkillsDir:        p.Pi.ResolvedSkillsDir(),
		PiPackageJSON:      p.Pi.ResolvedPackageJSON(),
		PiPackageName:      piPackageName(p.Pi),
		PiSkillsEntry:      p.Pi.ResolvedSkillsEntry(),
	}

	if content != nil {
		m.Skills, m.Commands, m.Agents = includeDomainContent(content, p.IncludeDomains)
	}

	return m, nil
}

// resolveMCP returns the plugin's bundled MCP servers: the explicit
// [[plugin.mcp]] entries if present, otherwise the project's resolved
// [[mcp_servers]] mapped to the canonical launch shape.
func resolveMCP(p *config.PluginAuthoring, cfg *config.Config, runtimes []string) []config.PluginMCPLaunch {
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
		if len(s.Headers) > 0 {
			cfg.Warn("MCP server headers are not included in plugin bundles", "server", name)
		}
		if !s.IsEnabled() && slices.ContainsFunc(runtimes, func(r string) bool { return r != config.PluginRuntimeOpenCode }) {
			cfg.Warn("A disabled MCP server is bundled enabled; only the OpenCode bundle carries the disabled flag",
				"server", name)
		}
		out = append(out, config.PluginMCPLaunch{
			Name:      s.Name,
			Command:   s.Command,
			Args:      s.Args,
			Env:       s.Env,
			Transport: s.GetTransport(),
			URL:       s.URL,
			Disabled:  !s.IsEnabled(),
		})
	}
	return out
}

// resolveNamedMCP maps the named root MCP servers to the canonical launch shape.
// A name with no resolved server is skipped here; the generator rejects it before
// a manifest is built.
func resolveNamedMCP(cfg *config.Config, names, runtimes []string) []config.PluginMCPLaunch {
	if len(names) == 0 {
		return nil
	}
	picked := make(map[string]*config.MCPServer, len(names))
	for _, name := range names {
		if s, ok := cfg.MCPServers[name]; ok {
			picked[name] = s
		}
	}
	scoped := *cfg
	scoped.MCPServers = picked
	return resolveMCP(&config.PluginAuthoring{}, &scoped, runtimes)
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

// evalsDir is the project-level eval tree: evals/ under the plugin content root
// when one is configured, otherwise under the config directory.
func evalsDir(cfg *config.Config, p *config.PluginAuthoring) string {
	if p != nil && p.ContentRoot != "" {
		return filepath.Join(cfg.BaseDir, p.ContentRoot, config.EvalsDirName)
	}
	return filepath.Join(cfg.ConfigDir, config.EvalsDirName)
}

// piPackageName returns the explicit [plugin.pi] package_name, or "". Safe on a
// nil receiver.
func piPackageName(e *config.PiExtras) string {
	if e == nil {
		return ""
	}
	return e.PackageName
}
