package config

import (
	"path"
	"regexp"
	"strings"

	"github.com/samber/oops"
)

// semverLike matches a lenient semantic-version shape (major.minor[.patch][-pre]).
// Plugin manifests across runtimes expect a version string; we only enforce a
// sane shape, not strict SemVer 2.0.
var semverLike = regexp.MustCompile(
	`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)` +
		`(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`,
)

// validPluginRuntimes is the set membership test for authored runtime names.
func isValidPluginRuntime(name string) bool {
	for _, r := range AllPluginRuntimes {
		if r == name {
			return true
		}
	}
	return false
}

// validatePluginAuthoring checks the [plugin] authoring block when present.
func (c *Config) validatePluginAuthoring() error {
	p := c.Plugin
	if p == nil {
		return nil
	}
	if err := validatePluginRequiredFields(p); err != nil {
		return err
	}
	if err := validatePluginPaths(p); err != nil {
		return err
	}
	if pluginTargetsRuntime(p, PluginRuntimeCodex) {
		if err := validateCodexPluginMetadata(p); err != nil {
			return err
		}
	}
	if err := validatePluginRuntimes(p); err != nil {
		return err
	}
	if err := validatePluginMCP(p); err != nil {
		return err
	}
	if p.Statusline != nil && p.Statusline.Script == "" {
		return oops.
			With("field", "plugin.statusline.script").
			Hint("Point 'script' at the status-line script to bundle").
			Errorf("plugin %q statusline requires 'script'", p.Name)
	}
	return c.validateHookGroups(p.Name, p.Hooks)
}

func validatePluginRequiredFields(p *PluginAuthoring) error {
	if p.Name == "" {
		return oops.
			With("field", "plugin.name").
			Hint("Add a 'name' to the [plugin] block").
			Errorf("plugin authoring requires field 'name'")
	}
	if p.Version == "" {
		return oops.
			With("field", "plugin.version").
			With("plugin_name", p.Name).
			Hint("Add a 'version' to the [plugin] block, e.g. version = \"1.0.0\"").
			Errorf("plugin %q requires field 'version'", p.Name)
	}
	if !semverLike.MatchString(p.Version) {
		return oops.
			With("field", "plugin.version").
			With("value", p.Version).
			Hint("Use a semantic version like 1.0.0 or 0.2.1-beta").
			Errorf("plugin %q has an invalid version %q", p.Name, p.Version)
	}
	if p.Description == "" {
		return oops.
			With("field", "plugin.description").
			With("plugin_name", p.Name).
			Hint("Add a 'description' to the [plugin] block").
			Errorf("plugin %q requires field 'description'", p.Name)
	}
	return nil
}

func validatePluginPaths(p *PluginAuthoring) error {
	if p.ContentRoot != "" && isUnsafeProjectPath(p.ContentRoot) {
		return oops.With("field", "plugin.content_root").With("value", p.ContentRoot).
			Hint("Use a project-relative directory that does not contain '..'").
			Errorf("plugin %q has an unsafe content root", p.Name)
	}
	if p.Hermes != nil && p.Hermes.Source != "" && isUnsafeProjectPath(p.Hermes.Source) {
		return oops.With("field", "plugin.hermes.source").With("value", p.Hermes.Source).
			Hint("Use a project-relative Python file that does not contain '..'").
			Errorf("plugin %q has an unsafe Hermes source", p.Name)
	}
	return nil
}

func isUnsafeProjectPath(value string) bool {
	normalized := strings.ReplaceAll(value, `\`, "/")
	cleaned := path.Clean(normalized)
	driveLetter := len(normalized) >= 1 && ((normalized[0] >= 'a' && normalized[0] <= 'z') ||
		(normalized[0] >= 'A' && normalized[0] <= 'Z'))
	windowsDrive := driveLetter && len(normalized) >= 3 && normalized[1] == ':' && normalized[2] == '/'
	return path.IsAbs(normalized) || windowsDrive || strings.HasPrefix(normalized, "//") ||
		cleaned == ".." || strings.HasPrefix(cleaned, "../")
}

func validatePluginRuntimes(p *PluginAuthoring) error {
	seen := make(map[string]bool, len(p.Runtimes))
	for _, r := range p.Runtimes {
		if !isValidPluginRuntime(r) {
			return oops.
				With("field", "plugin.runtimes").
				With("value", r).
				Hint("Valid runtimes: claude, cursor, codex, gemini, kimi, opencode, factory, hermes").
				Errorf("plugin %q lists unknown runtime %q", p.Name, r)
		}
		if seen[r] {
			return oops.
				With("field", "plugin.runtimes").
				With("value", r).
				Errorf("plugin %q lists duplicate runtime %q", p.Name, r)
		}
		seen[r] = true
	}
	return nil
}

func validatePluginMCP(p *PluginAuthoring) error {
	for i, m := range p.MCP {
		if m.Name == "" {
			return oops.
				With("field", "plugin.mcp").
				Hint("Each [[plugin.mcp]] entry needs a 'name'").
				Errorf("plugin %q MCP entry at index %d missing 'name'", p.Name, i)
		}
		remote := m.Transport == TransportHTTP || m.Transport == TransportSSE
		switch {
		case remote && m.URL == "":
			return oops.
				With("field", "plugin.mcp").
				With("mcp_name", m.Name).
				Hint("http/sse MCP servers require a 'url'").
				Errorf("plugin %q MCP server %q has transport %q but no url", p.Name, m.Name, m.Transport)
		case !remote && m.Command == "":
			return oops.
				With("field", "plugin.mcp").
				With("mcp_name", m.Name).
				Hint("stdio MCP servers require a 'command' (or set transport = \"http\"/\"sse\" with a url)").
				Errorf("plugin %q MCP server %q has no command", p.Name, m.Name)
		}
	}
	return nil
}

func pluginTargetsRuntime(plugin *PluginAuthoring, runtime string) bool {
	for _, candidate := range plugin.ResolvedRuntimes() {
		if candidate == runtime {
			return true
		}
	}
	return false
}

func validateCodexPluginMetadata(plugin *PluginAuthoring) error {
	if plugin.Author == nil || plugin.Author.Name == "" {
		return oops.With("field", "plugin.author.name").
			Hint("Codex plugins require [plugin.author] with a non-empty name").
			Errorf("plugin %q requires author.name for the Codex runtime", plugin.Name)
	}
	if plugin.Interface == nil {
		return oops.With("field", "plugin.interface").
			Hint("Codex plugins require a [plugin.interface] block").
			Errorf("plugin %q requires interface metadata for the Codex runtime", plugin.Name)
	}
	required := []struct {
		name  string
		value string
	}{
		{"display_name", plugin.Interface.DisplayName},
		{"short_description", plugin.Interface.ShortDescription},
		{"long_description", plugin.Interface.LongDescription},
		{"developer_name", plugin.Interface.DeveloperName},
		{"category", plugin.Interface.Category},
	}
	for _, field := range required {
		if field.value == "" {
			return oops.With("field", "plugin.interface."+field.name).
				Errorf("plugin %q requires interface.%s for the Codex runtime", plugin.Name, field.name)
		}
	}
	if plugin.Interface.Capabilities == nil {
		return oops.With("field", "plugin.interface.capabilities").
			Errorf("plugin %q requires interface.capabilities for the Codex runtime", plugin.Name)
	}
	if len(plugin.Interface.DefaultPrompt) == 0 {
		return oops.With("field", "plugin.interface.default_prompt").
			Errorf("plugin %q requires interface.default_prompt for the Codex runtime", plugin.Name)
	}
	return nil
}

// validateHookGroups checks hook declarations for a plugin.
func (c *Config) validateHookGroups(pluginName string, groups []HookGroup) error {
	for i, g := range groups {
		if g.Event == "" {
			return oops.
				With("field", "plugin.hooks").
				Hint("Each [[plugin.hooks]] group needs an 'event' (e.g. SessionStart)").
				Errorf("plugin %q hook group at index %d missing 'event'", pluginName, i)
		}
		for j, action := range g.Hooks {
			if action.Command == "" {
				return oops.
					With("field", "plugin.hooks.hooks").
					With("event", g.Event).
					Hint("Each hook action needs a 'command'").
					Errorf("plugin %q hook %s[%d] missing 'command'", pluginName, g.Event, j)
			}
		}
	}
	return nil
}

// validateMarketplaceAuthoring checks the [marketplace] authoring block.
func (c *Config) validateMarketplaceAuthoring() error {
	m := c.Marketplace
	if m == nil {
		return nil
	}
	if m.Name == "" {
		return oops.
			With("field", "marketplace.name").
			Hint("Add a 'name' to the [marketplace] block").
			Errorf("marketplace authoring requires field 'name'")
	}
	// Member existence is checked at generation time; here we reject duplicates
	// and paths that escape the project root (absolute or containing "..").
	seen := make(map[string]bool, len(m.Members))
	for _, member := range m.Members {
		if member == "" {
			return oops.
				With("field", "marketplace.members").
				Errorf("marketplace %q has an empty member path", m.Name)
		}
		if isUnsafeProjectPath(member) {
			return oops.
				With("field", "marketplace.members").
				With("value", member).
				Hint("Member paths must be relative to the project root and cannot use '..'").
				Errorf("marketplace %q has an invalid member path %q", m.Name, member)
		}
		if seen[member] {
			return oops.
				With("field", "marketplace.members").
				With("value", member).
				Errorf("marketplace %q lists duplicate member %q", m.Name, member)
		}
		seen[member] = true
	}
	return nil
}
