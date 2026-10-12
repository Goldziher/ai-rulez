package lint

import (
	"path/filepath"
	"slices"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
)

// CodeRootComponentShipped warns that a single-plugin bundle's root is the
// repository root, so a component path the host loads by default ships to every
// user even though it is not part of the plugin (docs/plugins.md).
const CodeRootComponentShipped = "AR9T0"

// hostComponent is one path a host loads from a plugin root by default, the host
// that loads it, and the runtime whose bundle would own it when ai-rulez writes
// it.
type hostComponent struct {
	path    string
	host    string
	runtime string
	// bundled reports whether ai-rulez writes this path as part of the plugin
	// bundle for runtime, given the configuration. A path it writes is intended
	// payload, not a stray developer file, so it is not reported. Nil means the
	// path is never generated.
	bundled func(r *runner) bool
}

// The hosts that load a component from the plugin root (the finding names one).
const (
	hostClaudeCode = "Claude Code"
	hostCodex      = "Codex"
)

func registerRootComponents(s *ruleSet) {
	s.addRules(RuleInfo{CodeRootComponentShipped, "root-component-shipped", SeverityWarning,
		"a single-plugin bundle whose root is the repository root ships a default host component path (.mcp.json, hooks/, agents/, commands/, settings.json, bin/) to every user; set [plugin] content_root or move the plugin into a monorepo member"})
	s.addDocs(map[string]RuleDoc{
		CodeRootComponentShipped: {
			Why:  "A single-plugin repository lists its bundle with source \"./\", so the repository root becomes the installed plugin root. Claude Code and Codex then load their default component paths from the root — `.mcp.json`, `hooks/`, `agents/`, `commands/`, `settings.json`, `bin/` — even though they are a maintainer's own development files, not part of the plugin.",
			Bad:  "A repository with a `[plugin]` block and a committed `.mcp.json` or `agents/` directory, published with `source: \"./\"`",
			Good: "Set `[plugin] content_root` to keep the payload out of the root, or move the plugin into a monorepo member (`plugins/<name>/`) so its root is not the repository root",
		},
	})
	s.addRunCheck(checkRootComponents, AnalyzerPlugin)
}

// rootComponents lists the default component paths Claude Code and Codex load
// from a plugin root (the Claude Code plugin "Standard layout" and the Codex
// plugin defaults), the host that loads each, and whether ai-rulez's bundle for
// that runtime writes it. It is a function, not a package variable, so no
// mutable table is added to the package.
func rootComponents() []hostComponent {
	return []hostComponent{
		{path: ".mcp.json", host: hostClaudeCode, runtime: config.PluginRuntimeClaude, bundled: codexLegacyWritesMCP},
		{path: keyHooks, host: hostClaudeCode, runtime: config.PluginRuntimeClaude, bundled: pluginDeclaresHooks},
		{path: keyAgents, host: hostClaudeCode, runtime: config.PluginRuntimeClaude, bundled: pluginBundlesAgents},
		{path: keyCommands, host: hostClaudeCode, runtime: config.PluginRuntimeClaude, bundled: pluginBundlesCommands},
		{path: "settings.json", host: hostClaudeCode, runtime: config.PluginRuntimeClaude},
		{path: "bin", host: hostClaudeCode, runtime: config.PluginRuntimeClaude},
		{path: ".lsp.json", host: hostClaudeCode, runtime: config.PluginRuntimeClaude},
		{path: "monitors", host: hostClaudeCode, runtime: config.PluginRuntimeClaude},
		{path: "output-styles", host: hostClaudeCode, runtime: config.PluginRuntimeClaude},
		{path: "themes", host: hostClaudeCode, runtime: config.PluginRuntimeClaude},
		{path: "workflows", host: hostClaudeCode, runtime: config.PluginRuntimeClaude},
		{path: "SKILL.md", host: hostClaudeCode, runtime: config.PluginRuntimeClaude},
		{path: "mcp.json", host: hostCodex, runtime: config.PluginRuntimeCodex, bundled: codexRootWritesMCP},
		{path: ".app.json", host: hostCodex, runtime: config.PluginRuntimeCodex},
	}
}

// checkRootComponents warns (AR9T0) about each default host component path that
// exists at the repository root while a single-plugin bundle's root is that same
// directory. The repository root is the plugin root only for a single-plugin
// `[plugin]` block whose catalog source is "./": a monorepo root renders its
// members under their own directories, so its root paths are not the plugin.
func checkRootComponents(r *runner) {
	if !r.singlePluginRootIsRepoRoot() {
		return
	}
	for _, hc := range rootComponents() {
		if !slices.Contains(r.cfg.Plugin.ResolvedRuntimes(), hc.runtime) {
			continue
		}
		if hc.bundled != nil && hc.bundled(r) {
			continue
		}
		if !r.existsRepo(hc.path) {
			continue
		}
		r.add(CodeRootComponentShipped, filepath.Join(r.rootAbs(), filepath.FromSlash(hc.path)), 1,
			"%s exists at the repository root, which is this plugin's root (the catalog lists it as \"./\"), so %s loads it for every user who installs the plugin even though it is not part of the bundle. Set [plugin] content_root to keep the payload out of the root, or move the plugin into a monorepo member (plugins/<name>/)",
			hc.path, hc.host)
	}
}

// singlePluginRootIsRepoRoot reports whether generate --plugin writes a
// single-plugin bundle whose root is the repository root: a [plugin] block with
// no monorepo members and no domain plugins, targeting Claude or Codex (the
// hosts whose single-plugin catalog points at "./").
func (r *runner) singlePluginRootIsRepoRoot() bool {
	p := r.cfg.Plugin
	if p == nil {
		return false
	}
	if mkt := r.cfg.Marketplace; mkt != nil && (len(mkt.Members) > 0 || mkt.HasDomainPlugins()) {
		return false
	}
	for _, rt := range p.ResolvedRuntimes() {
		if rt == config.PluginRuntimeClaude || rt == config.PluginRuntimeCodex {
			return true
		}
	}
	return false
}

// pluginDeclaresHooks reports whether ai-rulez writes hooks/ for the bundle: the
// plugin declares at least one lifecycle hook.
func pluginDeclaresHooks(r *runner) bool { return len(r.cfg.Plugin.Hooks) > 0 }

// pluginBundlesAgents reports whether ai-rulez writes agents/ for the Claude
// bundle: the plugin bundles at least one agent.
func pluginBundlesAgents(r *runner) bool { return r.pluginBundlesKind(kindAgent) }

// pluginBundlesCommands reports whether ai-rulez writes commands/ for the Claude
// bundle: the plugin bundles at least one command.
func pluginBundlesCommands(r *runner) bool { return r.pluginBundlesKind(kindCommand) }

// pluginBundlesKind reports whether the plugin bundles at least one item of
// kind. A content_root or include_domains moves or extends the payload beyond
// the scanned tree, so the check stays quiet rather than guess.
func (r *runner) pluginBundlesKind(kind string) bool {
	p := r.cfg.Plugin
	if p == nil {
		return false
	}
	if p.ContentRoot != "" || len(p.IncludeDomains) > 0 {
		return true // the payload comes from elsewhere; do not guess
	}
	for i := range r.items {
		it := &r.items[i]
		if it.owned && !it.isDoc && it.domain == "" && it.kind == kind {
			return true
		}
	}
	return false
}

// codexLegacyWritesMCP reports whether ai-rulez writes .mcp.json for the bundle:
// the Codex runtime is targeted, uses its legacy layout, and the plugin declares
// MCP servers.
func codexLegacyWritesMCP(r *runner) bool {
	p := r.cfg.Plugin
	return p != nil && slices.Contains(p.ResolvedRuntimes(), config.PluginRuntimeCodex) &&
		p.Codex.ManifestLayout() == config.CodexManifestLegacy && pluginHasMCP(r)
}

// codexRootWritesMCP reports whether ai-rulez writes mcp.json for the bundle:
// the Codex runtime is targeted, uses the root layout, and the plugin declares
// MCP servers.
func codexRootWritesMCP(r *runner) bool {
	p := r.cfg.Plugin
	return p != nil && slices.Contains(p.ResolvedRuntimes(), config.PluginRuntimeCodex) &&
		p.Codex.ManifestLayout() != config.CodexManifestLegacy && pluginHasMCP(r)
}

// pluginHasMCP reports whether the plugin bundles any MCP server, from the
// authored [[plugin.mcp]] entries or the project's [[mcp_servers]].
func pluginHasMCP(r *runner) bool {
	p := r.cfg.Plugin
	return p != nil && (len(p.MCP) > 0 || len(r.cfg.MCPServers) > 0)
}
