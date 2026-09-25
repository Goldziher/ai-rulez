package config

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/Goldziher/ai-rulez/internal/logger"
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
	windowsDrive := driveLetter && len(normalized) >= 2 && normalized[1] == ':'
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

// Config field paths reported for invalid hook declarations.
const (
	fieldHookGroups  = "plugin.hooks"
	fieldHookEvent   = "plugin.hooks.event"
	fieldHookMatcher = "plugin.hooks.matcher"
	fieldHookActions = "plugin.hooks.hooks"
	fieldHookIf      = "plugin.hooks.hooks.if"
)

// Advisory messages for hook declarations that load fine but will not behave as
// authored. They are warnings rather than errors because both conditions are
// judgements about the *runtime's* event vocabulary: rejecting them would make an
// older ai-rulez refuse a config written against a newer Claude Code.
const (
	warnUnknownHookEvent = "plugin hook declares an event this ai-rulez does not know; " +
		"a misspelled event name never fires"
	warnIgnoredHookMatcher = "plugin hook sets a matcher on an event that has no matchable subject; " +
		"the runtime silently ignores it"
	warnInertHookIf = "plugin hook sets 'if' on an event that does not evaluate it; " +
		"the handler never runs at all rather than running conditionally"
)

// hookWarning is one non-fatal hook advisory, carried as data rather than logged
// at the point of detection so the detection logic stays pure and testable.
type hookWarning struct {
	Event   string
	Field   string
	Message string
}

// hookDeclarationWarnings reports hook declarations that are structurally valid
// but will not do what the author expects: an event outside KnownHookEvents, or a
// matcher on an event that ignores matchers.
func hookDeclarationWarnings(groups []HookGroup) []hookWarning {
	var warnings []hookWarning
	for _, g := range groups {
		if g.Event == "" {
			continue // reported as an error by validateHookGroups
		}
		if !slices.Contains(KnownHookEvents, g.Event) {
			warnings = append(warnings, hookWarning{
				Event:   g.Event,
				Field:   fieldHookEvent,
				Message: warnUnknownHookEvent,
			})
		}
		if g.Matcher != "" && slices.Contains(HookEventsWithoutMatcher, g.Event) {
			warnings = append(warnings, hookWarning{
				Event:   g.Event,
				Field:   fieldHookMatcher,
				Message: warnIgnoredHookMatcher,
			})
		}
		// Only warn for events this ai-rulez recognizes: an unknown event is already
		// flagged above, and guessing at a newer runtime's `if` support would be noise.
		if !slices.Contains(KnownHookEvents, g.Event) || slices.Contains(HookEventsEvaluatingIf, g.Event) {
			continue
		}
		for i := range g.Hooks {
			if g.Hooks[i].If == "" {
				continue
			}
			warnings = append(warnings, hookWarning{
				Event:   g.Event,
				Field:   fieldHookIf,
				Message: warnInertHookIf,
			})
		}
	}
	return warnings
}

// validateHookGroups checks hook declarations for a plugin: every group needs an
// event, and every action needs exactly one of 'command' or 'script'. Declarations
// that are valid but suspicious are warned about, never rejected.
func (c *Config) validateHookGroups(pluginName string, groups []HookGroup) error {
	for i, g := range groups {
		if g.Event == "" {
			return oops.
				With("field", fieldHookGroups).
				Hint("Each [[plugin.hooks]] group needs an 'event' (e.g. SessionStart)").
				Errorf("plugin %q hook group at index %d missing 'event'", pluginName, i)
		}
		for j := range g.Hooks {
			if err := c.validateHookAction(pluginName, g.Event, j, &g.Hooks[j]); err != nil {
				return err
			}
		}
	}
	for _, warning := range hookDeclarationWarnings(groups) {
		logger.Warn(warning.Message, "plugin", pluginName, "event", warning.Event, "field", warning.Field)
	}
	return nil
}

// validateHookAction enforces the command/script contract for one hook action. A
// declared script must exist now, at load time: the generator copies it into the
// plugin bundle, so a missing file would otherwise surface as a generation failure
// or, worse, a bundle whose hook points at nothing.
func (c *Config) validateHookAction(pluginName, event string, index int, action *HookAction) error {
	switch {
	case action.Command != "" && action.Script != "":
		return oops.
			With("field", fieldHookActions).
			With("event", event).
			With("script", action.Script).
			Hint("Use 'command' for an executable the consumer already has, or 'script' for a project "+
				"file ai-rulez bundles into the plugin's hooks/ directory").
			Errorf("plugin %q hook %s[%d] sets both 'command' and 'script'", pluginName, event, index)
	case action.Command == "" && action.Script == "":
		return oops.
			With("field", fieldHookActions).
			With("event", event).
			Hint("Each hook action needs a 'command' or a bundled 'script'").
			Errorf("plugin %q hook %s[%d] requires either 'command' or 'script'", pluginName, event, index)
	case action.Script == "":
		return nil
	}

	if isUnsafeProjectPath(action.Script) {
		return oops.
			With("field", fieldHookActions).
			With("event", event).
			With("script", action.Script).
			Hint("Use a project-relative script path that does not contain '..'").
			Errorf("plugin %q hook %s[%d] has an unsafe hook script %q", pluginName, event, index, action.Script)
	}

	// Resolved exactly as the generator's passthroughFile does: relative to the
	// config's source directory unless already absolute.
	resolved := action.Script
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(c.BaseDir, resolved)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return oops.
			With("field", fieldHookActions).
			With("event", event).
			With("path", resolved).
			Hint("Point 'script' at a script committed to the project, relative to the project root").
			Wrapf(err, "plugin %q hook %s[%d] hook script not found: %q", pluginName, event, index, action.Script)
	}
	if info.IsDir() {
		return oops.
			With("field", fieldHookActions).
			With("event", event).
			With("path", resolved).
			Hint("Point 'script' at a file, not a directory").
			Errorf("plugin %q hook %s[%d] hook script is a directory: %q", pluginName, event, index, action.Script)
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
