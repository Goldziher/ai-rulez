package providers

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/config"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/docmerge"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/jsonmerge"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/presets"
	"github.com/Goldziher/ai-rulez/v5/internal/generator/settings"
	"github.com/Goldziher/ai-rulez/v5/schema"
)

// presetsResolveGlobalEffort is aliased so sidecar code can stay readable
// when calling the shared resolver from the presets package.
var presetsResolveGlobalEffort = presets.ResolveGlobalEffort

const (
	// settingsKeyMCPServers is the only top-level key the settings-style JSON
	// sidecars (.claude/settings.json, .mcp.json) own. Every other key in those
	// documents is hand-authored by the consumer and must survive generation.
	settingsKeyMCPServers = "mcpServers"

	// settingsKeyExtraKnownMarketplaces and settingsKeyEnabledPlugins are the
	// plugin keys of .claude/settings.json ai-rulez owns, entry by entry, when
	// [claude.settings] manage is set.
	settingsKeyExtraKnownMarketplaces = "extraKnownMarketplaces"
	settingsKeyEnabledPlugins         = "enabledPlugins"

	// sourceKey is the key naming a marketplace source's kind, and the key holding
	// the source object in an extraKnownMarketplaces entry.
	sourceKey = "source"

	// ampSettingsKeyEffort is the only top-level key ai-rulez owns in
	// .amp/settings.json; users keep arbitrary Amp settings alongside it.
	ampSettingsKeyEffort = "amp.anthropic.effort"

	// ampSettingsKeyMCPServers is the flat key holding Amp's MCP servers.
	ampSettingsKeyMCPServers = "amp.mcpServers"

	// piMCPKeyServers is the only top-level key ai-rulez owns in .pi/mcp.json.
	piMCPKeyServers = "mcpServers"

	// arraySidecarIndent is the indentation for the one sidecar ai-rulez owns
	// outright (.claude/plugins.json, a JSON array). Object-shaped sidecars take
	// their indentation from jsonmerge instead, which adapts to the document that
	// is already on disk.
	arraySidecarIndent = "  "
)

// evalPredicate dispatches the closed-set emit_when value. Most predicates
// only consult Config, but has_resolved_effort also needs the spec's
// effort_map to know whether the global tier translates to a non-empty
// emitted value — so the predicate is a method on Generator.
func (g *Generator) evalPredicate(predicate string, cfg *config.Config) bool {
	switch predicate {
	case "", PredicateAlways:
		return true
	case PredicateHasMCPServers:
		return cfg != nil && len(cfg.MCPServers) > 0
	case PredicateHasMCPServersOrPluginSettings:
		return cfg != nil && (len(cfg.MCPServers) > 0 || cfg.ManagesClaudeSettings())
	case PredicateHasClaudeSettings:
		// MCP servers are not a reason: Claude Code reads them from .mcp.json.
		return cfg != nil && (cfg.ManagesClaudeSettings() || cfg.HasClaudeSettingsContent())
	case PredicateHasHooks:
		return cfg != nil && cfg.HasSettingsHooks()
	case PredicateHasPermissions:
		return cfg != nil && !cfg.Permissions.IsEmpty()
	case PredicateHasMCPJSONEntries:
		return cfg != nil && (len(cfg.MCPServers) > 0 || cfg.HasSelfServer())
	case PredicateHasPlugins:
		return cfg != nil && len(cfg.Plugins) > 0
	case PredicateHasResolvedEffortOrMCPServers:
		return cfg != nil && (len(cfg.MCPServers) > 0 || g.resolveGlobalEffort(cfg) != "")
	case PredicateHasResolvedEffort:
		return g.resolveGlobalEffort(cfg) != ""
	}
	return false
}

// sidecarRender is the outcome of rendering one sidecar: the body to write, plus
// whether the document turned out to be shared with the consumer.
//
// An alias rather than its own struct so a sidecar renderer can return what
// jsonmerge.Apply produced without restating it; see jsonmerge.Result for why
// PartiallyOwned is derived from the document's contents rather than from the
// sidecar kind.
type sidecarRender = jsonmerge.Result

// renderSidecar dispatches the closed-set sidecar kind. Method on Generator so
// kind-specific renderers (e.g. amp_settings_json) can read the spec's
// effort_map. outputPath is the file this sidecar is about to be written to, so
// an object-shaped kind can read what is already there and merge into it.
//
// Sidecars like .claude/settings.json are shared documents: ai-rulez owns one
// key and the consumer owns the rest, including tracked settings such as
// permissions, hooks and skillOverrides. Rendering them from scratch destroyed
// everything ai-rulez does not own (#185), so every object-shaped sidecar goes
// through jsonmerge.Apply.
func (g *Generator) renderSidecar(kind string, cfg *config.Config, outputPath string) (sidecarRender, error) {
	switch kind {
	case SidecarClaudeSettingsJSON:
		owned, err := claudeSettingsOwnedKeys(cfg, outputPath)
		if err != nil {
			return sidecarRender{}, err
		}
		return jsonmerge.ApplyWith(cfg.ReadExisting, outputPath, owned)
	case SidecarMCPJSON:
		return jsonmerge.ApplyWith(cfg.ReadExisting, outputPath, mcpJSONOwnedKeys(cfg, g.Spec != nil && presets.IsLiteralMCPJSONWriter(g.Spec.Name)))
	case SidecarAmpSettingsJSON:
		return jsonmerge.ApplyWith(cfg.ReadExisting, outputPath, g.ampSettingsOwnedKeys(cfg))
	case SidecarPiMCPJSON:
		return jsonmerge.ApplyWith(cfg.ReadExisting, outputPath, []jsonmerge.OwnedKey{
			{Name: piMCPKeyServers, Value: piMCPServerEntries(cfg), Members: true},
		})
	case SidecarClaudePluginsJSON:
		// .claude/plugins.json is a JSON array wholly owned by ai-rulez: there
		// are no user-authored sibling keys to preserve.
		body, err := renderClaudePluginsJSON(cfg)
		return sidecarRender{Body: body}, err
	}
	return sidecarRender{}, fmt.Errorf("unknown sidecar kind %q", kind)
}

// SidecarIsMergedDocument reports whether a sidecar kind produces a document
// (a JSON object, or for the generic kinds a JSON, TOML or YAML one) that ai-rulez
// merges into rather than replaces — a document where it owns a fixed set of
// keys and the consumer may own others.
//
// Unlike jsonmerge.Result.PartiallyOwned this is a static property of the kind, and
// it is deliberately the coarser test. Stale cleanup runs from the previous
// run's manifest, which is a list of plain paths with no record of what the
// document contained, and a manifest written by an older ai-rulez lists these
// files unconditionally. Refusing to delete any merged document by manifest
// entry can at worst leave a wholly generated .mcp.json behind; the alternative
// deletes a hand-authored settings file.
func SidecarIsMergedDocument(kind string) bool {
	switch kind {
	case SidecarClaudeSettingsJSON, SidecarMCPJSON, SidecarAmpSettingsJSON, SidecarPiMCPJSON:
		return true
	}
	// The generic kinds merge into whatever document format the spec names.
	return isGenericSidecarKind(kind)
}

// aggregateChecksPath is the base-relative path of the file a spec's aggregate
// checks output merges its marker block into, or "".
func aggregateChecksPath(spec *ProviderSpec) string {
	out := spec.Outputs[OutputTypeChecks]
	if out == nil || out.Mode != OutputModeAggregate || out.File == "" {
		return ""
	}
	return filepath.ToSlash(out.File)
}

// MergedSidecarPaths returns every base-relative, slash-separated path that a
// builtin provider spec declares as a merged JSON document (see
// SidecarIsMergedDocument). Derived from the embedded specs so the set cannot
// drift from them.
func MergedSidecarPaths() []string {
	seen := make(map[string]bool)
	for _, spec := range loadBuiltinSpecs() {
		for _, sidecar := range spec.Sidecars {
			if sidecar == nil || !sidecarIsMerged(sidecar) {
				continue
			}
			seen[filepath.ToSlash(sidecar.Path)] = true
		}
		if path := aggregateChecksPath(spec); path != "" {
			seen[path] = true
		}
	}
	paths := make([]string, 0, len(seen))
	for path := range seen {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	return paths
}

// resolveGlobalEffort runs the shared global effort resolver and translates
// through the provider's effort_map. Empty string when no global effort
// applies for this provider.
func (g *Generator) resolveGlobalEffort(cfg *config.Config) string {
	raw := presetsResolveGlobalEffort(g.Spec.Name, cfg)
	if raw == "" || g.Spec.EffortMap == nil {
		return ""
	}
	if mapped, ok := g.Spec.EffortMap.Values[raw]; ok {
		return mapped
	}
	return ""
}

// mcpJSONServerEntries builds the .mcp.json server map. Lifted verbatim from
// the legacy MCPPresetGenerator.Generate body so the output is byte-identical.
// `disabled` is emitted unconditionally (true or false), not only when the
// server is disabled.
func mcpJSONServerEntries(cfg *config.Config, literal bool) map[string]any {
	mcpServers := make(map[string]any)
	if cfg == nil {
		return mcpServers
	}
	for name, server := range cfg.MCPServers {
		entry := map[string]any{
			"disabled": !server.IsEnabled(),
		}
		applyMCPTransport(entry, server)
		if !literal {
			presets.ApplySharedMCPJSONRefs(entry, server, cfg)
		}
		mcpServers[name] = entry
	}
	return mcpServers
}

// mcpJSONOwnedKeys decides what ai-rulez owns in .mcp.json.
//
// With [[mcp_servers]] declared it owns the whole mcpServers object, as it
// always has, and the self-server entry (when enabled and not declared by name)
// is one more member of it. With only [mcp] self_server it owns just the single
// mcpServers.ai-rulez entry, so servers already in the file survive.
func mcpJSONOwnedKeys(cfg *config.Config, literal bool) []jsonmerge.OwnedKey {
	if cfg == nil || !cfg.HasSelfServer() {
		return []jsonmerge.OwnedKey{{Name: settingsKeyMCPServers, Value: mcpJSONServerEntries(cfg, literal), Members: true}}
	}
	self := cfg.SelfMCPServerEntry(schema.Version)
	if len(cfg.MCPServers) == 0 {
		return []jsonmerge.OwnedKey{{
			Path:  []string{settingsKeyMCPServers, config.SelfMCPServerName},
			Value: self,
		}}
	}
	servers := mcpJSONServerEntries(cfg, literal)
	if _, declared := servers[config.SelfMCPServerName]; !declared {
		servers[config.SelfMCPServerName] = self
	}
	return []jsonmerge.OwnedKey{{Name: settingsKeyMCPServers, Value: servers, Members: true}}
}

// ampSettingsOwnedKeys decides what ai-rulez owns in .amp/settings.json: the
// resolved global effort and, member by member, the MCP servers under the flat
// "amp.mcpServers" key (the standard entry without the description Amp ignores).
func (g *Generator) ampSettingsOwnedKeys(cfg *config.Config) []jsonmerge.OwnedKey {
	var owned []jsonmerge.OwnedKey
	if effort := g.resolveGlobalEffort(cfg); effort != "" {
		owned = append(owned, jsonmerge.OwnedKey{Name: ampSettingsKeyEffort, Value: effort})
	}
	if cfg != nil && len(cfg.MCPServers) > 0 {
		owned = append(owned, jsonmerge.OwnedKey{
			Name: ampSettingsKeyMCPServers, Members: true,
			Value: mcpDialectEntriesFor(mcpDialects[MCPDialectAmp], cfg, &mcpEntryOpts{refSyntax: EnvRefSyntaxBraced}),
		})
	}
	return owned
}

// claudeSettingsOwnedKeys decides what ai-rulez owns in .claude/settings.json:
// with [claude.settings] manage = true the marketplace registration and plugin
// switches. MCP servers are not written here: Claude Code reads project servers
// from .mcp.json, and settings.json only has enableAllProjectMcpServers and
// enabledMcpjsonServers to approve those, so a mcpServers key in it would be dead
// weight carrying resolved secrets into a file teams commit. The plugin keys are owned entry
// by entry (Members), so every other marketplace and plugin the file lists
// survives, and an entry dropped from the config is removed by the previous
// run's ownership record. A mcpServers entry an earlier version wrote is taken back
// by that record.
//
// [[hooks]], [permissions] and [claude.settings.managed] add their own keys (see
// package settings): hooks and permission rules element by element, env and
// skillOverrides entry by entry. User scope owns those keys alone: MCP servers
// and plugin registration are project concepts.
func claudeSettingsOwnedKeys(cfg *config.Config, outputPath string) ([]jsonmerge.OwnedKey, error) {
	var owned []jsonmerge.OwnedKey
	extra, err := settings.ClaudeKeys(cfg, outputPath)
	if err != nil {
		return nil, fmt.Errorf("render .claude/settings.json settings: %w", err)
	}
	owned = append(owned, extra...)
	if !cfg.ManagesClaudeSettings() || cfg.UserScope {
		return owned, nil
	}
	s := cfg.Claude.Settings
	market := ""
	if cfg.Marketplace != nil {
		market = cfg.Marketplace.Name
	}
	if s.RegistersMarketplace() {
		owned = append(owned, jsonmerge.OwnedKey{
			Name:    settingsKeyExtraKnownMarketplaces,
			Value:   map[string]any{market: marketplaceSettingsEntry(cfg, s)},
			Members: true,
		})
	}
	plugins := map[string]any{}
	for _, name := range s.EnablePlugins {
		plugins[name+"@"+market] = true
	}
	for _, name := range s.DisablePlugins {
		plugins[name+"@"+market] = false
	}
	if len(plugins) > 0 {
		owned = append(owned, jsonmerge.OwnedKey{Name: settingsKeyEnabledPlugins, Value: plugins, Members: true})
	}
	return owned, nil
}

// marketplaceSettingsEntry builds the extraKnownMarketplaces value. Without an
// explicit marketplace_source it points a `directory` source at the marketplace
// output directory, relative to the repository (Claude Code resolves it against
// the main checkout).
func marketplaceSettingsEntry(cfg *config.Config, s *config.ClaudeSettings) map[string]any {
	source := map[string]any{sourceKey: "directory", "path": marketplaceDirPath(cfg)}
	if src := s.MarketplaceSource; src != nil {
		source = map[string]any{sourceKey: src.Source}
		for key, value := range map[string]string{"path": src.Path, "repo": src.Repo, "url": src.URL, "ref": src.Ref} {
			if value != "" {
				source[key] = value
			}
		}
	}
	entry := map[string]any{sourceKey: source}
	if s.AutoUpdate != nil {
		entry["autoUpdate"] = *s.AutoUpdate
	}
	return entry
}

// marketplaceDirPath is the marketplace root as a repository-relative path.
func marketplaceDirPath(cfg *config.Config) string {
	dir := "."
	if cfg.Marketplace != nil && cfg.Marketplace.OutputDir != "" {
		dir = filepath.ToSlash(filepath.Clean(cfg.Marketplace.OutputDir))
	}
	if dir != "." && !strings.HasPrefix(dir, "./") {
		dir = "./" + dir
	}
	return dir
}

// legacyClaudeMCPServerEntries is the server map earlier versions wrote to
// .claude/settings.json. It only recognises what they left behind (see
// serverLegacyClaims); nothing renders it any more.
func legacyClaudeMCPServerEntries(cfg *config.Config) map[string]any {
	mcpServers := make(map[string]any)
	for name, server := range cfg.MCPServers {
		entry := map[string]any{}
		applyMCPTransport(entry, server)
		if !server.IsEnabled() {
			entry["disabled"] = true
		}
		mcpServers[name] = entry
	}
	return mcpServers
}

// applyMCPTransport writes the transport-dependent keys of a single server
// entry. Claude Code keys remote transport on `type` (accepting "http", "sse",
// or "streamable-http"); a stdio entry with an empty command is invalid.
// See https://code.claude.com/docs/en/mcp.
func applyMCPTransport(entry map[string]any, server *config.MCPServer) {
	switch t := server.GetTransport(); t {
	case config.TransportHTTP, config.TransportSSE:
		entry["type"] = t
		if len(server.Headers) > 0 {
			entry["headers"] = server.Headers
		}
	default:
		entry["command"] = server.Command
		if len(server.Args) > 0 {
			entry["args"] = server.Args
		}
	}
	if len(server.Env) > 0 {
		entry["env"] = server.Env
	}
	if server.URL != "" {
		entry["url"] = server.URL
	}
}

// renderClaudePluginsJSON produces .claude/plugins.json. Lifted verbatim
// from the legacy claude.go::renderPluginsJSON.
func renderClaudePluginsJSON(cfg *config.Config) (string, error) {
	type pluginEntry struct {
		Marketplace string `json:"marketplace"`
		Name        string `json:"name"`
		Scope       string `json:"scope"`
		Enabled     bool   `json:"enabled"`
	}

	var plugins []pluginEntry
	for _, p := range cfg.Plugins {
		plugins = append(plugins, pluginEntry{
			Marketplace: p.Marketplace,
			Name:        p.Name,
			Scope:       p.GetScope(),
			Enabled:     p.IsEnabled(),
		})
	}

	jsonBytes, err := json.MarshalIndent(plugins, "", arraySidecarIndent)
	if err != nil {
		return "", fmt.Errorf("marshal plugins JSON: %w", err)
	}
	return string(jsonBytes) + "\n", nil
}

// piMCPServerEntries builds the mcpServers object of .pi/mcp.json. Unlike the
// settings-style sidecars this document is a plain {mcpServers: {...}} file, so
// it drops the escaping the shared `mcp_json` sidecar needs (pi has no `${VAR}`
// expansion of its own to preserve) and emits the stdio `command/args/env`
// form and the remote `url/headers` form. pi-mcp-adapter has no inline switch to
// turn a server off (that is a command writing .pi/mcp-adapter.json), so a
// disabled server is left out rather than written as an active one.
func piMCPServerEntries(cfg *config.Config) map[string]any {
	servers := presets.MCPServersByKey(cfg)
	if cfg == nil {
		return servers
	}
	for name, server := range cfg.MCPServers {
		if server != nil && !server.IsEnabled() {
			delete(servers, name)
		}
	}
	return servers
}

// LegacyMergeClaims is what clean may take back out of a provider-owned merged
// document at the base-relative path rel when no record of what was merged exists:
// the MCP server entries the current config would render, each guarded by that
// value so a hand-written server of the same name stays. See
// presets.LegacyMergeClaims for the preset-owned documents.
func LegacyMergeClaims(rel string, cfg *config.Config) []jsonmerge.Claim {
	return append(specLegacyClaims(rel, cfg), serverLegacyClaims(rel, cfg)...)
}

// specLegacyClaims derives, from the hooks and permissions sidecars the builtin
// specs declare for the document rel, what the current configuration would write
// there: the kind and dialect say which keys, the sidecar's format how they are
// recorded.
func specLegacyClaims(rel string, cfg *config.Config) []jsonmerge.Claim {
	if cfg == nil || (len(cfg.Hooks) == 0 && cfg.Permissions == nil) {
		return nil
	}
	var claims []jsonmerge.Claim
	seen := map[string]bool{}
	for _, spec := range loadBuiltinSpecs() {
		for _, sc := range spec.Sidecars {
			if sc == nil || filepath.ToSlash(sc.Path) != rel || !isMergedGenericSidecar(sc) || sc.Kind == SidecarMCP {
				continue
			}
			key := sc.Kind + "/" + sc.Dialect
			format := sc.DocFormat()
			if seen[key] || format == "" {
				continue
			}
			seen[key] = true
			var keys []jsonmerge.OwnedKey
			var err error
			if sc.Kind == SidecarHooks {
				keys, err = settings.HookKeys(cfg, sc.Dialect, "")
			} else {
				keys, err = settings.PermissionKeys(cfg, sc.Dialect, "")
			}
			if err != nil || len(keys) == 0 {
				continue
			}
			if result, err := docmerge.Apply("", docmerge.Format(format), keys); err == nil {
				claims = append(claims, result.Claims...)
			}
		}
	}
	return claims
}

// serverLegacyClaims is LegacyMergeClaims for the documents that hold MCP servers
// and Claude's settings.
func serverLegacyClaims(rel string, cfg *config.Config) []jsonmerge.Claim {
	if cfg == nil || (len(cfg.MCPServers) == 0 && !cfg.ManagesClaudeSettings() && !cfg.HasClaudeSettingsContent()) {
		return nil
	}
	var owned []jsonmerge.OwnedKey
	switch rel {
	case ".claude/settings.json":
		var err error
		if owned, err = claudeSettingsOwnedKeys(cfg, ""); err != nil {
			return nil
		}
		// Earlier versions also wrote the servers here. A clean with no record of
		// the run that wrote them still takes back an entry that is exactly what
		// they rendered, and only that.
		if len(cfg.MCPServers) > 0 && !cfg.UserScope {
			owned = append(owned, jsonmerge.OwnedKey{Name: settingsKeyMCPServers, Value: legacyClaudeMCPServerEntries(cfg), Members: true})
		}
	case presets.MergedDocMCPJSON:
		owned = mcpJSONOwnedKeys(cfg, false)
	case presets.MergedDocPiMCP:
		owned = []jsonmerge.OwnedKey{{Name: piMCPKeyServers, Value: piMCPServerEntries(cfg), Members: true}}
	default:
		return nil
	}
	result, err := jsonmerge.Apply("", owned)
	if err != nil {
		return nil
	}
	return result.Claims
}
