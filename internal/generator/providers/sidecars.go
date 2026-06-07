package providers

import (
	"encoding/json"
	"fmt"

	"github.com/Goldziher/ai-rulez/internal/config"
)

// evalPredicate dispatches the closed-set emit_when value against Config.
// Empty predicate or "always" → always emit; "has_mcp_servers" / "has_plugins"
// → emit only when the corresponding config block is non-empty.
func evalPredicate(predicate string, cfg *config.Config) bool {
	switch predicate {
	case "", PredicateAlways:
		return true
	case PredicateHasMCPServers:
		return cfg != nil && len(cfg.MCPServers) > 0
	case PredicateHasPlugins:
		return cfg != nil && len(cfg.Plugins) > 0
	}
	return false
}

// renderSidecar dispatches the closed-set sidecar kind to its Go handler.
// Adding a new kind is a deliberate three-step change: extend the enum in
// spec.go, accept it in loader.isValidSidecarKind, and add a case here.
func renderSidecar(kind string, cfg *config.Config) (string, error) {
	switch kind {
	case SidecarClaudeSettingsJSON:
		return renderClaudeSettingsJSON(cfg)
	case SidecarClaudePluginsJSON:
		return renderClaudePluginsJSON(cfg)
	}
	return "", fmt.Errorf("unknown sidecar kind %q", kind)
}

// renderClaudeSettingsJSON produces .claude/settings.json. Lifted verbatim
// from the legacy claude.go::renderSettingsJSON so the migrated output is
// byte-for-byte identical.
func renderClaudeSettingsJSON(cfg *config.Config) (string, error) {
	mcpServers := make(map[string]any)
	for name, server := range cfg.MCPServers {
		entry := map[string]any{
			"command": server.Command,
		}
		if len(server.Args) > 0 {
			entry["args"] = server.Args
		}
		if len(server.Env) > 0 {
			entry["env"] = server.Env
		}
		if server.Transport != "" {
			entry["transport"] = server.GetTransport()
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
