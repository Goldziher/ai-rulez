package config

import (
	"regexp"
	"strings"
	"sync"

	"github.com/samber/oops"
)

// GuardConfig configures the built-in generated-file guard: a PreToolUse hook that
// stops an agent from editing a file ai-rulez generated. It is a table of its own
// because `[hooks]` cannot coexist with the `[[hooks]]` array of tables in TOML.
type GuardConfig struct {
	// Generated, when true, makes generation add a PreToolUse hook to every
	// harness with a documented blocking PreToolUse hook. The hook runs
	// `ai-rulez guard`, which blocks edits to files in the generated manifest.
	Generated bool `yaml:"generated,omitempty" json:"generated,omitempty" toml:"generated,omitempty"`
	// Version pins the ai-rulez version the hook runs through npx. Empty means the
	// version of the running binary, or "latest" for a dev build.
	Version string `yaml:"version,omitempty" json:"version,omitempty" toml:"version,omitempty"`
	// Command replaces the whole launch command (executable followed by its
	// arguments, without the trailing "guard"), for installs that do not run
	// through npx, for example ["ai-rulez"]. Version is ignored when it is set.
	Command []string `yaml:"command,omitempty" json:"command,omitempty" toml:"command,omitempty"`
}

// HookBuiltinGuard marks the HookGroup that generation synthesizes for [guard].
const HookBuiltinGuard = "guard"

// GuardHarnesses are the harnesses whose documented PreToolUse hook blocks a tool
// call when the hook exits with code 2 and writes the reason to stderr. A harness
// outside this list is skipped, never approximated.
var GuardHarnesses = []string{HarnessClaude, HarnessCodex, HarnessGemini, HarnessCursor, HarnessFactory}

// guardMatcher selects the file-editing tools in Claude Code's vocabulary; each
// harness's own names are derived through internal/toolnames.
const guardMatcher = "Edit|Write|MultiEdit"

// guardTimeoutSeconds bounds the hook; the guard reads two small files.
const guardTimeoutSeconds = 10

// guardMu serializes EnableGuardHooks: generators built concurrently share one Config.
var guardMu sync.Mutex

var guardVersionPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// HasGuard reports whether generation should add the generated-file guard hook.
func (c *Config) HasGuard() bool {
	return c != nil && c.Guard != nil && c.Guard.Generated && !c.UserScope
}

// GuardCommand is the shell command of the guard hook. It resolves the executable
// the way the self MCP server entry does (SelfMCPServerEntry): npx pinned to the
// binary's version unless the config pins another or replaces the command.
func (c *Config) GuardCommand(binaryVersion string) string {
	var argv []string
	switch {
	case c.Guard != nil && len(c.Guard.Command) > 0:
		argv = append(argv, c.Guard.Command...)
	default:
		version := ""
		if c.Guard != nil {
			version = c.Guard.Version
		}
		if version == "" {
			version = binaryVersion
		}
		if version == "" || version == "dev" {
			version = "latest"
		}
		argv = []string{"npx", "-y", "ai-rulez@" + version}
	}
	argv = append(argv, "guard")
	parts := make([]string, len(argv))
	for i, arg := range argv {
		parts[i] = shellQuoteArg(arg)
	}
	return strings.Join(parts, " ")
}

func shellQuoteArg(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\n\"'\\$`!&|;<>()*?[]{}~#") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// EnableGuardHooks adds the synthesized guard group to Hooks when [guard] is on.
// It is idempotent, so every generator built from the same Config shares one group.
// The group is marked HookBuiltinGuard and never written back to config.toml.
func (c *Config) EnableGuardHooks(binaryVersion string) {
	if !c.HasGuard() {
		return
	}
	guardMu.Lock()
	defer guardMu.Unlock()
	for i := range c.Hooks {
		if c.Hooks[i].Builtin == HookBuiltinGuard {
			return
		}
	}
	c.Hooks = append(c.Hooks, HookGroup{
		Event:   "PreToolUse",
		Matcher: guardMatcher,
		Targets: append([]string(nil), GuardHarnesses...),
		// Factory documents no MultiEdit tool; Codex edits files through apply_patch,
		// which Edit and Write also select.
		Matchers: map[string]string{HarnessFactory: "Edit|Create", HarnessCodex: "apply_patch|Edit|Write"},
		Builtin:  HookBuiltinGuard,
		Hooks: []HookAction{{
			Type:    HookTypeCommand,
			Command: c.GuardCommand(binaryVersion),
			Timeout: guardTimeoutSeconds,
		}},
	})
}

// UserHooks returns the hook groups the user wrote, without synthesized ones.
func (c *Config) UserHooks() []HookGroup {
	var out []HookGroup
	for _, g := range c.Hooks {
		if g.Builtin == "" {
			out = append(out, g)
		}
	}
	return out
}

// validateGuard rejects the tuning fields when the guard is off, since they would
// be silently inert, and values that cannot form a launch command.
func (c *Config) validateGuard() error {
	g := c.Guard
	if g == nil {
		return nil
	}
	if !g.Generated && (g.Version != "" || len(g.Command) > 0) {
		return oops.
			Hint("Set `generated = true` under [guard], or remove version / command").
			Errorf("guard.version and guard.command require guard.generated = true")
	}
	if len(g.Command) > 0 && strings.TrimSpace(g.Command[0]) == "" {
		return oops.
			Hint("The first element of guard.command is the executable, e.g. [\"ai-rulez\"]").
			Errorf("guard.command executable must not be empty")
	}
	if len(g.Command) > 0 && g.Version != "" {
		return oops.
			Hint("guard.command replaces the whole launch command, so a pinned version has no effect; remove one of them").
			Errorf("guard.version and guard.command are mutually exclusive")
	}
	if g.Version != "" && !guardVersionPattern.MatchString(g.Version) {
		return oops.
			Hint("Use a bare version or npm dist-tag such as \"5.0.0\" or \"latest\"").
			Errorf("guard.version %q is not a valid version", g.Version)
	}
	return nil
}
