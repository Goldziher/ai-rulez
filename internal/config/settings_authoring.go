package config

import (
	"slices"
	"sort"
	"strings"

	"github.com/Goldziher/ai-rulez/v5/internal/logger"
	"github.com/samber/oops"
)

// This file defines the top-level [[hooks]], [permissions] and
// [claude.settings.managed] blocks: project (or user) settings that are not part
// of a plugin bundle. They are rendered into each harness's native settings
// file and merged into it key by key, so everything the consumer hand-authored
// in the same file survives.

// Harness names accepted in HookGroup.Targets and HookGroup.Matchers.
const (
	HarnessClaude  = "claude"
	HarnessCodex   = "codex"
	HarnessCursor  = "cursor"
	HarnessGemini  = "gemini"
	HarnessCopilot = "copilot"
)

// HookHarnesses lists the harnesses `[[hooks]]` can be rendered for, in a stable
// order. Every other preset is reported as unsupported.
var HookHarnesses = slices.Concat([]string{HarnessClaude, HarnessCodex, HarnessCursor, HarnessGemini, HarnessCopilot}, extraHookHarnesses)

// Permissions is the [permissions] block: allow, ask and deny rule lists in
// Claude Code permission-rule syntax ("Bash(git *)", "Read(./.env)"). It is
// rendered into .claude/settings.json verbatim and translated into the native
// permission format of the other harnesses that have one (see
// docs/permissions.md). Each listed rule is owned element by element, so rules
// the consumer wrote in the same arrays survive.
type Permissions struct {
	Allow []string `yaml:"allow,omitempty" json:"allow,omitempty" toml:"allow,omitempty"`
	Ask   []string `yaml:"ask,omitempty" json:"ask,omitempty" toml:"ask,omitempty"`
	Deny  []string `yaml:"deny,omitempty" json:"deny,omitempty" toml:"deny,omitempty"`
}

// IsEmpty reports whether no rule is declared.
func (p *Permissions) IsEmpty() bool {
	return p == nil || len(p.Allow)+len(p.Ask)+len(p.Deny) == 0
}

// ManagedSettings is the [claude.settings.managed] block: keys of
// .claude/settings.json ai-rulez owns entry by entry. Unlike the plugin keys of
// [claude.settings] it needs no `manage = true`: declaring an entry is the opt-in.
type ManagedSettings struct {
	// Env owns env.<NAME> for every listed variable.
	Env map[string]string `yaml:"env,omitempty" json:"env,omitempty" toml:"env,omitempty"`
	// SkillOverrides owns skillOverrides.<skill> for every listed skill. Values
	// are on, name-only, user-invocable-only or off.
	SkillOverrides map[string]string `yaml:"skill_overrides,omitempty" json:"skill_overrides,omitempty" toml:"skill_overrides,omitempty"` //nolint:tagliatelle
}

// IsEmpty reports whether no managed key is declared.
func (m *ManagedSettings) IsEmpty() bool {
	return m == nil || len(m.Env)+len(m.SkillOverrides) == 0
}

// Claude Code skillOverrides states that hide a skill from the model.
const (
	SkillModeOff               = "off"
	SkillModeUserInvocableOnly = "user-invocable-only"
)

// skillOverrideValues are the states Claude Code documents for skillOverrides.
var skillOverrideValues = []string{"on", "name-only", SkillModeUserInvocableOnly, SkillModeOff}

// SkillOverrideValues returns the documented skillOverrides states.
func SkillOverrideValues() []string { return slices.Clone(skillOverrideValues) }

// HasSettingsHooks reports whether top-level [[hooks]] are declared.
func (c *Config) HasSettingsHooks() bool { return c != nil && len(c.Hooks) > 0 }

// ManagedClaudeSettings returns the [claude.settings.managed] block, or nil.
func (c *Config) ManagedClaudeSettings() *ManagedSettings {
	if c == nil || c.Claude == nil || c.Claude.Settings == nil {
		return nil
	}
	return c.Claude.Settings.Managed
}

// HasClaudeSettingsContent reports whether .claude/settings.json carries any key
// declared by [[hooks]], [permissions] or [claude.settings.managed].
func (c *Config) HasClaudeSettingsContent() bool {
	return c.HasSettingsHooks() || !c.Permissions.IsEmpty() || !c.ManagedClaudeSettings().IsEmpty()
}

// HookTargetsHarness reports whether a hook group renders for the harness: a
// group without targets renders for every supported harness.
func (g *HookGroup) HookTargetsHarness(harness string) bool {
	return len(g.Targets) == 0 || slices.Contains(g.Targets, harness)
}

// Warning messages of the settings validators. Advisory only, because they are
// judgements about the runtimes' vocabulary that a newer runtime may outgrow.
const warnOverbroadPermission = "permission rule allows every call of a tool; " +
	"name the commands or paths it may run, for example Bash(git status)"

// overbroadAllowRule reports whether a Claude Code rule allows every call of a tool.
func overbroadAllowRule(rule string) bool {
	r := strings.TrimSpace(rule)
	if r == "*" {
		return true
	}
	name, args, found := strings.Cut(r, "(")
	if !found {
		// A bare tool name allows every call of it (Bash, Edit, WebFetch).
		return name != "" && !strings.HasPrefix(name, "mcp__")
	}
	args = strings.TrimSuffix(args, ")")
	return strings.Trim(args, "* ") == "" && strings.TrimSpace(name) != ""
}

// OverbroadAllowRules returns the allow rules that permit every call of a tool.
func (p *Permissions) OverbroadAllowRules() []string {
	if p == nil {
		return nil
	}
	var out []string
	for _, rule := range p.Allow {
		if overbroadAllowRule(rule) {
			out = append(out, rule)
		}
	}
	return out
}

// validateSettingsBlocks checks [[hooks]], [permissions] and
// [claude.settings.managed].
func (c *Config) validateSettingsBlocks() error {
	if err := c.validateSettingsHooks(); err != nil {
		return err
	}
	if err := c.validatePermissions(); err != nil {
		return err
	}
	return c.validateManagedSettings()
}

func (c *Config) validateSettingsHooks() error {
	for i, g := range c.Hooks {
		if g.Event == "" {
			return oops.With("field", "hooks").
				Hint("Each [[hooks]] group needs an 'event' (e.g. PreToolUse)").
				Errorf("hook group at index %d missing 'event'", i)
		}
		for _, target := range g.Targets {
			if !slices.Contains(HookHarnesses, target) {
				return oops.With("field", "hooks.targets").With("value", target).
					Hint("Use one of: "+strings.Join(HookHarnesses, ", ")).
					Errorf("hook group %d targets unknown harness %q", i, target)
			}
		}
		for harness := range g.Matchers {
			if !slices.Contains(HookHarnesses, harness) {
				return oops.With("field", "hooks.matchers").With("value", harness).
					Hint("Use one of: "+strings.Join(HookHarnesses, ", ")).
					Errorf("hook group %d sets a matcher for unknown harness %q", i, harness)
			}
		}
		for j := range g.Hooks {
			if err := validateSettingsHookAction(g.Event, j, &g.Hooks[j]); err != nil {
				return err
			}
		}
	}
	for _, warning := range hookDeclarationWarnings(c.Hooks) {
		logger.Warn(strings.TrimPrefix(warning.Message, "plugin "), "event", warning.Event,
			"field", strings.Replace(warning.Field, "plugin.hooks", "hooks", 1))
	}
	return nil
}

// validateSettingsHookAction checks one action of a top-level [[hooks]] group:
// exactly one of command or script, and a script that stays inside the project.
// Whether the script exists and is executable is reported by `validate --strict`
// (AR504, AR505), because the file may be produced by a build step.
func validateSettingsHookAction(event string, index int, action *HookAction) error {
	switch {
	case action.Type != "" && action.Type != HookTypeCommand:
		// The JSON schema (validate) allows only "command" here, and the settings
		// renderers write only command handlers: accepting another type would
		// silently turn a prompt or http hook into a shell command.
		return oops.With("field", "hooks.hooks").With("event", event).With("type", action.Type).
			Hint("Only type = \"command\" is supported for top-level [[hooks]]; omit 'type' or set it to \"command\"").
			Errorf("hook %s[%d] has unsupported type %q", event, index, action.Type)
	case action.Command != "" && action.Script != "":
		return oops.With("field", "hooks.hooks").With("event", event).
			Hint("Use 'command' for an executable the harness can resolve, or 'script' for a file in the project").
			Errorf("hook %s[%d] sets both 'command' and 'script'", event, index)
	case action.Command == "" && action.Script == "":
		return oops.With("field", "hooks.hooks").With("event", event).
			Hint("Each hook action needs a 'command' or a 'script'").
			Errorf("hook %s[%d] requires either 'command' or 'script'", event, index)
	case action.Script != "" && isUnsafeProjectPath(action.Script):
		return oops.With("field", "hooks.hooks").With("event", event).With("script", action.Script).
			Hint("Use a project-relative script path that does not contain '..'").
			Errorf("hook %s[%d] has an unsafe script %q", event, index, action.Script)
	case action.Script != "" && !IsSafeHookScript(action.Script):
		return oops.With("field", "hooks.hooks").With("event", event).With("script", action.Script).
			Hint("Rename the script so its path only uses letters, digits, '.', '_', '-' and '/'").
			Errorf("hook %s[%d] script %q may only contain letters, digits, '.', '_', '-' and '/'", event, index, action.Script)
	}
	return nil
}

func (c *Config) validatePermissions() error {
	if c.Permissions == nil {
		return nil
	}
	lists := map[string][]string{"allow": c.Permissions.Allow, "ask": c.Permissions.Ask, "deny": c.Permissions.Deny}
	names := make([]string, 0, len(lists))
	for name := range lists {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		for _, rule := range lists[name] {
			if strings.TrimSpace(rule) == "" {
				return oops.With("field", "permissions."+name).
					Errorf("permissions.%s holds an empty rule", name)
			}
		}
	}
	for _, rule := range c.Permissions.OverbroadAllowRules() {
		logger.Warn(warnOverbroadPermission, "field", "permissions.allow", "rule", rule)
	}
	return nil
}

func (c *Config) validateManagedSettings() error {
	managed := c.ManagedClaudeSettings()
	if managed == nil {
		return nil
	}
	for name := range managed.Env {
		if strings.TrimSpace(name) == "" || strings.ContainsAny(name, "= \t") {
			return oops.With("field", "claude.settings.managed.env").With("value", name).
				Errorf("%q is not a valid environment variable name", name)
		}
	}
	for skill, state := range managed.SkillOverrides {
		if strings.TrimSpace(skill) == "" {
			return oops.With("field", "claude.settings.managed.skill_overrides").Errorf("skill_overrides holds an empty skill name")
		}
		if !slices.Contains(skillOverrideValues, state) {
			return oops.With("field", "claude.settings.managed.skill_overrides").With("value", state).
				Hint("Use one of: "+strings.Join(skillOverrideValues, ", ")).
				Errorf("skill_overrides.%s has unsupported state %q", skill, state)
		}
	}
	return nil
}
