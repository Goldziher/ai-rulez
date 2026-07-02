package providers

import (
	"encoding/json"
	"fmt"

	"github.com/Goldziher/ai-rulez/internal/config"
	"github.com/Goldziher/ai-rulez/internal/generator/presets"
)

// presetsResolveGlobalEffort is aliased so sidecar code can stay readable
// when calling the shared resolver from the presets package.
var presetsResolveGlobalEffort = presets.ResolveGlobalEffort

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

// renderSidecar dispatches the closed-set sidecar kind. Method on Generator
// so kind-specific renderers (e.g. amp_settings_json) can read the spec's
// effort_map.
func (g *Generator) renderSidecar(kind string, cfg *config.Config) (string, error) {
	switch kind {
	case SidecarClaudeSettingsJSON:
		return renderClaudeSettingsJSON(cfg)
	case SidecarClaudePluginsJSON:
		return renderClaudePluginsJSON(cfg)
	case SidecarMCPJSON:
		return renderMCPJSON(cfg)
	case SidecarAmpSettingsJSON:
		return g.renderAmpSettingsJSON(cfg)
	}
	return "", fmt.Errorf("unknown sidecar kind %q", kind)
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

// renderMCPJSON produces .mcp.json. Lifted verbatim from the legacy
// MCPPresetGenerator.Generate body so the output is byte-identical.
// Difference from claude_settings_json: `disabled` is emitted
// unconditionally (true or false), not only when the server is disabled.
func renderMCPJSON(cfg *config.Config) (string, error) {
	mcpServers := make(map[string]any)
	for name, server := range cfg.MCPServers {
		entry := map[string]any{
			"disabled": !server.IsEnabled(),
		}
		// Claude Code keys remote transport on `type` (accepting "http", "sse",
		// or "streamable-http"); a stdio entry with an empty command is invalid.
		// See https://code.claude.com/docs/en/mcp.
		switch t := server.GetTransport(); t {
		case "http", "sse":
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
		mcpServers[name] = entry
	}
	payload := map[string]any{"mcpServers": mcpServers}
	jsonBytes, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal mcp JSON: %w", err)
	}
	return string(jsonBytes) + "\n", nil
}

// renderAmpSettingsJSON produces .amp/settings.json with `amp.anthropic.effort`
// set to the spec's effort_map translation of the resolved global tier.
// Bypasses re-resolving — evalPredicate already confirmed a value exists.
func (g *Generator) renderAmpSettingsJSON(cfg *config.Config) (string, error) {
	effort := g.resolveGlobalEffort(cfg)
	settings := map[string]string{"amp.anthropic.effort": effort}
	jsonBytes, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal amp settings: %w", err)
	}
	return string(jsonBytes) + "\n", nil
}

// renderClaudeSettingsJSON produces .claude/settings.json. Lifted verbatim
// from the legacy claude.go::renderSettingsJSON so the migrated output is
// byte-for-byte identical.
func renderClaudeSettingsJSON(cfg *config.Config) (string, error) {
	mcpServers := make(map[string]any)
	for name, server := range cfg.MCPServers {
		entry := map[string]any{}
		// Claude Code keys remote transport on `type` (accepting "http", "sse",
		// or "streamable-http"); a stdio entry with an empty command is invalid.
		// See https://code.claude.com/docs/en/mcp.
		switch t := server.GetTransport(); t {
		case "http", "sse":
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
		if !server.IsEnabled() {
			entry["disabled"] = true
		}
		mcpServers[name] = entry
	}
	settings := map[string]any{
		"mcpServers": mcpServers,
	}
	jsonBytes, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal settings JSON: %w", err)
	}
	return string(jsonBytes) + "\n", nil
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

	jsonBytes, err := json.MarshalIndent(plugins, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal plugins JSON: %w", err)
	}
	return string(jsonBytes) + "\n", nil
}
