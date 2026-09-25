package providers

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/jsonmerge"
	"github.com/Goldziher/ai-rulez/internal/generator/presets"
)

// presetsResolveGlobalEffort is aliased so sidecar code can stay readable
// when calling the shared resolver from the presets package.
var presetsResolveGlobalEffort = presets.ResolveGlobalEffort

const (
	// settingsKeyMCPServers is the only top-level key the settings-style JSON
	// sidecars (.claude/settings.json, .mcp.json) own. Every other key in those
	// documents is hand-authored by the consumer and must survive generation.
	settingsKeyMCPServers = "mcpServers"

	// ampSettingsKeyEffort is the only top-level key ai-rulez owns in
	// .amp/settings.json; users keep arbitrary Amp settings alongside it.
	ampSettingsKeyEffort = "amp.anthropic.effort"

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
	case PredicateHasPlugins:
		return cfg != nil && len(cfg.Plugins) > 0
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
		return jsonmerge.Apply(outputPath, []jsonmerge.OwnedKey{
			{Name: settingsKeyMCPServers, Value: claudeMCPServerEntries(cfg)},
		})
	case SidecarMCPJSON:
		return jsonmerge.Apply(outputPath, []jsonmerge.OwnedKey{
			{Name: settingsKeyMCPServers, Value: mcpJSONServerEntries(cfg)},
		})
	case SidecarAmpSettingsJSON:
		return jsonmerge.Apply(outputPath, []jsonmerge.OwnedKey{
			{Name: ampSettingsKeyEffort, Value: g.resolveGlobalEffort(cfg)},
		})
	case SidecarClaudePluginsJSON:
		// .claude/plugins.json is a JSON array wholly owned by ai-rulez: there
		// are no user-authored sibling keys to preserve.
		body, err := renderClaudePluginsJSON(cfg)
		return sidecarRender{Body: body}, err
	}
	return sidecarRender{}, fmt.Errorf("unknown sidecar kind %q", kind)
}

// SidecarIsMergedDocument reports whether a sidecar kind produces a JSON object
// that ai-rulez merges into rather than replaces — a document where it owns a
// fixed set of top-level keys and the consumer may own others.
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
	case SidecarClaudeSettingsJSON, SidecarMCPJSON, SidecarAmpSettingsJSON:
		return true
	}
	return false
}

// MergedSidecarPaths returns every base-relative, slash-separated path that a
// builtin provider spec declares as a merged JSON document (see
// SidecarIsMergedDocument). Derived from the embedded specs so the set cannot
// drift from them.
func MergedSidecarPaths() []string {
	names, err := BuiltinNames()
	if err != nil {
		return nil
	}
	seen := make(map[string]bool)
	for _, name := range names {
		gen, err := LoadBuiltin(name)
		if err != nil {
			continue
		}
		for _, sidecar := range gen.Spec.Sidecars {
			if sidecar == nil || !SidecarIsMergedDocument(sidecar.Kind) {
				continue
			}
			seen[filepath.ToSlash(sidecar.Path)] = true
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
// Difference from claudeMCPServerEntries: `disabled` is emitted unconditionally
// (true or false), not only when the server is disabled.
func mcpJSONServerEntries(cfg *config.Config) map[string]any {
	mcpServers := make(map[string]any)
	if cfg == nil {
		return mcpServers
	}
	for name, server := range cfg.MCPServers {
		entry := map[string]any{
			"disabled": !server.IsEnabled(),
		}
		applyMCPTransport(entry, server)
		mcpServers[name] = entry
	}
	return mcpServers
}

// claudeMCPServerEntries builds the .claude/settings.json server map. Lifted
// verbatim from the legacy claude.go::renderSettingsJSON so the migrated output
// is byte-for-byte identical.
func claudeMCPServerEntries(cfg *config.Config) map[string]any {
	mcpServers := make(map[string]any)
	if cfg == nil {
		return mcpServers
	}
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
